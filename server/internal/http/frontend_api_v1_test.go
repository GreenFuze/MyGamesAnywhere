package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/contentdelivery"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/frontendauth"
	"github.com/go-chi/chi/v5"
)

// scopedFrontendAPIClientService honours the scopes a route demands, which the
// simpler fake in frontend_api_client_controller_test.go deliberately ignores.
// It also records what was demanded, so a test can prove a route asked for its
// declared scope rather than merely that some refusal happened.
type scopedFrontendAPIClientService struct {
	principal frontendauth.Principal
	rejectAll error
	required  []frontendauth.Scope
}

func (s *scopedFrontendAPIClientService) Create(context.Context, string, string, []frontendauth.Scope, *time.Time) (*frontendauth.IssuedClient, error) {
	return nil, nil
}
func (s *scopedFrontendAPIClientService) List(context.Context, string) ([]frontendauth.Client, error) {
	return nil, nil
}
func (s *scopedFrontendAPIClientService) Rotate(context.Context, string, string) (*frontendauth.IssuedClient, error) {
	return nil, nil
}
func (s *scopedFrontendAPIClientService) Revoke(context.Context, string, string) (*frontendauth.Client, error) {
	return nil, nil
}

func (s *scopedFrontendAPIClientService) Authenticate(_ context.Context, _ string, required ...frontendauth.Scope) (frontendauth.Principal, error) {
	s.required = required
	if s.rejectAll != nil {
		return frontendauth.Principal{}, s.rejectAll
	}
	for _, scope := range required {
		if !s.principal.Has(scope) {
			return frontendauth.Principal{}, frontendauth.ErrForbidden
		}
	}
	return s.principal, nil
}

var frontendPathParameter = regexp.MustCompile(`\{[^}]+\}`)

func frontendTestProfile() *core.Profile {
	return &core.Profile{ID: "profile-1", DisplayName: "Frontend", Role: core.ProfileRoleAdminPlayer}
}

