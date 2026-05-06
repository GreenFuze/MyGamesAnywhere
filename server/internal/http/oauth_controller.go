package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	appconfig "github.com/GreenFuze/MyGamesAnywhere/server/internal/config"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/go-chi/chi/v5"
)

// OAuthController handles OAuth redirect callbacks for plugins that require
// browser-based consent flows (e.g. Xbox via Microsoft Entra).
type OAuthController struct {
	pluginHost plugins.PluginHost
	config     core.Configuration
	logger     core.Logger
	eventBus   *events.EventBus
	repo       core.IntegrationRepository
	states     *OAuthStateStore
}

func NewOAuthController(
	pluginHost plugins.PluginHost,
	config core.Configuration,
	logger core.Logger,
	eventBus *events.EventBus,
	repo ...core.IntegrationRepository,
) *OAuthController {
	var integrationRepo core.IntegrationRepository
	if len(repo) > 0 {
		integrationRepo = repo[0]
	}
	return &OAuthController{
		pluginHost: pluginHost,
		config:     config,
		logger:     logger,
		eventBus:   eventBus,
		repo:       integrationRepo,
		states:     defaultOAuthStateStore,
	}
}

var defaultOAuthStateStore = NewOAuthStateStore()

type OAuthState struct {
	IntegrationID string
	ProfileID     string
	PluginID      string
}

type OAuthStateStore struct {
	mu     sync.Mutex
	states map[string]OAuthState
}

func NewOAuthStateStore() *OAuthStateStore {
	return &OAuthStateStore{states: make(map[string]OAuthState)}
}

