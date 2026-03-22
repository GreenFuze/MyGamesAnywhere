package http

import (
	"encoding/json"
	"net/http"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// SyncController serves the /api/sync/* endpoints.
type SyncController struct {
	syncSvc core.SyncService
	logger  core.Logger
}

func NewSyncController(syncSvc core.SyncService, logger core.Logger) *SyncController {
	return &SyncController{syncSvc: syncSvc, logger: logger}
}

func (c *SyncController) Push(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
	}

	result, err := c.syncSvc.Push(r.Context(), body.Passphrase)
	if err != nil {
		c.logger.Error("sync push", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":          "ok",
		"exported_at":     result.ExportedAt,
		"integrations":    result.Integrations,
		"settings":        result.Settings,
		"remote_versions": result.RemoteVersions,
	})
}

func (c *SyncController) Pull(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
	}

	result, err := c.syncSvc.Pull(r.Context(), body.Passphrase)
	if err != nil {
		c.logger.Error("sync pull", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"result": result,
	})
}

func (c *SyncController) Status(w http.ResponseWriter, r *http.Request) {
	status, err := c.syncSvc.Status(r.Context())
	if err != nil {
		c.logger.Error("sync status", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (c *SyncController) StoreKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.Passphrase == "" {
		http.Error(w, "passphrase is required", http.StatusBadRequest)
		return
	}

	if err := c.syncSvc.StoreKey(body.Passphrase); err != nil {
		c.logger.Error("store sync key", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (c *SyncController) ClearKey(w http.ResponseWriter, r *http.Request) {
	if err := c.syncSvc.ClearKey(); err != nil {
		c.logger.Error("clear sync key", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
