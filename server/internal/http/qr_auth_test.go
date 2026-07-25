package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/go-chi/chi/v5"
)

// fakeQRPluginHost returns canned auth.qr.* replies and records what it got.
type fakeQRPluginHost struct {
	beginResult map[string]any
	pollResult  map[string]any
	callErr     error
	lastMethod  string
	lastParams  map[string]any
}

func (f *fakeQRPluginHost) Discover(context.Context) error { return nil }
func (f *fakeQRPluginHost) Call(_ context.Context, _, method string, params any, result any) error {
	f.lastMethod = method
	f.lastParams, _ = params.(map[string]any)
	if f.callErr != nil {
		return f.callErr
	}
	payload := f.beginResult
	if method == qrPollMethod {
		payload = f.pollResult
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}
func (f *fakeQRPluginHost) Close() error                          { return nil }
func (f *fakeQRPluginHost) GetPluginIDs() []string                { return nil }
func (f *fakeQRPluginHost) GetPlugin(string) (*core.Plugin, bool) { return nil, false }
func (f *fakeQRPluginHost) ListPlugins() []plugins.PluginInfo     { return nil }
func (f *fakeQRPluginHost) GetPluginIDsProviding(string) []string { return nil }

// qrIntegrationRepo is a minimal profile-scoped integration repository.
type qrIntegrationRepo struct {
	integration *core.Integration
	updated     *core.Integration
}

func (r *qrIntegrationRepo) Create(context.Context, *core.Integration) error { return nil }
func (r *qrIntegrationRepo) Update(_ context.Context, integration *core.Integration) error {
	r.updated = integration
	return nil
}
func (r *qrIntegrationRepo) Delete(context.Context, string) error { return nil }
func (r *qrIntegrationRepo) List(context.Context) ([]*core.Integration, error) {
	return []*core.Integration{r.integration}, nil
}
func (r *qrIntegrationRepo) GetByID(_ context.Context, id string) (*core.Integration, error) {
	if r.integration != nil && r.integration.ID == id {
		return r.integration, nil
	}
	return nil, nil
}
func (r *qrIntegrationRepo) ListByPluginID(context.Context, string) ([]*core.Integration, error) {
	return nil, nil
}

func newQRController(host *fakeQRPluginHost, repo core.IntegrationRepository) *OAuthController {
	return NewOAuthController(host, restoreConfig{}, &noopLogger{}, nil, repo)
}

// qrRequest issues a QR endpoint request with chi URL params and profile scope.
func qrRequest(t *testing.T, handler http.HandlerFunc, pluginID, profileID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/qr/"+pluginID+"/poll", strings.NewReader(body))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("plugin_id", pluginID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	if profileID != "" {
		ctx = core.WithProfile(ctx, &core.Profile{ID: profileID})
	}
	rec := httptest.NewRecorder()
	handler(rec, req.WithContext(ctx))
	return rec
}

func steamQRIntegration() *core.Integration {
	return &core.Integration{
		ID:         "int-1",
		ProfileID:  "profile-1",
		PluginID:   "game-source-steam",
		ConfigJSON: `{"api_key":"key","steam_id":"76561198000000001"}`,
	}
}