func (s *OAuthStateStore) Register(state string, value OAuthState) {
	if s == nil || strings.TrimSpace(state) == "" || (strings.TrimSpace(value.IntegrationID) == "" && strings.TrimSpace(value.PluginID) == "") {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = value
}

func (s *OAuthStateStore) Peek(state string) (OAuthState, bool) {
	if s == nil || strings.TrimSpace(state) == "" {
		return OAuthState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.states[state]
	return value, ok
}

func (s *OAuthStateStore) Consume(state string) (OAuthState, bool) {
	if s == nil || strings.TrimSpace(state) == "" {
		return OAuthState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.states[state]
	if ok {
		delete(s.states, state)
	}
	return value, ok
}

// Callback handles GET /api/auth/callback/{plugin_id}.
// Supports both OAuth2 (code+state) and OpenID 2.0 (openid.* params).
func (c *OAuthController) Callback(w http.ResponseWriter, r *http.Request) {
	pluginID := chi.URLParam(r, "plugin_id")
	if pluginID == "" {
		c.renderCallbackPage(w, false, "Missing plugin_id in callback URL")
		return
	}
	c.callbackForPlugin(w, r, pluginID)
}

// XboxCallback is a compatibility route registered in the Microsoft app.
// Keep /auth/xbox/callback active unless the Microsoft app registration changes.
func (c *OAuthController) XboxCallback(w http.ResponseWriter, r *http.Request) {
	c.callbackForPlugin(w, r, "game-source-xbox")
}

func (c *OAuthController) callbackForPlugin(w http.ResponseWriter, r *http.Request, pluginID string) {
	state := r.URL.Query().Get("state")

	// Route to the appropriate handler based on query params.
	if r.URL.Query().Get("openid.mode") != "" {
		c.handleOpenIDCallback(w, r, pluginID, state)
	} else {
		c.handleOAuth2Callback(w, r, pluginID, state)
	}
}

type oauthCallbackImportRequest struct {
	PluginID    string `json:"plugin_id"`
	CallbackURL string `json:"callback_url"`
}

type oauthCallbackImportResponse struct {
	Status string `json:"status"`
}

// ImportCallback lets a remote browser complete an OAuth/OpenID attempt by
// pasting the final provider callback URL when the loopback redirect cannot
// reach MGA automatically.
func (c *OAuthController) ImportCallback(w http.ResponseWriter, r *http.Request) {
	var body oauthCallbackImportRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	body.PluginID = strings.TrimSpace(body.PluginID)
	body.CallbackURL = strings.TrimSpace(body.CallbackURL)
	if body.PluginID == "" {
		http.Error(w, "plugin_id is required", http.StatusBadRequest)
		return
	}
	callbackURL, err := url.Parse(body.CallbackURL)
	if err != nil || callbackURL == nil || !callbackURL.IsAbs() {
		http.Error(w, "callback_url must be a full absolute URL copied from the browser address bar", http.StatusBadRequest)
		return
	}
	pluginID, err := pluginIDFromCallbackPath(callbackURL.EscapedPath())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if pluginID != body.PluginID {
		http.Error(w, fmt.Sprintf("callback URL is for plugin %q, not %q", pluginID, body.PluginID), http.StatusBadRequest)
		return
	}

	query := callbackURL.Query()
	state := strings.TrimSpace(query.Get("state"))
	if state == "" {
		http.Error(w, "callback URL is missing state", http.StatusBadRequest)
		return
	}
	if err := c.validateCallbackState(r.Context(), pluginID, state); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var completeErr error
	if query.Get("openid.mode") != "" {
		completeErr = c.importOpenIDCallback(r.Context(), pluginID, state, query)
	} else {
		completeErr = c.importOAuth2Callback(r.Context(), pluginID, state, query)
	}
	if completeErr != nil {
		c.publishOAuthError(pluginID, state, completeErr.Error())
		http.Error(w, completeErr.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(oauthCallbackImportResponse{Status: "ok"})
}

func pluginIDFromCallbackPath(escapedPath string) (string, error) {
	path, err := url.PathUnescape(escapedPath)
	if err != nil {
		return "", fmt.Errorf("callback URL path is invalid")
	}
	path = strings.TrimRight(path, "/")
	const apiPrefix = "/api/auth/callback/"
	const googlePrefix = "/auth/google/callback/"
	if strings.HasPrefix(path, apiPrefix) {
		return strings.TrimPrefix(path, apiPrefix), nil
	}
	if strings.HasPrefix(path, googlePrefix) {
		return strings.TrimPrefix(path, googlePrefix), nil
	}
	if path == "/auth/xbox/callback" {
		return "game-source-xbox", nil
	}
	return "", fmt.Errorf("callback URL path is not an MGA OAuth callback path")
}

func (c *OAuthController) validateCallbackState(ctx context.Context, pluginID, state string) error {
	oauthState, ok := c.states.Peek(state)
	if !ok {
		return fmt.Errorf("OAuth state is missing, expired, or was already used")
	}
	if strings.TrimSpace(oauthState.PluginID) != "" && oauthState.PluginID != pluginID {
		return fmt.Errorf("OAuth state plugin mismatch: got %s, want %s", pluginID, oauthState.PluginID)
	}
	if strings.TrimSpace(oauthState.IntegrationID) == "" || c.repo == nil {
		return nil
	}
	repoCtx := ctx
	if oauthState.ProfileID != "" {
		repoCtx = core.WithProfile(ctx, &core.Profile{ID: oauthState.ProfileID})
	}
	integration, err := c.repo.GetByID(repoCtx, oauthState.IntegrationID)
	if err != nil {
		return fmt.Errorf("load integration for OAuth state: %w", err)
	}
	if integration == nil {
		return fmt.Errorf("integration for OAuth state was not found")
	}
	if integration.PluginID != pluginID {
		return fmt.Errorf("OAuth state plugin mismatch: got %s, want %s", pluginID, integration.PluginID)
	}
	return nil
}

func (c *OAuthController) importOAuth2Callback(ctx context.Context, pluginID, state string, query url.Values) error {
	if errMsg := query.Get("error"); errMsg != "" {
		desc := strings.TrimSpace(query.Get("error_description"))
		if desc != "" {
			return fmt.Errorf("%s: %s", errMsg, desc)
		}
		return fmt.Errorf("%s", errMsg)
	}
	code := strings.TrimSpace(query.Get("code"))
	if code == "" {
		return fmt.Errorf("callback URL is missing code")
	}
	redirectURI, err := appconfig.OAuthCallbackURL(c.config, pluginID)
	if err != nil {
		return fmt.Errorf("server network configuration is invalid")
	}
	return c.completeCallback(ctx, pluginID, state, map[string]any{
		"code":         code,
		"state":        state,
		"redirect_uri": redirectURI,
	})
}

func (c *OAuthController) importOpenIDCallback(ctx context.Context, pluginID, state string, query url.Values) error {
	if query.Get("openid.mode") == "cancel" {
		return fmt.Errorf("user_cancelled")
	}
	params := make(map[string]string)
	for key, vals := range query {
		if len(vals) > 0 {
			params[key] = vals[0]
		}
	}
	if len(params) == 0 {
		return fmt.Errorf("callback URL is missing OpenID parameters")
	}
	return c.completeCallback(ctx, pluginID, state, map[string]any{
		"state":  state,
		"params": params,
	})
}

// handleOAuth2Callback processes standard OAuth2 callbacks (code + state).
func (c *OAuthController) handleOAuth2Callback(w http.ResponseWriter, r *http.Request, pluginID, state string) {
	code := r.URL.Query().Get("code")

	// Check for OAuth error response from the provider.
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		desc := r.URL.Query().Get("error_description")
		c.logger.Error("OAuth callback error", fmt.Errorf("%s: %s", errMsg, desc),
			"plugin_id", pluginID)
		c.publishErrorAndRender(w, pluginID, state, errMsg)
		return
	}

	if code == "" || state == "" {
		c.renderCallbackPage(w, false, "Missing code or state in callback")
		return
	}

	// Build the redirect_uri that was used (must match what was sent to the provider).
	redirectURI, err := appconfig.OAuthCallbackURL(c.config, pluginID)
	if err != nil {
		c.logger.Error("OAuth callback redirect URL", err, "plugin_id", pluginID)
		c.renderCallbackPage(w, false, "Server network configuration is invalid")
		return
	}

	if err := c.completeCallback(r.Context(), pluginID, state, map[string]any{
		"code":         code,
		"state":        state,
		"redirect_uri": redirectURI,
	}); err != nil {
		c.logger.Error("OAuth callback failed", err, "plugin_id", pluginID, "state", state)
		c.publishErrorAndRender(w, pluginID, state, err.Error())
		return
	}
	c.renderCallbackPage(w, true, "Authentication successful!")
}

// handleOpenIDCallback processes OpenID 2.0 callbacks (e.g. Steam).
func (c *OAuthController) handleOpenIDCallback(w http.ResponseWriter, r *http.Request, pluginID, state string) {
	// Handle user cancellation.
	if r.URL.Query().Get("openid.mode") == "cancel" {
		c.publishErrorAndRender(w, pluginID, state, "user_cancelled")
		return
	}

	if state == "" {
		c.renderCallbackPage(w, false, "Missing state in callback")
		return
	}

	// Collect all query params into a flat map for the plugin.
	params := make(map[string]string)
	for key, vals := range r.URL.Query() {
		if len(vals) > 0 {
			params[key] = vals[0]
		}
	}

	if err := c.completeCallback(r.Context(), pluginID, state, map[string]any{
		"state":  state,
		"params": params,
	}); err != nil {
		c.logger.Error("OpenID callback failed", err, "plugin_id", pluginID, "state", state)
		c.publishErrorAndRender(w, pluginID, state, err.Error())
		return
	}
	c.renderCallbackPage(w, true, "Authentication successful!")
}

// completeCallback calls the plugin's auth.oauth.callback IPC, persists any
// config updates, and publishes the common completion event used by callback
// redirects and pasted callback imports.
func (c *OAuthController) completeCallback(ctx context.Context, pluginID, state string, ipcPayload map[string]any) error {
	var result oauthCallbackResult
	err := c.pluginHost.Call(ctx, pluginID, "auth.oauth.callback", ipcPayload, &result)

	if err != nil {
		return fmt.Errorf("auth.oauth.callback IPC failed: %w", err)
	}

	if result.Status != "ok" {
		msg := result.Message
		if msg == "" {
			msg = result.Status
		}
		return fmt.Errorf("%s", msg)
	}
	if err := c.persistOAuthConfigUpdates(ctx, pluginID, state, result.ConfigUpdates); err != nil {
		return err
	}

	// Success — publish SSE event so the frontend wizard knows.
	events.PublishJSON(c.eventBus, "oauth_complete", map[string]any{
		"plugin_id": pluginID,
		"state":     state,
	})
	return nil
}

type oauthCallbackResult struct {
	Status        string         `json:"status"`
	Message       string         `json:"message"`
	ConfigUpdates map[string]any `json:"config_updates,omitempty"`
}

func (c *OAuthController) persistOAuthConfigUpdates(ctx context.Context, pluginID, state string, updates map[string]any) error {
	oauthState, ok := c.states.Consume(state)
	if !ok || len(updates) == 0 {
		return nil
	}
	if strings.TrimSpace(oauthState.IntegrationID) == "" {
		return nil
	}
	if c.repo == nil {
		return nil
	}

	repoCtx := ctx
	if oauthState.ProfileID != "" {
		repoCtx = core.WithProfile(ctx, &core.Profile{ID: oauthState.ProfileID})
	}
	integration, err := c.repo.GetByID(repoCtx, oauthState.IntegrationID)
	if err != nil {
		return fmt.Errorf("load integration for OAuth update: %w", err)
	}
	if integration == nil {
		return fmt.Errorf("integration for OAuth state was not found")
	}
	if integration.PluginID != pluginID {
		return fmt.Errorf("OAuth state plugin mismatch: got %s, want %s", pluginID, integration.PluginID)
	}
	configMap, err := decodeIntegrationConfig(integration.ConfigJSON)
	if err != nil {
		return fmt.Errorf("decode integration config: %w", err)
	}
	mergeConfigUpdates(configMap, updates)
	configBytes, err := json.Marshal(configMap)
	if err != nil {
		return fmt.Errorf("encode integration config: %w", err)
	}
	integration.ConfigJSON = string(configBytes)
	integration.UpdatedAt = time.Now()
	if err := c.repo.Update(repoCtx, integration); err != nil {
		return fmt.Errorf("save OAuth config update: %w", err)
	}
	return nil
}

// publishErrorAndRender publishes an oauth_error SSE event and renders the error page.
func (c *OAuthController) publishErrorAndRender(w http.ResponseWriter, pluginID, state, errMsg string) {
	c.publishOAuthError(pluginID, state, errMsg)
	c.renderCallbackPage(w, false, "Authentication failed: "+errMsg)
}

func (c *OAuthController) publishOAuthError(pluginID, state, errMsg string) {
	events.PublishJSON(c.eventBus, "oauth_error", map[string]any{
		"plugin_id": pluginID,
		"state":     state,
		"error":     errMsg,
	})
}

// renderCallbackPage writes a self-contained HTML page for the OAuth redirect tab.
func (c *OAuthController) renderCallbackPage(w http.ResponseWriter, success bool, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	icon := "&#10060;" // red X
	if success {
		icon = "&#9989;" // green check
	}

	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>MGA Authentication</title>
<style>
  body { font-family: system-ui, sans-serif; display: flex; justify-content: center;
         align-items: center; min-height: 100vh; margin: 0; background: #1a1a2e; color: #e0e0e0; }
  .card { text-align: center; padding: 2rem; border-radius: 12px; background: #16213e;
          border: 1px solid #0f3460; max-width: 400px; }
  .icon { font-size: 3rem; }
  p { color: #a0a0a0; }
</style></head>
<body><div class="card">
  <div class="icon">%s</div>
  <h2>%s</h2>
  <p>You can close this tab and return to MyGamesAnywhere.</p>
</div>
<script>setTimeout(function() { window.close(); }, 2000);</script>
</body></html>`, icon, message)
}
