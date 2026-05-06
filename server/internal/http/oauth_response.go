package http

import (
	"net"
	"net/http"
	"strings"
)

type oauthRequiredPayload struct {
	Status                 string `json:"status"`
	PluginID               string `json:"plugin_id"`
	AuthorizeURL           string `json:"authorize_url"`
	State                  string `json:"state"`
	CallbackURL            string `json:"callback_url"`
	PasteCallbackSupported bool   `json:"paste_callback_supported"`
	RemoteBrowserHint      bool   `json:"remote_browser_hint"`
}

func buildOAuthRequiredPayload(r *http.Request, pluginID, authorizeURL, state, callbackURL string) oauthRequiredPayload {
	return oauthRequiredPayload{
		Status:                 "oauth_required",
		PluginID:               pluginID,
		AuthorizeURL:           authorizeURL,
		State:                  state,
		CallbackURL:            callbackURL,
		PasteCallbackSupported: true,
		RemoteBrowserHint:      !isLocalRequestHost(r),
	}
}

func isLocalRequestHost(r *http.Request) bool {
	if r == nil {
		return true
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return true
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
