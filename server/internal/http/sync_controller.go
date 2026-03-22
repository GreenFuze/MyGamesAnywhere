package http

import (
	"encoding/json"
	"net/http"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
)

// SyncController serves the /api/sync/* endpoints.
type SyncController struct {
	syncSvc  core.SyncService
	logger   core.Logger
	eventBus *events.EventBus
}

func NewSyncController(syncSvc core.SyncService, logger core.Logger, eventBus *events.EventBus) *SyncController {
	return &SyncController{syncSvc: syncSvc, logger: logger, eventBus: eventBus}
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

	events.PublishJSON(c.eventBus, "sync_operation_started", map[string]any{"operation": "push"})
	result, err := c.syncSvc.Push(r.Context(), body.Passphrase)
	if err != nil {
		c.logger.Error("sync push", err)
		events.PublishJSON(c.eventBus, "sync_operation_finished", map[string]any{
			"operation": "push",
			"ok":        false,
			"error":     err.Error(),
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	events.PublishJSON(c.eventBus, "sync_operation_finished", map[string]any{
		"operation":         "push",
		"ok":                true,
		"integrations":      result.Integrations,
		"settings":          result.Settings,
		"remote_versions":   result.RemoteVersions,
		"exported_at_rfc3339": result.ExportedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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

	events.PublishJSON(c.eventBus, "sync_operation_started", map[string]any{"operation": "pull"})
	result, err := c.syncSvc.Pull(r.Context(), body.Passphrase)
	if err != nil {
		c.logger.Error("sync pull", err)
		events.PublishJSON(c.eventBus, "sync_operation_finished", map[string]any{
			"operation": "pull",
			"ok":        false,
			"error":     err.Error(),
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	events.PublishJSON(c.eventBus, "sync_operation_finished", map[string]any{
		"operation": "pull",
		"ok":        true,
		"integrations_added":   result.IntegrationsAdded,
		"integrations_updated": result.IntegrationsUpdated,
		"integrations_skipped": result.IntegrationsSkipped,
		"settings_added":       result.SettingsAdded,
		"settings_updated":     result.SettingsUpdated,
		"settings_skipped":     result.SettingsSkipped,
		"remote_exported_at_rfc3339": result.RemoteExportedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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
		events.PublishJSON(c.eventBus, "operation_error", map[string]any{
			"scope": "sync_key", "operation": "store", "error": err.Error(),
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	events.PublishJSON(c.eventBus, "sync_key_stored", map[string]any{"status": "ok"})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (c *SyncController) ClearKey(w http.ResponseWriter, r *http.Request) {
	if err := c.syncSvc.ClearKey(); err != nil {
		c.logger.Error("clear sync key", err)
		events.PublishJSON(c.eventBus, "operation_error", map[string]any{
			"scope": "sync_key", "operation": "clear", "error": err.Error(),
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	events.PublishJSON(c.eventBus, "sync_key_cleared", map[string]any{"status": "ok"})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