func TestQRBeginReturnsChallenge(t *testing.T) {
	host := &fakeQRPluginHost{beginResult: map[string]any{
		"status": "pending", "client_id": "c1", "request_id": "r1",
		"challenge_url": "https://s.team/q/abc", "interval_seconds": 5,
	}}
	ctrl := newQRController(host, &qrIntegrationRepo{integration: steamQRIntegration()})

	rec := qrRequest(t, ctrl.QRBegin, "game-source-steam", "profile-1", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var got qrBeginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ChallengeURL != "https://s.team/q/abc" || got.ClientID != "c1" || got.RequestID != "r1" {
		t.Fatalf("challenge = %+v", got)
	}
	if host.lastMethod != qrBeginMethod {
		t.Fatalf("plugin method = %q, want %q", host.lastMethod, qrBeginMethod)
	}
}

func TestQRBeginRequiresProfileAndChallenge(t *testing.T) {
	host := &fakeQRPluginHost{beginResult: map[string]any{"status": "pending"}}
	ctrl := newQRController(host, &qrIntegrationRepo{})

	// No profile scope.
	if rec := qrRequest(t, ctrl.QRBegin, "game-source-steam", "", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status without profile = %d, want 400", rec.Code)
	}
	// Provider returned no challenge URL.
	if rec := qrRequest(t, ctrl.QRBegin, "game-source-steam", "profile-1", `{}`); rec.Code != http.StatusBadGateway {
		t.Fatalf("status without challenge = %d, want 502", rec.Code)
	}
}

func TestQRPollPendingDoesNotPersist(t *testing.T) {
	repo := &qrIntegrationRepo{integration: steamQRIntegration()}
	host := &fakeQRPluginHost{pollResult: map[string]any{"status": "pending"}}
	ctrl := newQRController(host, repo)

	rec := qrRequest(t, ctrl.QRPoll, "game-source-steam", "profile-1",
		`{"integration_id":"int-1","client_id":"c1","request_id":"r1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"pending"`) {
		t.Fatalf("body = %s, want pending", rec.Body.String())
	}
	if repo.updated != nil {
		t.Fatal("a pending poll must not write credentials")
	}
}

func TestQRPollApprovedPersistsRefreshToken(t *testing.T) {
	repo := &qrIntegrationRepo{integration: steamQRIntegration()}
	host := &fakeQRPluginHost{pollResult: map[string]any{
		"status": "ok", "account_name": "orr",
		"config_updates": map[string]any{"refresh_token": "refresh-abc"},
	}}
	ctrl := newQRController(host, repo)

	rec := qrRequest(t, ctrl.QRPoll, "game-source-steam", "profile-1",
		`{"integration_id":"int-1","client_id":"c1","request_id":"r1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if repo.updated == nil {
		t.Fatal("approved poll did not save the connection")
	}

	var saved map[string]any
	if err := json.Unmarshal([]byte(repo.updated.ConfigJSON), &saved); err != nil {
		t.Fatal(err)
	}
	if saved["refresh_token"] != "refresh-abc" {
		t.Fatalf("saved refresh_token = %#v", saved["refresh_token"])
	}
	// Existing configuration must survive the merge.
	if saved["api_key"] != "key" || saved["steam_id"] != "76561198000000001" {
		t.Fatalf("existing config was lost: %#v", saved)
	}
	if repo.updated.NeedsReauth {
		t.Fatal("needs_reauth should be cleared after a successful sign-in")
	}
	// The known Steam identity is forwarded so the plugin can complete the set.
	if host.lastParams["steam_id"] != "76561198000000001" {
		t.Fatalf("steam_id was not forwarded to the plugin: %#v", host.lastParams)
	}
}

func TestQRPollRejectsAnotherProfilesConnection(t *testing.T) {
	repo := &qrIntegrationRepo{integration: steamQRIntegration()}
	host := &fakeQRPluginHost{pollResult: map[string]any{
		"status": "ok", "config_updates": map[string]any{"refresh_token": "stolen"},
	}}
	ctrl := newQRController(host, repo)

	// profile-2 must not be able to write profile-1's connection.
	rec := qrRequest(t, ctrl.QRPoll, "game-source-steam", "profile-2",
		`{"integration_id":"int-1","client_id":"c1","request_id":"r1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a foreign connection", rec.Code)
	}
	if repo.updated != nil {
		t.Fatal("a foreign profile wrote credentials")
	}
}

func TestQRPollRejectsPluginMismatchAndMissingFields(t *testing.T) {
	repo := &qrIntegrationRepo{integration: steamQRIntegration()}
	host := &fakeQRPluginHost{pollResult: map[string]any{"status": "pending"}}
	ctrl := newQRController(host, repo)

	// Connection belongs to a different plugin.
	if rec := qrRequest(t, ctrl.QRPoll, "game-source-epic", "profile-1",
		`{"integration_id":"int-1","client_id":"c1","request_id":"r1"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("plugin mismatch status = %d, want 400", rec.Code)
	}
	// Missing integration_id.
	if rec := qrRequest(t, ctrl.QRPoll, "game-source-steam", "profile-1",
		`{"client_id":"c1","request_id":"r1"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing integration_id status = %d, want 400", rec.Code)
	}
	// Missing session identity.
	if rec := qrRequest(t, ctrl.QRPoll, "game-source-steam", "profile-1",
		`{"integration_id":"int-1"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing session status = %d, want 400", rec.Code)
	}
}
