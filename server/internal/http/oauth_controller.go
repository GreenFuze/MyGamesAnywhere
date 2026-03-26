package http

import (
	"fmt"
	"net/http"

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
}

func NewOAuthController(
	pluginHost plugins.PluginHost,
	config core.Configuration,
	logger core.Logger,
	eventBus *events.EventBus,
) *OAuthController {
	return &OAuthController{
		pluginHost: pluginHost,
		config:     config,
		logger:     logger,
		eventBus:   eventBus,
	}
}

// Callback handles GET /api/auth/callback/{plugin_id}.
// Supports both OAuth2 (code+state) and OpenID 2.0 (openid.* params).
func (c *OAuthController) Callback(w http.ResponseWriter, r *http.Request) {
	pluginID := chi.URLParam(r, "plugin_id")
	if pluginID == "" {
		c.renderCallbackPage(w, false, "Missing plugin_id in callback URL")
		return
	}

	state := r.URL.Query().Get("state")

	// Route to the appropriate handler based on query params.
	if r.URL.Query().Get("openid.mode") != "" {
		c.handleOpenIDCallback(w, r, pluginID, state)
	} else {
		c.handleOAuth2Callback(w, r, pluginID, state)
	}
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
	port := c.config.Get("PORT")
	if port == "" {
		port = "8900"
	}
	redirectURI := fmt.Sprintf("http://localhost:%s/api/auth/callback/%s", port, pluginID)

	c.finishCallback(w, r, pluginID, state, map[string]any{
		"code":         code,
		"state":        state,
		"redirect_uri": redirectURI,
	})
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

	c.finishCallback(w, r, pluginID, state, map[string]any{
		"state":  state,
		"params": params,
	})
}

// finishCallback calls the plugin's auth.oauth.callback IPC and handles the result.
func (c *OAuthController) finishCallback(w http.ResponseWriter, r *http.Request, pluginID, state string, ipcPayload map[string]any) {
	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	err := c.pluginHost.Call(r.Context(), pluginID, "auth.oauth.callback", ipcPayload, &result)

	if err != nil {
		c.logger.Error("auth.oauth.callback IPC failed", err, "plugin_id", pluginID)
		c.publishErrorAndRender(w, pluginID, state, err.Error())
		return
	}

	if result.Status != "ok" {
		msg := result.Message
		if msg == "" {
			msg = result.Status
		}
		c.publishErrorAndRender(w, pluginID, state, msg)
		return
	}

	// Success — publish SSE event so the frontend wizard knows.
	events.PublishJSON(c.eventBus, "oauth_complete", map[string]any{
		"plugin_id": pluginID,
		"state":     state,
	})
	c.renderCallbackPage(w, true, "Authentication successful!")
}

// publishErrorAndRender publishes an oauth_error SSE event and renders the error page.
func (c *OAuthController) publishErrorAndRender(w http.ResponseWriter, pluginID, state, errMsg string) {
	events.PublishJSON(c.eventBus, "oauth_error", map[string]any{
		"plugin_id": pluginID,
		"state":     state,
		"error":     errMsg,
	})
	c.renderCallbackPage(w, false, "Authentication failed: "+errMsg)
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