// buildFrontendAPIRouter builds the real router, not a hand-assembled one, so
// these tests see whatever registerFrontendAPIV1 actually mounted.
func buildFrontendAPIRouter(t *testing.T, service FrontendAPIClientService) chi.Router {
	t.Helper()
	controller, err := NewFrontendAPIClientController(service, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	return BuildRouter(&RouteBuilder{
		GameCtrl: &GameController{}, CatalogCtrl: &CatalogController{}, ContentCtrl: &ContentController{},
		MediaCtrl: &MediaController{}, DiscoCtrl: &DiscoveryController{}, AboutCtrl: &AboutController{},
		ConfigCtrl: &ConfigController{}, PluginCtrl: &PluginController{}, ReviewCtrl: &ReviewController{},
		AchievementCtrl: &AchievementController{}, SyncCtrl: &SyncController{}, UpdateCtrl: &UpdateController{},
		SaveSyncCtrl: &SaveSyncController{}, CacheCtrl: &CacheController{}, SSECtrl: &SSEController{},
		OAuthCtrl: &OAuthController{}, ProfileCtrl: &ProfileController{}, AuthCtrl: &AuthController{},
		FrontendAPIClientCtrl: controller, FrontendAPIClientSvc: service,
		ProfileRepo: lanProfileRepository{profile: frontendTestProfile()},
	}, 0, "")
}

// frontendAPIPaths collects the scoped routes from a built router. Reading them
// back off the router is the point: a route that exists but was never declared
// in the table would still be found here and still have to fail closed.
func frontendAPIPaths(t *testing.T, router chi.Router) []RouteKey {
	t.Helper()
	var keys []RouteKey
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/api/frontend/v1/") {
			keys = append(keys, RouteKey{Method: method, Path: route})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

// RouteKey is a method and path pair discovered from the router.
type RouteKey struct {
	Method string
	Path   string
}

func (k RouteKey) request(body string) *http.Request {
	target := frontendPathParameter.ReplaceAllString(k.Path, "test-id")
	if body == "" {
		return httptest.NewRequest(k.Method, target, nil)
	}
	return httptest.NewRequest(k.Method, target, strings.NewReader(body))
}

func TestEveryScopedFrontendRouteFailsClosed(t *testing.T) {
	// The ticket exists because the scope system gated nothing. This walks the
	// router rather than the table, so a route added later without a scope, or
	// registered outside registerFrontendAPIV1, is caught here.
	service := &scopedFrontendAPIClientService{
		principal: frontendauth.Principal{ClientID: "client-1", ProfileID: "profile-1", Scopes: frontendauth.AllScopes()},
	}
	router := buildFrontendAPIRouter(t, service)
	routes := frontendAPIPaths(t, router)
	if len(routes) < 14 {
		t.Fatalf("expected the scoped surface to be mounted, found %d routes", len(routes))
	}

	probes := []struct {
		name       string
		authorize  func(*http.Request)
		reject     error
		wantStatus int
	}{
		{
			name:       "anonymous",
			authorize:  func(*http.Request) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "revoked token",
			authorize:  func(r *http.Request) { r.Header.Set("Authorization", "Bearer revoked") },
			reject:     frontendauth.ErrRevoked,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "expired token",
			authorize:  func(r *http.Request) { r.Header.Set("Authorization", "Bearer expired") },
			reject:     frontendauth.ErrExpired,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "scope withheld",
			authorize:  func(r *http.Request) { r.Header.Set("Authorization", "Bearer scopeless") },
			reject:     frontendauth.ErrForbidden,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, route := range routes {
		for _, probe := range probes {
			service.rejectAll = probe.reject
			request := route.request("{}")
			probe.authorize(request)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != probe.wantStatus {
				t.Errorf("%s %s with %s: status=%d, want=%d, body=%q",
					route.Method, route.Path, probe.name, recorder.Code, probe.wantStatus, recorder.Body.String())
			}
		}
	}
	service.rejectAll = nil
}

func TestEachScopedRouteDemandsItsDeclaredScope(t *testing.T) {
	// Fourteen routes behind one middleware is only safe if each one asks for
	// its own scope. A single wrong constant would hand a catalog-read frontend
	// the ability to trigger materializations, and no status code would show it.
	service := &scopedFrontendAPIClientService{
		principal: frontendauth.Principal{ClientID: "client-1", ProfileID: "profile-1"},
	}
	router := buildFrontendAPIRouter(t, service)

	for _, route := range frontendAPIV1Routes() {
		key := RouteKey{Method: route.Method, Path: "/api" + route.Pattern}
		service.required = nil
		request := key.request("{}")
		request.Header.Set("Authorization", "Bearer scopeless")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s %s: status=%d, want 403 for a client holding no scopes", key.Method, key.Path, recorder.Code)
			continue
		}
		if !strings.Contains(recorder.Body.String(), "insufficient_scope") {
			t.Errorf("%s %s: body=%q, want an insufficient_scope code", key.Method, key.Path, recorder.Body.String())
		}
		if len(service.required) != 1 || service.required[0] != route.Scope {
			t.Errorf("%s %s: demanded %v, want exactly [%s]", key.Method, key.Path, service.required, route.Scope)
		}
	}
}

func TestScopeAssignmentsAreWhatTheyClaimToBe(t *testing.T) {
	// TestEachScopedRouteDemandsItsDeclaredScope reads its expectation from the
	// same table it checks, so it proves the wiring but not the assignment. This
	// pins the assignments independently. The one that matters most is that
	// preparation is not reachable with a read scope: materialization spends the
	// server's disk, and a read-only frontend must not be able to trigger it.
	expected := map[string]frontendauth.Scope{
		"GET /frontend/v1/games":                                      frontendauth.ScopeCatalogRead,
		"GET /frontend/v1/games/{id}/detail":                          frontendauth.ScopeCatalogRead,
		"GET /frontend/v1/catalog/offers":                             frontendauth.ScopeCatalogRead,
		"GET /frontend/v1/media/{assetID}":                            frontendauth.ScopeMetadataRead,
		"HEAD /frontend/v1/media/{assetID}":                           frontendauth.ScopeMetadataRead,
		"GET /frontend/v1/content/copies/{copy_id}/manifest":          frontendauth.ScopeContentRead,
		"GET /frontend/v1/content/copies/{copy_id}/files/{file_id}":   frontendauth.ScopeContentRead,
		"HEAD /frontend/v1/content/copies/{copy_id}/files/{file_id}":  frontendauth.ScopeContentRead,
		"POST /frontend/v1/content/copies/{copy_id}/materializations": frontendauth.ScopeContentPrepare,
		"POST /frontend/v1/content/materializations/{job_id}/cancel":  frontendauth.ScopeContentPrepare,
	}
	actual := map[string]frontendauth.Scope{}
	for _, route := range frontendAPIV1Routes() {
		actual[route.Method+" "+route.Pattern] = route.Scope
		if route.Scope == "" {
			t.Errorf("%s %s has no scope and would be reachable by any valid token", route.Method, route.Pattern)
		}
	}
	for key, scope := range expected {
		if actual[key] != scope {
			t.Errorf("%s requires %q, want %q", key, actual[key], scope)
		}
	}
}

func TestAnotherProfileCannotBeNamedOnAScopedRoute(t *testing.T) {
	// Profile scope is derived from the token. Naming a different profile in the
	// header or the query must not widen it, and must not be quietly ignored
	// either: a frontend that thinks it is reading profile-2 has to be told.
	service := &scopedFrontendAPIClientService{
		principal: frontendauth.Principal{ClientID: "client-1", ProfileID: "profile-1", Scopes: frontendauth.AllScopes()},
	}
	router := buildFrontendAPIRouter(t, service)

	for _, target := range []string{
		"/api/frontend/v1/capabilities?profile_id=profile-2",
		"/api/frontend/v1/games?profile_id=profile-2",
		"/api/frontend/v1/media/asset-1?profile_id=profile-2",
		"/api/frontend/v1/content/copies/copy-1/manifest?profile_id=profile-2",
	} {
		for _, header := range []bool{false, true} {
			request := httptest.NewRequest(http.MethodGet, target, nil)
			request.Header.Set("Authorization", "Bearer valid")
			if header {
				request = httptest.NewRequest(http.MethodGet, strings.Split(target, "?")[0], nil)
				request.Header.Set("Authorization", "Bearer valid")
				request.Header.Set(profileHeader, "profile-2")
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Errorf("%s (header=%v): status=%d, want 403", target, header, recorder.Code)
			}
		}
	}
}

func TestCapabilitiesReportOnlyWhatTheClientCanReach(t *testing.T) {
	// The defect that motivated this ticket: the endpoint advertised five
	// features the token could not use. Discovery must now describe the mounted
	// surface, split by what this client's scopes actually unlock.
	service := &scopedFrontendAPIClientService{
		principal: frontendauth.Principal{
			ClientID: "client-1", ProfileID: "profile-1",
			Scopes: []frontendauth.Scope{frontendauth.ScopeCatalogRead},
		},
	}
	router := buildFrontendAPIRouter(t, service)

	request := httptest.NewRequest(http.MethodGet, "/api/frontend/v1/capabilities", nil)
	request.Header.Set("Authorization", "Bearer valid")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("capabilities status=%d body=%q", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Features    []FrontendAPIFeature `json:"features"`
		Unavailable []FrontendAPIFeature `json:"unavailable_features"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	available := map[string]FrontendAPIFeature{}
	for _, feature := range response.Features {
		available[feature.Name] = feature
	}
	if _, ok := available[featureCapabilityDiscovery]; !ok {
		t.Error("capability discovery must always be reachable")
	}
	catalog, ok := available[featureCatalogProjection]
	if !ok {
		t.Fatalf("catalog.read client cannot see catalog projection: %+v", response.Features)
	}
	if len(catalog.Endpoints) == 0 || catalog.Scope != frontendauth.ScopeCatalogRead {
		t.Errorf("catalog feature is not actionable: %+v", catalog)
	}

	withheld := map[string]frontendauth.Scope{}
	for _, feature := range response.Unavailable {
		withheld[feature.Name] = feature.Scope
	}
	for name, scope := range map[string]frontendauth.Scope{
		featureMetadataMedia:    frontendauth.ScopeMetadataRead,
		featureContentDelivery:  frontendauth.ScopeContentRead,
		featureCachePreparation: frontendauth.ScopeContentPrepare,
	} {
		if withheld[name] != scope {
			t.Errorf("%s should be listed as needing %s, got %q", name, scope, withheld[name])
		}
	}
	if _, leaked := available[featureContentDelivery]; leaked {
		t.Error("content delivery was advertised to a client that cannot reach it")
	}
	// The retired advertisement must be gone rather than merely unreachable.
	if strings.Contains(recorder.Body.String(), "runtime-artifacts") {
		t.Error("capabilities still advertise runtime artifacts, which the scope model does not cover")
	}
}

func TestAdvertisedEndpointsAreTheMountedOnes(t *testing.T) {
	// Discovery and reality drifting apart is exactly what this ticket fixed, so
	// the two are compared directly rather than trusted to stay in step.
	service := &scopedFrontendAPIClientService{
		principal: frontendauth.Principal{ClientID: "client-1", ProfileID: "profile-1", Scopes: frontendauth.AllScopes()},
	}
	router := buildFrontendAPIRouter(t, service)

	mounted := map[string]bool{}
	for _, route := range frontendAPIPaths(t, router) {
		mounted[route.Path] = true
	}

	advertised := map[string]bool{}
	for _, feature := range frontendAPIFeatures(frontendAPIV1Routes()) {
		for _, endpoint := range feature.Endpoints {
			advertised[endpoint] = true
			if !mounted[endpoint] {
				t.Errorf("capabilities advertise %s, which the router does not serve", endpoint)
			}
		}
	}
	for path := range mounted {
		if !advertised[path] {
			t.Errorf("router serves %s, which capability discovery never mentions", path)
		}
	}
}

func TestDiscoveryRouterMountsTheSameScopedPaths(t *testing.T) {
	// The nil-builder router generates the committed OpenAPI document. If it
	// drifts from the live router the published contract describes a server that
	// does not exist, which is worse than publishing nothing.
	service := &scopedFrontendAPIClientService{
		principal: frontendauth.Principal{ClientID: "client-1", ProfileID: "profile-1", Scopes: frontendauth.AllScopes()},
	}
	live := map[RouteKey]bool{}
	for _, route := range frontendAPIPaths(t, buildFrontendAPIRouter(t, service)) {
		live[route] = true
	}
	discovery := map[RouteKey]bool{}
	for _, route := range frontendAPIPaths(t, BuildRouter(nil, 0, "")) {
		discovery[route] = true
	}

	for route := range live {
		if !discovery[route] {
			t.Errorf("%s %s is served but missing from OpenAPI discovery", route.Method, route.Path)
		}
	}
	for route := range discovery {
		if !live[route] {
			t.Errorf("%s %s is in OpenAPI discovery but not served", route.Method, route.Path)
		}
	}
}

func TestAFrontendTokenNeverSatisfiesAdminAuthorization(t *testing.T) {
	// Shared handlers are safe to reuse only because RequireAdminProfile reads
	// the session-access context, which the bearer path never sets. That is a
	// load-bearing property of the design, so it is asserted rather than assumed.
	profile := frontendTestProfile()
	service := &scopedFrontendAPIClientService{
		principal: frontendauth.Principal{ClientID: "client-1", ProfileID: profile.ID, Scopes: frontendauth.AllScopes()},
	}
	handler := RequireFrontendAPIClient(service, lanProfileRepository{profile: profile})(
		RequireAdminProfile(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("an administrator handler ran for a frontend API client")
		})))

	request := httptest.NewRequest(http.MethodGet, "/api/frontend/v1/games", nil)
	request.Header.Set("Authorization", "Bearer valid")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401 for an admin route reached with a frontend token", recorder.Code)
	}
}

// buildFrontendAPIRouterWithContent is the same router with a content service
// that serves real bytes, so the delivery assertions below exercise the whole
// path a frontend takes rather than a stub that returns a status code.
func buildFrontendAPIRouterWithContent(t *testing.T, service FrontendAPIClientService, content ContentDeliveryService) chi.Router {
	t.Helper()
	clientController, err := NewFrontendAPIClientController(service, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	contentController, err := NewContentController(content, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	return BuildRouter(&RouteBuilder{
		GameCtrl: &GameController{}, CatalogCtrl: &CatalogController{}, ContentCtrl: contentController,
		MediaCtrl: &MediaController{}, DiscoCtrl: &DiscoveryController{}, AboutCtrl: &AboutController{},
		ConfigCtrl: &ConfigController{}, PluginCtrl: &PluginController{}, ReviewCtrl: &ReviewController{},
		AchievementCtrl: &AchievementController{}, SyncCtrl: &SyncController{}, UpdateCtrl: &UpdateController{},
		SaveSyncCtrl: &SaveSyncController{}, CacheCtrl: &CacheController{}, SSECtrl: &SSEController{},
		OAuthCtrl: &OAuthController{}, ProfileCtrl: &ProfileController{}, AuthCtrl: &AuthController{},
		FrontendAPIClientCtrl: clientController, FrontendAPIClientSvc: service,
		ProfileRepo: lanProfileRepository{profile: frontendTestProfile()},
	}, 0, "")
}

func TestContentIsDeliveredOverABearerTokenExactlyAsOverASession(t *testing.T) {
	// A frontend downloads a large game over a slow link and resumes, so Range,
	// HEAD and ETag are the difference between a usable integration and one that
	// restarts a 40GB transfer. They belong to http.ServeContent, but only if the
	// scoped route reaches the handler with the profile resolved — which is the
	// part this ticket added and therefore the part worth proving with bytes.
	fileID := contentdelivery.FileID("copy-1", "folder/game.bin")
	path := filepath.Join(t.TempDir(), "game.bin")
	if err := os.WriteFile(path, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	openService := func(t *testing.T) *fakeContentDeliveryService {
		t.Helper()
		opened, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		// Refused requests never reach the handler that closes this, and Windows
		// will not remove the temp directory while a handle is open.
		t.Cleanup(func() { _ = opened.Close() })
		return &fakeContentDeliveryService{opened: &contentdelivery.OpenFileResult{
			Reader: opened,
			File:   contentdelivery.ManifestFile{ID: fileID, RelativePath: "folder/game.bin", Name: "game.bin", Length: 6, ETag: `"file-revision"`},
			Name:   "game.bin", Size: 6, ModTime: time.Unix(1_700_000_000, 0),
		}}
	}

	client := &scopedFrontendAPIClientService{principal: frontendauth.Principal{
		ClientID: "client-1", ProfileID: "profile-1",
		Scopes: []frontendauth.Scope{frontendauth.ScopeContentRead},
	}}
	target := "/api/frontend/v1/content/copies/copy-1/files/" + fileID

	for _, test := range []struct {
		name       string
		method     string
		rangeValue string
		wantStatus int
		wantBody   string
	}{
		{name: "whole file", method: http.MethodGet, wantStatus: http.StatusOK, wantBody: "abcdef"},
		{name: "resume from the middle", method: http.MethodGet, rangeValue: "bytes=1-3", wantStatus: http.StatusPartialContent, wantBody: "bcd"},
		{name: "head", method: http.MethodHead, wantStatus: http.StatusOK},
		{name: "multiple ranges refused", method: http.MethodGet, rangeValue: "bytes=0-1,3-4", wantStatus: http.StatusRequestedRangeNotSatisfiable},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := buildFrontendAPIRouterWithContent(t, client, openService(t))
			request := httptest.NewRequest(test.method, target, nil)
			request.Header.Set("Authorization", "Bearer valid")
			if test.rangeValue != "" {
				request.Header.Set("Range", test.rangeValue)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%q", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if test.wantBody != "" && recorder.Body.String() != test.wantBody {
				t.Fatalf("body=%q want=%q", recorder.Body.String(), test.wantBody)
			}
			if test.wantStatus == http.StatusRequestedRangeNotSatisfiable {
				return
			}
			if recorder.Header().Get("ETag") != `"file-revision"` {
				t.Errorf("missing revision ETag: %v", recorder.Header())
			}
			if recorder.Header().Get("Accept-Ranges") != "bytes" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Errorf("unsafe or missing headers: %v", recorder.Header())
			}
		})
	}

	// The same bytes must stay unreachable without the scope.
	t.Run("without content.read", func(t *testing.T) {
		withheld := &scopedFrontendAPIClientService{principal: frontendauth.Principal{
			ClientID: "client-2", ProfileID: "profile-1",
			Scopes: []frontendauth.Scope{frontendauth.ScopeCatalogRead},
		}}
		router := buildFrontendAPIRouterWithContent(t, withheld, openService(t))
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Authorization", "Bearer valid")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden || recorder.Body.String() == "abcdef" {
			t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
		}
	})
}
