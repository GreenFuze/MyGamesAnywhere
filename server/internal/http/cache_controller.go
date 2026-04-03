package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

type CacheController struct {
	gameStore       core.GameStore
	integrationRepo core.IntegrationRepository
	cacheSvc        core.SourceCacheService
	logger          core.Logger
}

type CachePrepareResponse struct {
	Accepted  bool                       `json:"accepted"`
	Immediate bool                       `json:"immediate"`
	Job       *core.SourceCacheJobStatus `json:"job,omitempty"`
}

type CacheEntryDTO struct {
	core.SourceCacheEntry
	IntegrationLabel string `json:"integration_label,omitempty"`
}

func NewCacheController(
	gameStore core.GameStore,
	integrationRepo core.IntegrationRepository,
	cacheSvc core.SourceCacheService,
	logger core.Logger,
) *CacheController {
	return &CacheController{
		gameStore:       gameStore,
		integrationRepo: integrationRepo,
		cacheSvc:        cacheSvc,
		logger:          logger,
	}
}

func (c *CacheController) PrepareGameCache(w http.ResponseWriter, r *http.Request) {
	if c.cacheSvc == nil {
		http.Error(w, "cache service unavailable", http.StatusServiceUnavailable)
		return
	}
	gameID, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body struct {
		SourceGameID string `json:"source_game_id"`
		Profile      string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	game, err := c.gameStore.GetCanonicalGameByID(r.Context(), gameID)
	if err != nil {
		c.logger.Error("get game for cache prepare", err, "game_id", gameID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	sourceGame := findSourceGame(game, body.SourceGameID)
	if sourceGame == nil {
		http.Error(w, "source_game_id not found", http.StatusBadRequest)
		return
	}

	job, immediate, err := c.cacheSvc.Prepare(r.Context(), core.SourceCachePrepareRequest{
		CanonicalGameID: game.ID,
		CanonicalTitle:  game.Title,
		SourceGameID:    body.SourceGameID,
		Profile:         body.Profile,
	}, game.Platform, sourceGame)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if immediate {
		_ = json.NewEncoder(w).Encode(CachePrepareResponse{Accepted: true, Immediate: true, Job: job})
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(CachePrepareResponse{Accepted: true, Immediate: false, Job: job})
}

func (c *CacheController) GetJob(w http.ResponseWriter, r *http.Request) {
	if c.cacheSvc == nil {
		http.Error(w, "cache service unavailable", http.StatusServiceUnavailable)
		return
	}
	jobID := chi.URLParam(r, "job_id")
	if strings.TrimSpace(jobID) == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}
	job, err := c.cacheSvc.GetJob(r.Context(), jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(job)
}

func (c *CacheController) ListJobs(w http.ResponseWriter, r *http.Request) {
	if c.cacheSvc == nil {
		http.Error(w, "cache service unavailable", http.StatusServiceUnavailable)
		return
	}
	limit := 25
	if raw := r.URL.Query().Get("limit"); strings.TrimSpace(raw) != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	jobs, err := c.cacheSvc.ListJobs(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jobs": jobs})
}

func (c *CacheController) ListEntries(w http.ResponseWriter, r *http.Request) {
	if c.cacheSvc == nil {
		http.Error(w, "cache service unavailable", http.StatusServiceUnavailable)
		return
	}
	entries, err := c.cacheSvc.ListEntries(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	labels := map[string]string{}
	if c.integrationRepo != nil {
		if integrations, err := c.integrationRepo.List(r.Context()); err == nil {
			for _, integration := range integrations {
				if integration != nil {
					labels[integration.ID] = integration.Label
				}
			}
		}
	}
	result := make([]CacheEntryDTO, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		result = append(result, CacheEntryDTO{
			SourceCacheEntry: *entry,
			IntegrationLabel: labels[entry.IntegrationID],
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"entries": result})
}

func (c *CacheController) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	if c.cacheSvc == nil {
		http.Error(w, "cache service unavailable", http.StatusServiceUnavailable)
		return
	}
	entryID := chi.URLParam(r, "entry_id")
	if strings.TrimSpace(entryID) == "" {
		http.Error(w, "entry_id is required", http.StatusBadRequest)
		return
	}
	if err := c.cacheSvc.DeleteEntry(r.Context(), entryID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *CacheController) Clear(w http.ResponseWriter, r *http.Request) {
	if c.cacheSvc == nil {
		http.Error(w, "cache service unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := c.cacheSvc.ClearEntries(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func findSourceGame(game *core.CanonicalGame, sourceGameID string) *core.SourceGame {
	if game == nil {
		return nil
	}
	for _, sourceGame := range game.SourceGames {
		if sourceGame != nil && sourceGame.ID == sourceGameID {
			return sourceGame
		}
	}
	return nil
}
