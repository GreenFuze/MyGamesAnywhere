package http

// QR sign-in endpoints (ADR-0033).
//
// Some providers authenticate by having the player approve a challenge in their
// own mobile app instead of typing credentials into MGA. The player's password
// and second factor never reach MGA; the provider returns a long-lived refresh
// token that MGA stores on the profile's connection.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

const (
	qrBeginMethod = "auth.qr.begin"
	qrPollMethod  = "auth.qr.poll"
)

type qrBeginRequest struct {
	IntegrationID string `json:"integration_id"`
}

type qrBeginResponse struct {
	Status          string `json:"status"`
	ClientID        string `json:"client_id"`
	RequestID       string `json:"request_id"`
	ChallengeURL    string `json:"challenge_url"`
	IntervalSeconds int    `json:"interval_seconds"`
}

type qrPollRequest struct {
	IntegrationID string `json:"integration_id"`
	ClientID      string `json:"client_id"`
	RequestID     string `json:"request_id"`
}

type qrPollResponse struct {
	Status           string         `json:"status"`
	AccountName      string         `json:"account_name,omitempty"`
	ProviderIdentity map[string]any `json:"provider_identity,omitempty"`
}

// pluginQRPollResult is the plugin's reply to auth.qr.poll.
type pluginQRPollResult struct {
	Status           string         `json:"status"`
	AccountName      string         `json:"account_name"`
	Message          string         `json:"message"`
	ProviderIdentity map[string]any `json:"provider_identity,omitempty"`
	ConfigUpdates    map[string]any `json:"config_updates,omitempty"`
}

// QRBegin handles POST /api/auth/qr/{plugin_id}/begin. It asks the plugin to
// start a sign-in challenge and returns what the player must scan.
func (c *OAuthController) QRBegin(w http.ResponseWriter, r *http.Request) {
	pluginID := strings.TrimSpace(chi.URLParam(r, "plugin_id"))
	if pluginID == "" {
		http.Error(w, "plugin_id is required", http.StatusBadRequest)
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	if profileID == "" {
		http.Error(w, core.ErrProfileRequired.Error(), http.StatusBadRequest)
		return
	}

	// The request body is optional; an integration ID only scopes logging here.
	var body qrBeginRequest
	_ = json.NewDecoder(r.Body).Decode(&body)

	var result qrBeginResponse
	if err := c.pluginHost.Call(r.Context(), pluginID, qrBeginMethod, map[string]any{}, &result); err != nil {
		c.logger.Error("start QR sign-in", err, "plugin_id", pluginID, "profile_id", profileID)
		http.Error(w, fmt.Sprintf("start sign-in: %v", err), http.StatusBadGateway)
		return
	}
	if strings.TrimSpace(result.ChallengeURL) == "" {
		http.Error(w, "the provider did not return a sign-in challenge", http.StatusBadGateway)
		return
	}
	if result.Status == "" {
		result.Status = "pending"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// QRPoll handles POST /api/auth/qr/{plugin_id}/poll. While the player has not
// approved yet it reports "pending". On approval it persists the returned
// credentials onto the profile's own connection.
func (c *OAuthController) QRPoll(w http.ResponseWriter, r *http.Request) {
	pluginID := strings.TrimSpace(chi.URLParam(r, "plugin_id"))
	if pluginID == "" {
		http.Error(w, "plugin_id is required", http.StatusBadRequest)
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	if profileID == "" {
		http.Error(w, core.ErrProfileRequired.Error(), http.StatusBadRequest)
		return
	}

	var body qrPollRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	body.IntegrationID = strings.TrimSpace(body.IntegrationID)
	if body.IntegrationID == "" {
		http.Error(w, "integration_id is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.ClientID) == "" || strings.TrimSpace(body.RequestID) == "" {
		http.Error(w, "client_id and request_id are required", http.StatusBadRequest)
		return
	}

	// Load the profile's own connection first so the poll can never write to
	// another profile's connection.
	integration, config, err := c.loadOwnedIntegration(r.Context(), pluginID, body.IntegrationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	params := map[string]any{
		"client_id":  body.ClientID,
		"request_id": body.RequestID,
	}
	// A plugin may use an existing application credential to resolve safe
	// player-facing identity metadata after approval. Provider identity itself
	// must still come from the newly approved token, never from this config.
	if apiKey, ok := config["api_key"].(string); ok && strings.TrimSpace(apiKey) != "" {
		params["api_key"] = apiKey
	}

	var result pluginQRPollResult
	if err := c.pluginHost.Call(r.Context(), pluginID, qrPollMethod, params, &result); err != nil {
		// A dead challenge is a normal outcome the player can retry, not a 5xx.
		c.logger.Error("poll QR sign-in", err, "plugin_id", pluginID, "profile_id", profileID)
		http.Error(w, fmt.Sprintf("sign-in did not complete: %v", err), http.StatusBadRequest)
		return
	}

	if result.Status != "ok" {
		status := result.Status
		if status == "" {
			status = "pending"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(qrPollResponse{Status: status})
		return
	}

	if err := c.persistQRCredentials(r.Context(), integration, config, result.ConfigUpdates); err != nil {
		c.logger.Error("save QR sign-in credentials", err, "plugin_id", pluginID, "profile_id", profileID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(qrPollResponse{
		Status:           "ok",
		AccountName:      result.AccountName,
		ProviderIdentity: result.ProviderIdentity,
	})
}

// loadOwnedIntegration returns the requested connection only when it belongs to
// the calling profile and the expected plugin.
func (c *OAuthController) loadOwnedIntegration(ctx context.Context, pluginID, integrationID string) (*core.Integration, map[string]any, error) {
	if c.repo == nil {
		return nil, nil, fmt.Errorf("connections are unavailable")
	}
	integration, err := c.repo.GetByID(ctx, integrationID)
	if err != nil {
		return nil, nil, fmt.Errorf("load connection: %w", err)
	}
	if integration == nil {
		return nil, nil, fmt.Errorf("connection was not found")
	}
	if integration.PluginID != pluginID {
		return nil, nil, fmt.Errorf("connection is for %s, not %s", integration.PluginID, pluginID)
	}
	if profileID := core.ProfileIDFromContext(ctx); profileID != "" && integration.ProfileID != "" && integration.ProfileID != profileID {
		return nil, nil, core.ErrProfileForbidden
	}
	config, err := decodeIntegrationConfig(integration.ConfigJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("decode connection configuration: %w", err)
	}
	return integration, config, nil
}

// persistQRCredentials merges the plugin's credential updates into the
// connection and clears any outstanding re-authentication flag.
func (c *OAuthController) persistQRCredentials(ctx context.Context, integration *core.Integration, config map[string]any, updates map[string]any) error {
	if len(updates) == 0 {
		return fmt.Errorf("sign-in returned no credentials to save")
	}
	mergeConfigUpdates(config, updates)
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode connection configuration: %w", err)
	}
	integration.ConfigJSON = string(configBytes)
	integration.NeedsReauth = false
	integration.UpdatedAt = time.Now()
	if err := c.repo.Update(ctx, integration); err != nil {
		return fmt.Errorf("save connection credentials: %w", err)
	}
	return nil
}
