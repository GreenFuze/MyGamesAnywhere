package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
)

func TestOAuthControllerImportGoogleCallbackCompletesAndPersistsUpdates(t *testing.T) {
	repo := &fakeControllerIntegrationRepo{
		byID: map[string]*core.Integration{
			"int-google": {
				ID:         "int-google",
				PluginID:   "sync-settings-google-drive",
				Label:      "Google Sync",
				ConfigJSON: `{"path":"Games/mga_sync"}`,
			},
		},
	}
	host := &fakeOAuthCallbackPluginHost{
		results: map[string]oauthCallbackResult{
			"sync-settings-google-drive": {
				Status:        "ok",
				ConfigUpdates: map[string]any{"refresh_token": "stored-refresh"},
			},
		},
	}
	controller := NewOAuthController(host, staticConfig{values: map[string]string{"PORT": "8900", "LISTEN_IP": "127.0.0.1"}}, noopLogger{}, nil, repo)
	controller.states = NewOAuthStateStore()
	controller.states.Register("state-google", OAuthState{IntegrationID: "int-google", PluginID: "sync-settings-google-drive"})

	rec := httptest.NewRecorder()
	req := importCallbackRequest(t, "sync-settings-google-drive", "http://127.0.0.1:8900/auth/google/callback/sync-settings-google-drive?code=abc&state=state-google")
	controller.ImportCallback(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if repo.updated == nil || !strings.Contains(repo.updated.ConfigJSON, "stored-refresh") {
		t.Fatalf("updated config = %#v, want persisted refresh token", repo.updated)
	}
	if got := host.lastPayload["redirect_uri"]; got != "http://127.0.0.1:8900/auth/google/callback/sync-settings-google-drive" {
		t.Fatalf("redirect_uri = %v", got)
	}
}

func TestOAuthControllerImportXboxCallbackPathVariants(t *testing.T) {
	for _, callbackURL := range []string{
		"http://127.0.0.1:8900/api/auth/callback/game-source-xbox?code=abc&state=state-xbox",
		"http://127.0.0.1:8900/auth/xbox/callback?code=abc&state=state-xbox",
	} {
		t.Run(callbackURL, func(t *testing.T) {
			repo := &fakeControllerIntegrationRepo{
				byID: map[string]*core.Integration{
					"int-xbox": {ID: "int-xbox", PluginID: "game-source-xbox", ConfigJSON: `{}`},
				},
			}
			host := &fakeOAuthCallbackPluginHost{
				results: map[string]oauthCallbackResult{
					"game-source-xbox": {Status: "ok", ConfigUpdates: map[string]any{"xuid": "123"}},
				},
			}
			controller := NewOAuthController(host, staticConfig{values: map[string]string{"PORT": "8900", "LISTEN_IP": "127.0.0.1"}}, noopLogger{}, nil, repo)
			controller.states = NewOAuthStateStore()
			controller.states.Register("state-xbox", OAuthState{IntegrationID: "int-xbox", PluginID: "game-source-xbox"})

			rec := httptest.NewRecorder()
			controller.ImportCallback(rec, importCallbackRequest(t, "game-source-xbox", callbackURL))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestOAuthControllerImportSteamOpenIDPreservesParams(t *testing.T) {
	repo := &fakeControllerIntegrationRepo{
		byID: map[string]*core.Integration{
			"int-steam": {ID: "int-steam", PluginID: "game-source-steam", ConfigJSON: `{}`},
		},
	}
	host := &fakeOAuthCallbackPluginHost{
		results: map[string]oauthCallbackResult{
			"game-source-steam": {Status: "ok", ConfigUpdates: map[string]any{"steam_id": "7656119"}},
		},
	}
	controller := NewOAuthController(host, staticConfig{values: map[string]string{"PORT": "8900", "LISTEN_IP": "127.0.0.1"}}, noopLogger{}, nil, repo)
	controller.states = NewOAuthStateStore()
	controller.states.Register("state-steam", OAuthState{IntegrationID: "int-steam", PluginID: "game-source-steam"})

	callbackURL := "http://127.0.0.1:8900/api/auth/callback/game-source-steam?state=state-steam&openid.mode=id_res&openid.claimed_id=https%3A%2F%2Fsteamcommunity.com%2Fopenid%2Fid%2F7656119"
	rec := httptest.NewRecorder()
	controller.ImportCallback(rec, importCallbackRequest(t, "game-source-steam", callbackURL))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	payloadParams, ok := host.lastPayload["params"].(map[string]string)
	if !ok {
		t.Fatalf("params payload = %#v, want map", host.lastPayload["params"])
	}
	if got := payloadParams["openid.claimed_id"]; !strings.Contains(got, "7656119") {
		t.Fatalf("openid.claimed_id = %q", got)
	}
}

func TestOAuthControllerImportDraftCallbackWithPluginOwnedState(t *testing.T) {
	host := &fakeOAuthCallbackPluginHost{
		results: map[string]oauthCallbackResult{
			"plugin.oauth": {Status: "ok", ConfigUpdates: map[string]any{"token": "cached-by-plugin"}},
		},
	}
	controller := NewOAuthController(host, staticConfig{values: map[string]string{"PORT": "8900", "LISTEN_IP": "127.0.0.1"}}, noopLogger{}, nil)
	controller.states = NewOAuthStateStore()
	controller.states.Register("state-draft", OAuthState{PluginID: "plugin.oauth"})

	rec := httptest.NewRecorder()
	controller.ImportCallback(rec, importCallbackRequest(t, "plugin.oauth", "http://127.0.0.1:8900/api/auth/callback/plugin.oauth?code=abc&state=state-draft"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestOAuthControllerImportRejectsBadCallbacks(t *testing.T) {
	controller := NewOAuthController(&fakeOAuthCallbackPluginHost{}, staticConfig{values: map[string]string{"PORT": "8900", "LISTEN_IP": "127.0.0.1"}}, noopLogger{}, nil, &fakeControllerIntegrationRepo{})
	controller.states = NewOAuthStateStore()
	controller.states.Register("known", OAuthState{PluginID: "plugin.oauth"})

	cases := []struct {
		name        string
		pluginID    string
		callbackURL string
	}{
		{name: "malformed", pluginID: "plugin.oauth", callbackURL: ":://bad"},
		{name: "wrong path", pluginID: "plugin.oauth", callbackURL: "http://127.0.0.1:8900/not/oauth?code=abc&state=known"},
		{name: "wrong plugin", pluginID: "plugin.oauth", callbackURL: "http://127.0.0.1:8900/api/auth/callback/other?code=abc&state=known"},
		{name: "unknown state", pluginID: "plugin.oauth", callbackURL: "http://127.0.0.1:8900/api/auth/callback/plugin.oauth?code=abc&state=missing"},
		{name: "missing code", pluginID: "plugin.oauth", callbackURL: "http://127.0.0.1:8900/api/auth/callback/plugin.oauth?state=known"},
		{name: "provider error", pluginID: "plugin.oauth", callbackURL: "http://127.0.0.1:8900/api/auth/callback/plugin.oauth?error=access_denied&state=known"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			controller.ImportCallback(rec, importCallbackRequest(t, tc.pluginID, tc.callbackURL))
			if rec.Code < 400 {
				t.Fatalf("status = %d, want failure", rec.Code)
			}
		})
	}
}

func importCallbackRequest(t *testing.T, pluginID, callbackURL string) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"plugin_id":    pluginID,
		"callback_url": callbackURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/callback/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

type fakeOAuthCallbackPluginHost struct {
	results     map[string]oauthCallbackResult
	lastPayload map[string]any
}

func (f *fakeOAuthCallbackPluginHost) Discover(context.Context) error { panic("unexpected call") }
func (f *fakeOAuthCallbackPluginHost) Call(_ context.Context, pluginID, method string, params any, result any) error {
	if method != "auth.oauth.callback" {
		panic("unexpected call")
	}
	payload, _ := params.(map[string]any)
	f.lastPayload = payload
	callbackResult := f.results[pluginID]
	data, err := json.Marshal(callbackResult)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}
func (f *fakeOAuthCallbackPluginHost) Close() error { return nil }
func (f *fakeOAuthCallbackPluginHost) GetPluginIDs() []string {
	return nil
}
func (f *fakeOAuthCallbackPluginHost) GetPlugin(string) (*core.Plugin, bool) { return nil, false }
func (f *fakeOAuthCallbackPluginHost) ListPlugins() []plugins.PluginInfo     { return nil }
func (f *fakeOAuthCallbackPluginHost) GetPluginIDsProviding(string) []string {
	return nil
}
