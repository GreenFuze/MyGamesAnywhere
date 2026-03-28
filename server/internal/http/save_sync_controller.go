package http

import (
	"encoding/json"
	"net/http"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

type SaveSyncController struct {
	service core.SaveSyncService
	logger  core.Logger
}

func NewSaveSyncController(service core.SaveSyncService, logger core.Logger) *SaveSyncController {
	return &SaveSyncController{service: service, logger: logger}
}

func (c *SaveSyncController) ListSlots(w http.ResponseWriter, r *http.Request) {
	gameID, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	summaries, err := c.service.ListSlots(r.Context(), core.SaveSyncListRequest{
		CanonicalGameID: gameID,
		SourceGameID:    r.URL.Query().Get("source_game_id"),
		Runtime:         r.URL.Query().Get("runtime"),
		IntegrationID:   r.URL.Query().Get("integration_id"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"slots": summaries})
}

func (c *SaveSyncController) GetSlot(w http.ResponseWriter, r *http.Request) {
	gameID, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	snapshot, err := c.service.GetSlot(r.Context(), core.SaveSyncSlotRef{
		CanonicalGameID: gameID,
		SourceGameID:    r.URL.Query().Get("source_game_id"),
		Runtime:         r.URL.Query().Get("runtime"),
		SlotID:          chi.URLParam(r, "slot_id"),
		IntegrationID:   r.URL.Query().Get("integration_id"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if snapshot == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snapshot)
}

func (c *SaveSyncController) PutSlot(w http.ResponseWriter, r *http.Request) {
	gameID, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var body struct {
		IntegrationID    string                 `json:"integration_id"`
		SourceGameID     string                 `json:"source_game_id"`
		Runtime          string                 `json:"runtime"`
		BaseManifestHash string                 `json:"base_manifest_hash"`
		Force            bool                   `json:"force"`
		Snapshot         core.SaveSyncSnapshot  `json:"snapshot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	result, err := c.service.PutSlot(r.Context(), core.SaveSyncPutRequest{
		SaveSyncSlotRef: core.SaveSyncSlotRef{
			CanonicalGameID: gameID,
			SourceGameID:    body.SourceGameID,
			Runtime:         body.Runtime,
			SlotID:          chi.URLParam(r, "slot_id"),
			IntegrationID:   body.IntegrationID,
		},
		BaseManifestHash: body.BaseManifestHash,
		Force:            body.Force,
		Snapshot:         body.Snapshot,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if result.Conflict != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(result)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (c *SaveSyncController) StartMigration(w http.ResponseWriter, r *http.Request) {
	var body core.SaveSyncMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	status, err := c.service.StartMigration(r.Context(), body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(status)
}

func (c *SaveSyncController) GetMigrationStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	if jobID == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}
	status, err := c.service.GetMigrationStatus(r.Context(), jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if status == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}
