package http

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// SettingKeyFrontend is the settings row key for SPA preferences (JSON object).
const SettingKeyFrontend = "frontend"

const maxFrontendJSONBytes = 256 * 1024

// GetFrontend returns stored frontend preferences JSON (GET /api/config/frontend).
// Missing or invalid stored value yields {}.
func (c *ConfigController) GetFrontend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := c.repo.Get(ctx, SettingKeyFrontend)
	if err != nil {
		c.logger.Error("get frontend settings", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if s == nil || s.Value == "" {
		_, _ = w.Write([]byte("{}"))
		return
	}
	var check map[string]any
	if err := json.Unmarshal([]byte(s.Value), &check); err != nil || check == nil {
		_, _ = w.Write([]byte("{}"))
		return
	}
	_, _ = w.Write([]byte(s.Value))
}

// SetFrontend stores a JSON object as frontend preferences (POST /api/config/frontend).
func (c *ConfigController) SetFrontend(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxFrontendJSONBytes+1))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(body) > maxFrontendJSONBytes {
		http.Error(w, "JSON body too large", http.StatusBadRequest)
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		http.Error(w, "body must be a JSON object", http.StatusBadRequest)
		return
	}
	if raw == nil {
		http.Error(w, "body must be a JSON object", http.StatusBadRequest)
		return
	}
	out, err := json.Marshal(raw)
	if err != nil {
		http.Error(w, "invalid JSON object", http.StatusBadRequest)
		return
	}
	s := &core.Setting{Key: SettingKeyFrontend, Value: string(out), UpdatedAt: time.Now()}
	if err := c.repo.Upsert(r.Context(), s); err != nil {
		c.logger.Error("upsert frontend settings", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
