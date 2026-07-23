package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/auth"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

// TestEveryNonPublicAPIRouteFailsClosedForInvalidProfileSessions is the
// route-policy inventory. Adding a route requires either placing it behind the
// profile/device policy or explicitly adding it to this reviewed public list.
func TestEveryNonPublicAPIRouteFailsClosedForInvalidProfileSessions(t *testing.T) {
	profile := &core.Profile{ID: "protected-admin", DisplayName: "Protected", Role: core.ProfileRoleAdminPlayer}
	otherProfile := &core.Profile{ID: "other-player", DisplayName: "Other", Role: core.ProfileRolePlayer}
	profiles := multiLANProfileRepository{profiles: map[string]*core.Profile{
		profile.ID:      profile,
		otherProfile.ID: otherProfile,
	}}
	store := newLANAuthStore()
	authService, err := auth.NewService(store, profiles)
	if err != nil {
		t.Fatal(err)
	}
	if err := authService.InitializeCredential(context.Background(), profile.ID, "test", auth.CredentialPassword); err != nil {
		t.Fatal(err)
	}
	if err := authService.InitializeCredential(context.Background(), otherProfile.ID, "test", auth.CredentialPassword); err != nil {
		t.Fatal(err)
	}
	wrongProfileToken, _, err := authService.Login(context.Background(), otherProfile.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	expiredToken, expiredSession, err := authService.Login(context.Background(), profile.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	for tokenHash, session := range store.sessions {
		if session.ID == expiredSession.ID {
			session.ExpiresAt = time.Now().Add(-time.Minute)
			store.sessions[tokenHash] = session
		}
	}
	deletedToken, _, err := authService.Login(context.Background(), profile.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := authService.Logout(context.Background(), deletedToken); err != nil {
		t.Fatal(err)
	}

	router := BuildRouter(&RouteBuilder{
		GameCtrl: &GameController{}, MediaCtrl: &MediaController{}, DiscoCtrl: &DiscoveryController{}, AboutCtrl: &AboutController{},
		ConfigCtrl: &ConfigController{}, PluginCtrl: &PluginController{}, IntegrationRefreshCtrl: &IntegrationRefreshController{},
		ReviewCtrl: &ReviewController{}, AchievementCtrl: &AchievementController{}, AchievementRefreshCtrl: &AchievementRefreshController{},
		SyncCtrl: &SyncController{}, UpdateCtrl: &UpdateController{}, SaveSyncCtrl: &SaveSyncController{}, CacheCtrl: &CacheController{},
		SSECtrl: &SSEController{}, OAuthCtrl: &OAuthController{}, ProfileCtrl: &ProfileController{}, AuthCtrl: &AuthController{},
		ProfileRepo: profiles, AuthService: authService, DeviceCtrl: &DeviceController{},
	}, 0, "")

	public := map[string]string{
		"GET /health":                              "liveness contains no user data",
		"GET /auth/google/callback/{plugin_id}":    "provider redirect validated by expiring opaque OAuth state",
		"GET /auth/xbox/callback":                  "provider redirect validated by expiring opaque OAuth state",
		"GET /api/auth/session":                    "session bootstrap",
		"POST /api/auth/login":                     "credential exchange",
		"POST /api/auth/logout":                    "session teardown",
		"PUT /api/auth/credential":                 "must-change credential replacement",
		"GET /api/auth/credential":                 "credential bootstrap status",
		"POST /api/auth/credential":                "create-only initial credential bootstrap",
		"DELETE /api/auth/credential":              "self-authenticated credential removal",
		"POST /api/devices/pair":                   "opaque pairing capability",
		"POST /api/devices/client-launches/redeem": "opaque one-time launch capability",
		"GET /api/devices/connect":                 "client protocol bootstrap",
		"GET /api/devices/client-download":         "public client download metadata",
		"GET /api/devices/client-installer":        "public signed installer download",
		"HEAD /api/devices/client-installer":       "public signed installer metadata",
		"GET /api/device-transfers/archive":        "opaque transfer capability",
		"HEAD /api/device-transfers/archive":       "opaque transfer capability metadata",
		"GET /api/device-transfers/content":        "opaque transfer capability",
		"HEAD /api/device-transfers/content":       "opaque transfer capability metadata",
		"GET /api/device-transfers/save-domain":    "opaque transfer capability",
		"PUT /api/device-transfers/save-domain":    "opaque transfer capability",
		"GET /api/setup/status":                    "first-run bootstrap",
		"POST /api/setup/start-fresh":              "first-run bootstrap guarded by setup state",
		"POST /api/setup/restore-sync/check":       "first-run restore bootstrap",
		"POST /api/setup/restore-sync/browse":      "first-run restore bootstrap",
		"POST /api/setup/restore-sync/points":      "first-run restore bootstrap",
		"POST /api/setup/restore-sync":             "first-run restore bootstrap",
		"GET /api/profiles":                        "profile picker exposes only safe identity fields",
		"GET /api/auth/callback/{plugin_id}":       "provider redirect validated by expiring opaque OAuth state",
		"POST /api/auth/callback/import":           "provider callback validated by expiring opaque OAuth state",
		"GET /api/about":                           "public product/version metadata",
		"GET /api/about/license":                   "public license text",
	}
	parameter := regexp.MustCompile(`\{[^}]+\}`)
	probes := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{name: "anonymous", wantStatus: http.StatusUnauthorized},
		{name: "wrong-profile", token: wrongProfileToken, wantStatus: http.StatusForbidden},
		{name: "expired-session", token: expiredToken, wantStatus: http.StatusUnauthorized},
		{name: "deleted-session", token: deletedToken, wantStatus: http.StatusUnauthorized},
	}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		key := method + " " + route
		if _, ok := public[key]; ok || route == "/*" || route == "/" {
			return nil
		}
		if len(route) < 5 || route[:5] != "/api/" {
			return nil
		}
		for _, probe := range probes {
			request := httptest.NewRequest(method, parameter.ReplaceAllString(route, "test-id"), nil)
			request.Header.Set("X-MGA-Profile-ID", profile.ID)
			if probe.token != "" {
				request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: probe.token})
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != probe.wantStatus {
				t.Errorf("%s lacks a fail-closed profile/device policy: %s status=%d, want=%d, body=%q",
					key, probe.name, response.Code, probe.wantStatus, response.Body.String())
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
