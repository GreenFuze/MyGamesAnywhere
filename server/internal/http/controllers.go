// Package http implements the server's public HTTP API.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	appconfig "github.com/GreenFuze/MyGamesAnywhere/server/internal/config"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	defaultGamesPageSize = 100
	maxGamesPageSize     = 2000
	maxGamesFetchAll     = 20000
)

// ListGamesResponse is the response for GET /api/games (paginated, full detail rows for library UI).
type ListGamesResponse struct {
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Games    []GameDetailResponse `json:"games"`
}

// GameSummary is a lightweight row (e.g. POST /api/scan results). List uses GameDetailResponse per item.
type GameSummary struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Platform     string          `json:"platform"`
	Kind         string          `json:"kind"`
	ParentGameID string          `json:"parent_game_id,omitempty"`
	GroupKind    string          `json:"group_kind"`
	RootPath     string          `json:"root_path,omitempty"`
	Files        []GameFileDTO   `json:"files,omitempty"`
	ExternalIDs  []ExternalIDDTO `json:"external_ids,omitempty"`
	// Unified Xbox / storefront (from resolver metadata, same as detail).
	IsGamePass      bool   `json:"is_game_pass,omitempty"`
	XcloudAvailable bool   `json:"xcloud_available,omitempty"`
	StoreProductID  string `json:"store_product_id,omitempty"`
	XcloudURL       string `json:"xcloud_url,omitempty"`
}

// ExternalIDDTO is a reference to an external metadata database.
type ExternalIDDTO struct {
	Source     string `json:"source"`
	ExternalID string `json:"external_id"`
	URL        string `json:"url,omitempty"`
}

// GameFileDTO is a file belonging to a game.
type GameFileDTO struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Role     string `json:"role"`
	FileKind string `json:"file_kind,omitempty"`
	Size     int64  `json:"size"`
}

// GameController serves GET /api/games (list) and GET /api/games/{id} (single game).
type GameController struct {
	gameStore       core.GameStore
	refreshSvc      core.GameMetadataRefreshService
	deleteSvc       core.GameDeletionService
	integrationRepo core.IntegrationRepository
	cacheSvc        core.SourceCacheService
	logger          core.Logger
}

type DeleteSourceGameResponse struct {
	DeletedSourceGameID string              `json:"deleted_source_game_id"`
	CanonicalExists     bool                `json:"canonical_exists"`
	Game                *GameDetailResponse `json:"game,omitempty"`
}

type DeleteSourceGamePreviewResponse = core.DeleteSourceGamePreview

type SetCoverOverrideRequest struct {
	MediaAssetID int `json:"media_asset_id"`
}

type SetHoverOverrideRequest struct {
	MediaAssetID int `json:"media_asset_id"`
}

type SetBackgroundOverrideRequest struct {
	MediaAssetID int `json:"media_asset_id"`
}

type AchievementsDashboardResponse struct {
	Totals  AchievementSummaryDTO       `json:"totals"`
	Systems []AchievementSystemSummary  `json:"systems"`
	Games   []AchievementGameSummaryDTO `json:"games"`
}

type AchievementsExplorerResponse struct {
	Games []AchievementGameExplorerDTO `json:"games"`
}

type AchievementSystemSummary struct {
	Source        string `json:"source"`
	GameCount     int    `json:"game_count"`
	TotalCount    int    `json:"total_count"`
	UnlockedCount int    `json:"unlocked_count"`
	TotalPoints   int    `json:"total_points,omitempty"`
	EarnedPoints  int    `json:"earned_points,omitempty"`
}

type AchievementGameSummaryDTO struct {
	Game    GameDetailResponse         `json:"game"`
	Systems []AchievementSystemSummary `json:"systems"`
}

type AchievementGameExplorerDTO struct {
	Game    GameDetailResponse  `json:"game"`
	Systems []AchievementSetDTO `json:"systems"`
}

func NewGameController(gameStore core.GameStore, refreshSvc core.GameMetadataRefreshService, deleteSvc core.GameDeletionService, integrationRepo core.IntegrationRepository, cacheSvc core.SourceCacheService, logger core.Logger) *GameController {
	return &GameController{gameStore: gameStore, refreshSvc: refreshSvc, deleteSvc: deleteSvc, integrationRepo: integrationRepo, cacheSvc: cacheSvc, logger: logger}
}

func decodedPathParam(r *http.Request, key string) (string, error) {
	value := chi.URLParam(r, key)
	if value == "" {
		return "", nil
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return "", fmt.Errorf("invalid path parameter %q: %w", key, err)
	}
	return decoded, nil
}

func (c *GameController) loadIntegrationLabels(ctx context.Context) map[string]string {
	if c == nil || c.integrationRepo == nil {
		return nil
	}
	integrations, err := c.integrationRepo.List(ctx)
	if err != nil {
		c.logger.Warn("list integrations for labels failed", "error", err)
		return nil
	}
	labels := make(map[string]string, len(integrations))
	for _, integration := range integrations {
		if integration == nil {
			continue
		}
		labels[integration.ID] = integration.Label
	}
	return labels
}

func writeActionError(w http.ResponseWriter, status int, message string) {
	http.Error(w, strings.TrimSpace(message), status)
}

// ListGames returns a page of canonical games as full detail rows (GET /api/games).
// Query: page (0-based, default 0), page_size (default 100, max 2000). page_size=0 means all games
// (capped at maxGamesFetchAll); use GET /api/stats canonical_game_count for totals.
func (c *GameController) ListGames(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	total, err := c.gameStore.CountVisibleCanonicalGames(ctx)
	if err != nil {
		c.logger.Error("count games", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	page := 0
	if s := r.URL.Query().Get("page"); s != "" {
		p, err := strconv.Atoi(s)
		if err != nil || p < 0 {
			http.Error(w, "invalid page", http.StatusBadRequest)
			return
		}
		page = p
	}

	pageSizeStr := r.URL.Query().Get("page_size")
	pageSize := defaultGamesPageSize
	fetchAll := false
	if pageSizeStr != "" {
		ps, err := strconv.Atoi(pageSizeStr)
		if err != nil || ps < 0 {
			http.Error(w, "invalid page_size", http.StatusBadRequest)
			return
		}
		if ps == 0 {
			fetchAll = true
		} else {
			pageSize = ps
			if pageSize > maxGamesPageSize {
				pageSize = maxGamesPageSize
			}
		}
	}

	var offset, sqlLimit int
	respPageSize := pageSize
	if fetchAll {
		if total > maxGamesFetchAll {
			http.Error(w, fmt.Sprintf("library too large (%d games); use page_size between 1 and %d", total, maxGamesPageSize), http.StatusBadRequest)
			return
		}
		offset = 0
		sqlLimit = -1
		respPageSize = 0
		page = 0
	} else {
		offset = page * pageSize
		if offset >= total {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(ListGamesResponse{
				Total: total, Page: page, PageSize: pageSize, Games: []GameDetailResponse{},
			})
			return
		}
		sqlLimit = pageSize
	}

	ids, err := c.gameStore.GetVisibleCanonicalIDs(ctx, offset, sqlLimit)
	if err != nil {
		c.logger.Error("list game ids", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	games, err := c.gameStore.GetCanonicalGamesByIDs(ctx, ids)
	if err != nil {
		c.logger.Error("get games page", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	integrationLabels := c.loadIntegrationLabels(ctx)

	out := make([]GameDetailResponse, 0, len(games))
	for _, cg := range games {
		if cg == nil {
			continue
		}
		out = append(out, c.canonicalToGameDetailWithIntegrationLabels(ctx, cg, integrationLabels))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ListGamesResponse{
		Total:    total,
		Page:     page,
		PageSize: respPageSize,
		Games:    out,
	})
}

// Get returns one canonical game by ID (GET /api/games/{id}) as full detail — same JSON as GET /api/games/{id}/detail and each list item.
func (c *GameController) Get(w http.ResponseWriter, r *http.Request) {
	id, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	game, err := c.gameStore.GetCanonicalGameByID(ctx, id)
	if err != nil {
		c.logger.Error("get game", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.canonicalToGameDetailWithIntegrationLabels(ctx, game, c.loadIntegrationLabels(ctx)))
}

// GetDetail returns full game metadata and per-source resolver data (GET /api/games/{id}/detail).
func (c *GameController) GetDetail(w http.ResponseWriter, r *http.Request) {
	id, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	game, err := c.gameStore.GetCanonicalGameByID(ctx, id)
	if err != nil {
		c.logger.Error("get game detail", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.canonicalToGameDetailWithIntegrationLabels(ctx, game, c.loadIntegrationLabels(ctx)))
}

func (c *GameController) SetCoverOverride(w http.ResponseWriter, r *http.Request) {
	id, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body SetCoverOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if body.MediaAssetID <= 0 {
		http.Error(w, "media_asset_id is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := c.gameStore.SetCanonicalCoverOverride(ctx, id, body.MediaAssetID); err != nil {
		c.writeCoverOverrideError(w, err)
		return
	}
	game, err := c.gameStore.GetCanonicalGameByID(ctx, id)
	if err != nil {
		c.logger.Error("get game after cover override", err, "game_id", id)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.canonicalToGameDetailWithIntegrationLabels(ctx, game, c.loadIntegrationLabels(ctx)))
}

func (c *GameController) ClearCoverOverride(w http.ResponseWriter, r *http.Request) {
	id, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := c.gameStore.ClearCanonicalCoverOverride(ctx, id); err != nil {
		c.writeCoverOverrideError(w, err)
		return
	}
	game, err := c.gameStore.GetCanonicalGameByID(ctx, id)
	if err != nil {
		c.logger.Error("get game after clear cover override", err, "game_id", id)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.canonicalToGameDetailWithIntegrationLabels(ctx, game, c.loadIntegrationLabels(ctx)))
}

func (c *GameController) SetHoverOverride(w http.ResponseWriter, r *http.Request) {
	id, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body SetHoverOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if body.MediaAssetID <= 0 {
		http.Error(w, "media_asset_id is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := c.gameStore.SetCanonicalHoverOverride(ctx, id, body.MediaAssetID); err != nil {
		c.writeHoverOverrideError(w, err)
		return
	}
	game, err := c.gameStore.GetCanonicalGameByID(ctx, id)
	if err != nil {
		c.logger.Error("get game after hover override", err, "game_id", id)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.canonicalToGameDetailWithIntegrationLabels(ctx, game, c.loadIntegrationLabels(ctx)))
}

func (c *GameController) SetBackgroundOverride(w http.ResponseWriter, r *http.Request) {
	id, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body SetBackgroundOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if body.MediaAssetID <= 0 {
		http.Error(w, "media_asset_id is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := c.gameStore.SetCanonicalBackgroundOverride(ctx, id, body.MediaAssetID); err != nil {
		c.writeBackgroundOverrideError(w, err)
		return
	}
	game, err := c.gameStore.GetCanonicalGameByID(ctx, id)
	if err != nil {
		c.logger.Error("get game after background override", err, "game_id", id)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.canonicalToGameDetailWithIntegrationLabels(ctx, game, c.loadIntegrationLabels(ctx)))
}

func (c *GameController) SetFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := c.gameStore.SetCanonicalFavorite(ctx, id); err != nil {
		c.writeFavoriteError(w, err)
		return
	}
	game, err := c.gameStore.GetCanonicalGameByID(ctx, id)
	if err != nil {
		c.logger.Error("get game after set favorite", err, "game_id", id)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.canonicalToGameDetailWithIntegrationLabels(ctx, game, c.loadIntegrationLabels(ctx)))
}

func (c *GameController) ClearFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := c.gameStore.ClearCanonicalFavorite(ctx, id); err != nil {
		c.writeFavoriteError(w, err)
		return
	}
	game, err := c.gameStore.GetCanonicalGameByID(ctx, id)
	if err != nil {
		c.logger.Error("get game after clear favorite", err, "game_id", id)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.canonicalToGameDetailWithIntegrationLabels(ctx, game, c.loadIntegrationLabels(ctx)))
}

func (c *GameController) writeCoverOverrideError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrCanonicalGameNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, core.ErrCoverOverrideMediaNotFound):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		c.logger.Error("cover override failed", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (c *GameController) writeHoverOverrideError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrCanonicalGameNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, core.ErrHoverOverrideMediaNotFound):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		c.logger.Error("hover override failed", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (c *GameController) writeBackgroundOverrideError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrCanonicalGameNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, core.ErrBackgroundOverrideMediaNotFound):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		c.logger.Error("background override failed", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (c *GameController) writeFavoriteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrCanonicalGameNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		c.logger.Error("favorite update failed", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (c *GameController) AchievementsDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dashboard, err := c.gameStore.GetCachedAchievementsDashboard(ctx)
	if err != nil {
		c.logger.Error("cached achievements dashboard", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := AchievementsDashboardResponse{
		Systems: make([]AchievementSystemSummary, 0, len(dashboard.Systems)),
		Games:   make([]AchievementGameSummaryDTO, 0, len(dashboard.Games)),
	}
	resp.Totals = AchievementSummaryDTO{
		SourceCount:   dashboard.Totals.SourceCount,
		TotalCount:    dashboard.Totals.TotalCount,
		UnlockedCount: dashboard.Totals.UnlockedCount,
		TotalPoints:   dashboard.Totals.TotalPoints,
		EarnedPoints:  dashboard.Totals.EarnedPoints,
	}
	for _, system := range dashboard.Systems {
		resp.Systems = append(resp.Systems, achievementSystemSummaryDTO(system))
	}
	labels := c.loadIntegrationLabels(ctx)
	for _, game := range dashboard.Games {
		if game.Game == nil {
			continue
		}
		item := AchievementGameSummaryDTO{
			Game:    c.canonicalToGameDetailWithIntegrationLabels(ctx, game.Game, labels),
			Systems: make([]AchievementSystemSummary, 0, len(game.Systems)),
		}
		for _, system := range game.Systems {
			item.Systems = append(item.Systems, achievementSystemSummaryDTO(system))
		}
		resp.Games = append(resp.Games, item)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (c *GameController) AchievementsExplorer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	explorer, err := c.gameStore.GetCachedAchievementsExplorer(ctx)
	if err != nil {
		c.logger.Error("cached achievements explorer", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := AchievementsExplorerResponse{
		Games: make([]AchievementGameExplorerDTO, 0, len(explorer.Games)),
	}
	labels := c.loadIntegrationLabels(ctx)
	for _, game := range explorer.Games {
		if game.Game == nil {
			continue
		}
		item := AchievementGameExplorerDTO{
			Game:    c.canonicalToGameDetailWithIntegrationLabels(ctx, game.Game, labels),
			Systems: make([]AchievementSetDTO, 0, len(game.Systems)),
		}
		for _, system := range game.Systems {
			item.Systems = append(item.Systems, achievementSetToDTO(system))
		}
		resp.Games = append(resp.Games, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func achievementSystemSummaryDTO(system core.CachedAchievementSystemSummary) AchievementSystemSummary {
	return AchievementSystemSummary{
		Source:        system.Source,
		GameCount:     system.GameCount,
		TotalCount:    system.TotalCount,
		UnlockedCount: system.UnlockedCount,
		TotalPoints:   system.TotalPoints,
		EarnedPoints:  system.EarnedPoints,
	}
}

func (c *GameController) RefreshMetadata(w http.ResponseWriter, r *http.Request) {
	id, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if c.refreshSvc == nil {
		http.Error(w, "metadata refresh is not available", http.StatusNotImplemented)
		return
	}

	game, err := c.refreshSvc.RefreshGameMetadata(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, core.ErrMetadataRefreshNoEligible):
			writeActionError(w, http.StatusConflict, err.Error())
		case errors.Is(err, core.ErrMetadataProvidersUnavailable):
			writeActionError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			c.logger.Error("refresh game metadata", err, "game_id", id)
			writeActionError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c.canonicalToGameDetailWithIntegrationLabels(r.Context(), game, c.loadIntegrationLabels(r.Context())))
}

func (c *GameController) DeleteSourceGame(w http.ResponseWriter, r *http.Request) {
	canonicalID, err := decodedPathParam(r, "id")
	if err != nil {
		writeActionError(w, http.StatusBadRequest, err.Error())
		return
	}
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		writeActionError(w, http.StatusBadRequest, err.Error())
		return
	}
	if canonicalID == "" || sourceGameID == "" {
		writeActionError(w, http.StatusBadRequest, "id and source_game_id are required")
		return
	}
	if c.deleteSvc == nil {
		writeActionError(w, http.StatusNotImplemented, "source record hard delete is not available")
		return
	}

	result, err := c.deleteSvc.DeleteSourceGame(r.Context(), canonicalID, sourceGameID)
	if err != nil {
		switch {
		case errors.Is(err, core.ErrSourceGameDeleteNotFound):
			http.NotFound(w, r)
		case errors.Is(err, core.ErrSourceGameDeleteNotEligible):
			writeActionError(w, http.StatusConflict, err.Error())
		default:
			c.logger.Error("delete source game", err, "game_id", canonicalID, "source_game_id", sourceGameID)
			writeActionError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	resp := DeleteSourceGameResponse{
		DeletedSourceGameID: result.DeletedSourceGameID,
		CanonicalExists:     result.CanonicalExists,
	}
	if result.CanonicalGame != nil {
		detail := c.canonicalToGameDetailWithIntegrationLabels(r.Context(), result.CanonicalGame, c.loadIntegrationLabels(r.Context()))
		resp.Game = &detail
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (c *GameController) PreviewDeleteSourceGame(w http.ResponseWriter, r *http.Request) {
	canonicalID, err := decodedPathParam(r, "id")
	if err != nil {
		writeActionError(w, http.StatusBadRequest, err.Error())
		return
	}
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		writeActionError(w, http.StatusBadRequest, err.Error())
		return
	}
	if canonicalID == "" || sourceGameID == "" {
		writeActionError(w, http.StatusBadRequest, "id and source_game_id are required")
		return
	}
	if c.deleteSvc == nil {
		writeActionError(w, http.StatusNotImplemented, "source record hard delete preview is not available")
		return
	}

	preview, err := c.deleteSvc.PreviewDeleteSourceGame(r.Context(), canonicalID, sourceGameID)
	if err != nil {
		switch {
		case errors.Is(err, core.ErrSourceGameDeleteNotFound):
			http.NotFound(w, r)
		case errors.Is(err, core.ErrSourceGameDeleteNotEligible):
			writeActionError(w, http.StatusConflict, err.Error())
		default:
			c.logger.Error("preview source game delete", err, "game_id", canonicalID, "source_game_id", sourceGameID)
			writeActionError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(DeleteSourceGamePreviewResponse(*preview))
}

// Stats returns aggregate library statistics (GET /api/stats).
func (c *GameController) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := c.gameStore.GetLibraryStats(r.Context())
	if err != nil {
		c.logger.Error("library stats", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// DeleteAll removes all games and their files (DELETE /api/games).
func (c *GameController) DeleteAll(w http.ResponseWriter, r *http.Request) {
	if err := c.gameStore.DeleteAllGames(r.Context()); err != nil {
		c.logger.Error("delete all games", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "all games deleted"})
}

func canonicalToSummary(cg *core.CanonicalGame) GameSummary {
	s := GameSummary{
		ID:              cg.ID,
		Title:           cg.Title,
		Platform:        string(cg.Platform),
		Kind:            string(cg.Kind),
		GroupKind:       "",
		IsGamePass:      cg.IsGamePass,
		XcloudAvailable: cg.XcloudAvailable,
		StoreProductID:  cg.StoreProductID,
		XcloudURL:       cg.XcloudURL,
	}

	// Collect files from all source games.
	for _, sg := range cg.SourceGames {
		if sg.Status != "found" {
			continue
		}
		if s.GroupKind == "" {
			s.GroupKind = string(sg.GroupKind)
		}
		if s.RootPath == "" {
			s.RootPath = sg.RootPath
		}
		for _, f := range sg.Files {
			s.Files = append(s.Files, GameFileDTO{
				Path:     f.Path,
				Role:     string(f.Role),
				FileKind: f.FileKind,
				Size:     f.Size,
			})
		}
	}
	for _, eid := range cg.ExternalIDs {
		s.ExternalIDs = append(s.ExternalIDs, ExternalIDDTO{
			Source:     eid.Source,
			ExternalID: eid.ExternalID,
			URL:        eid.URL,
		})
	}
	return s
}

type DiscoveryController struct {
	orchestrator scanRunner
	scanJobs     *scanJobManager
	gameStore    core.GameStore
	logger       core.Logger
}

func NewDiscoveryController(orchestrator scanRunner, gameStore core.GameStore, logger core.Logger, eventBus *events.EventBus) *DiscoveryController {
	return &DiscoveryController{
		orchestrator: orchestrator,
		scanJobs:     newScanJobManager(orchestrator, eventBus, logger),
		gameStore:    gameStore,
		logger:       logger,
	}
}

func (c *DiscoveryController) StartScan(ctx context.Context, req ScanRequest) (*core.ScanJobStatus, bool, error) {
	return c.scanJobs.Start(ctx, req)
}

type ScanRequest struct {
	GameSources  []string `json:"game_sources"`
	MetadataOnly bool     `json:"metadata_only"`
}

func (c *DiscoveryController) Scan(w http.ResponseWriter, r *http.Request) {
	var body ScanRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
	}
	var integrationIDs []string
	if len(body.GameSources) > 0 {
		integrationIDs = body.GameSources
	}
	status, alreadyRunning, err := c.scanJobs.Start(r.Context(), ScanRequest{
		GameSources:  integrationIDs,
		MetadataOnly: body.MetadataOnly,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if alreadyRunning {
		w.WriteHeader(http.StatusConflict)
	} else {
		w.WriteHeader(http.StatusAccepted)
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (c *DiscoveryController) GetScanJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	if jobID == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}
	status := c.scanJobs.Get(jobID)
	if status == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (c *DiscoveryController) CancelScanJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	if jobID == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}
	status, result := c.scanJobs.Cancel(jobID)
	if result == scanCancelNotFound {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch result {
	case scanCancelAccepted:
		w.WriteHeader(http.StatusAccepted)
	case scanCancelNoop:
		w.WriteHeader(http.StatusOK)
	case scanCancelConflict:
		w.WriteHeader(http.StatusConflict)
	}
	_ = json.NewEncoder(w).Encode(status)
}

// GetScanReports returns the last N scan reports.
func (c *DiscoveryController) GetScanReports(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if limitStr != "" {
		if n, err := fmt.Sscanf(limitStr, "%d", &limit); n != 1 || err != nil {
			limit = 10
		}
	}

	reports, err := c.gameStore.GetScanReports(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if reports == nil {
		reports = []*core.ScanReport{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reports)
}

// GetScanReport returns a single scan report by ID.
func (c *DiscoveryController) GetScanReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	report, err := c.gameStore.GetScanReport(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if report == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

type ConfigController struct {
	repo   core.SettingRepository
	logger core.Logger
}

func NewConfigController(repo core.SettingRepository, logger core.Logger) *ConfigController {
	return &ConfigController{repo: repo, logger: logger}
}

func (c *ConfigController) Set(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s := &core.Setting{Key: key, Value: body.Value, UpdatedAt: time.Now()}
	if err := c.repo.Upsert(r.Context(), s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

type PluginController struct {
	repo       core.IntegrationRepository
	pluginHost plugins.PluginHost
	gameStore  core.GameStore
	config     core.Configuration
	logger     core.Logger
	eventBus   *events.EventBus
}

func NewPluginController(repo core.IntegrationRepository, pluginHost plugins.PluginHost, gameStore core.GameStore, config core.Configuration, logger core.Logger, eventBus *events.EventBus) *PluginController {
	return &PluginController{repo: repo, pluginHost: pluginHost, gameStore: gameStore, config: config, logger: logger, eventBus: eventBus}
}

func (c *PluginController) publishNotification(typ string, payload map[string]any) {
	events.PublishJSON(c.eventBus, typ, payload)
}

// oauthRedirectURI computes the OAuth callback URL for a given plugin.
func (c *PluginController) oauthRedirectURI(pluginID string) string {
	redirectURI, err := appconfig.OAuthCallbackURL(c.config, pluginID)
	if err != nil {
		c.logger.Error("oauth redirect url", err, "plugin_id", pluginID)
		return ""
	}
	return redirectURI
}

func (c *PluginController) ListPlugins(w http.ResponseWriter, r *http.Request) {
	list := c.pluginHost.ListPlugins()
	json.NewEncoder(w).Encode(list)
}

func (c *PluginController) GetPluginByID(w http.ResponseWriter, r *http.Request) {
	pluginID := chi.URLParam(r, "plugin_id")
	if pluginID == "" {
		http.Error(w, "plugin_id is required", http.StatusBadRequest)
		return
	}
	plugin, ok := c.pluginHost.GetPlugin(pluginID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	info := plugins.PluginInfo{
		PluginID:     plugin.Manifest.ID,
		Version:      plugin.Manifest.Version,
		Provides:     plugin.Manifest.Provides,
		Capabilities: plugin.Manifest.Capabilities,
		ConfigSchema: plugin.Manifest.ConfigSchema,
	}
	json.NewEncoder(w).Encode(info)
}

func (c *PluginController) List(w http.ResponseWriter, r *http.Request) {
	integrations, err := c.repo.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(integrations)
}

type IntegrationStatusEntry struct {
	IntegrationID string `json:"integration_id"`
	PluginID      string `json:"plugin_id"`
	Label         string `json:"label"`
	Status        string `json:"status"`
	Message       string `json:"message"`
}

type integrationCheckResult struct {
	Status         string `json:"status"`
	Message        string `json:"message"`
	AuthorizeURL   string `json:"authorize_url,omitempty"`
	State          string `json:"state,omitempty"`
	SourceIdentity string `json:"source_identity,omitempty"`
}

func decodeIntegrationConfig(configJSON string) (map[string]any, error) {
	if strings.TrimSpace(configJSON) == "" {
		return map[string]any{}, nil
	}
	var configMap map[string]any
	if err := json.Unmarshal([]byte(configJSON), &configMap); err != nil {
		return nil, err
	}
	if configMap == nil {
		configMap = map[string]any{}
	}
	return configMap, nil
}

func (c *PluginController) validateIntegrationConfig(ctx context.Context, pluginID string, configMap map[string]any) (*core.Plugin, integrationCheckResult, map[string]any, error) {
	if configMap == nil {
		configMap = map[string]any{}
	}
	normalizedConfig := sourcescope.NormalizeConfig(pluginID, configMap)

	plugin, ok := c.pluginHost.GetPlugin(pluginID)
	if !ok {
		return nil, integrationCheckResult{}, nil, fmt.Errorf("plugin not found: %s", pluginID)
	}
	if err := plugins.ValidateConfig(normalizedConfig, plugin.Manifest.ConfigSchema); err != nil {
		return nil, integrationCheckResult{}, nil, err
	}

	var checkResult integrationCheckResult
	if err := c.pluginHost.Call(ctx, pluginID, "plugin.check_config", map[string]any{
		"config":       normalizedConfig,
		"redirect_uri": c.oauthRedirectURI(pluginID),
	}, &checkResult); err != nil {
		return nil, integrationCheckResult{}, nil, fmt.Errorf("plugin validation failed: %w", err)
	}

	return plugin, checkResult, normalizedConfig, nil
}

func (c *PluginController) writeOAuthRequired(w http.ResponseWriter, pluginID string, checkResult integrationCheckResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":        "oauth_required",
		"plugin_id":     pluginID,
		"authorize_url": checkResult.AuthorizeURL,
		"state":         checkResult.State,
	})
}

func (c *PluginController) Status(w http.ResponseWriter, r *http.Request) {
	integrations, err := c.repo.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	total := len(integrations)
	c.publishNotification("integration_status_run_started", map[string]any{"total": total})

	results := make([]IntegrationStatusEntry, 0, len(integrations))
	for i, integration := range integrations {
		idx := i + 1
		var configMap map[string]any
		if integration.ConfigJSON != "" {
			if err := json.Unmarshal([]byte(integration.ConfigJSON), &configMap); err != nil {
				entry := IntegrationStatusEntry{
					IntegrationID: integration.ID,
					PluginID:      integration.PluginID,
					Label:         integration.Label,
					Status:        "error",
					Message:       "Invalid config JSON",
				}
				results = append(results, entry)
				c.publishNotification("integration_status_checked", map[string]any{
					"index": idx, "total": total,
					"integration_id": entry.IntegrationID, "plugin_id": entry.PluginID, "label": entry.Label,
					"status": entry.Status, "message": entry.Message,
				})
				continue
			}
		}
		if configMap == nil {
			configMap = map[string]any{}
		}
		configMap = sourcescope.NormalizeConfig(integration.PluginID, configMap)
		_, pluginOk := c.pluginHost.GetPlugin(integration.PluginID)
		if !pluginOk {
			entry := IntegrationStatusEntry{
				IntegrationID: integration.ID,
				PluginID:      integration.PluginID,
				Label:         integration.Label,
				Status:        "unavailable",
				Message:       "plugin not found",
			}
			results = append(results, entry)
			c.publishNotification("integration_status_checked", map[string]any{
				"index": idx, "total": total,
				"integration_id": entry.IntegrationID, "plugin_id": entry.PluginID, "label": entry.Label,
				"status": entry.Status, "message": entry.Message,
			})
			continue
		}
		var checkResult integrationCheckResult
		callErr := c.pluginHost.Call(r.Context(), integration.PluginID, "plugin.check_config", map[string]any{
			"config":       configMap,
			"redirect_uri": c.oauthRedirectURI(integration.PluginID),
		}, &checkResult)
		status := "ok"
		message := checkResult.Message
		if callErr != nil {
			status = "error"
			message = callErr.Error()
		} else if checkResult.Status != "" && checkResult.Status != "ok" {
			status = checkResult.Status
		}
		entry := IntegrationStatusEntry{
			IntegrationID: integration.ID,
			PluginID:      integration.PluginID,
			Label:         integration.Label,
			Status:        status,
			Message:       message,
		}
		results = append(results, entry)
		c.publishNotification("integration_status_checked", map[string]any{
			"index": idx, "total": total,
			"integration_id": entry.IntegrationID, "plugin_id": entry.PluginID, "label": entry.Label,
			"status": entry.Status, "message": entry.Message,
		})
	}
	c.publishNotification("integration_status_run_complete", map[string]any{"total": total})
	json.NewEncoder(w).Encode(results)
}

// checkOneIntegration validates a single integration's config via the plugin IPC.
func (c *PluginController) checkOneIntegration(ctx context.Context, integration *core.Integration) IntegrationStatusEntry {
	var configMap map[string]any
	if integration.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(integration.ConfigJSON), &configMap); err != nil {
			return IntegrationStatusEntry{
				IntegrationID: integration.ID,
				PluginID:      integration.PluginID,
				Label:         integration.Label,
				Status:        "error",
				Message:       "Invalid config JSON",
			}
		}
	}
	if configMap == nil {
		configMap = map[string]any{}
	}
	configMap = sourcescope.NormalizeConfig(integration.PluginID, configMap)

	_, pluginOk := c.pluginHost.GetPlugin(integration.PluginID)
	if !pluginOk {
		return IntegrationStatusEntry{
			IntegrationID: integration.ID,
			PluginID:      integration.PluginID,
			Label:         integration.Label,
			Status:        "unavailable",
			Message:       "plugin not found",
		}
	}

	var checkResult integrationCheckResult
	callErr := c.pluginHost.Call(ctx, integration.PluginID, "plugin.check_config", map[string]any{
		"config":       configMap,
		"redirect_uri": c.oauthRedirectURI(integration.PluginID),
	}, &checkResult)

	status := "ok"
	message := checkResult.Message
	if callErr != nil {
		status = "error"
		message = callErr.Error()
	} else if checkResult.Status != "" && checkResult.Status != "ok" {
		status = checkResult.Status
	}

	return IntegrationStatusEntry{
		IntegrationID: integration.ID,
		PluginID:      integration.PluginID,
		Label:         integration.Label,
		Status:        status,
		Message:       message,
	}
}

// StatusOne validates a single integration (GET /api/integrations/{id}/status).
func (c *PluginController) StatusOne(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	integration, err := c.repo.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	entry := c.checkOneIntegration(r.Context(), integration)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry)
}

// StartIntegrationAuth validates one saved integration and returns either the
// current status or the OAuth consent details needed to complete sign-in.
func (c *PluginController) StartIntegrationAuth(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	integration, err := c.repo.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if integration == nil {
		http.NotFound(w, r)
		return
	}

	configMap, err := decodeIntegrationConfig(integration.ConfigJSON)
	if err != nil {
		http.Error(w, "invalid saved config JSON", http.StatusBadRequest)
		return
	}

	_, checkResult, _, err := c.validateIntegrationConfig(r.Context(), integration.PluginID, configMap)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if checkResult.Status == "oauth_required" {
		c.writeOAuthRequired(w, integration.PluginID, checkResult)
		return
	}

	status := "ok"
	message := checkResult.Message
	if checkResult.Status != "" && checkResult.Status != "ok" {
		status = checkResult.Status
		if message == "" {
			message = checkResult.Status
		}
	}
	if status == "ok" && message == "" {
		message = "Integration is already connected."
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(IntegrationStatusEntry{
		IntegrationID: integration.ID,
		PluginID:      integration.PluginID,
		Label:         integration.Label,
		Status:        status,
		Message:       message,
	})
}

// IntegrationGames returns canonical games discovered by a source integration.
func (c *PluginController) IntegrationGames(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	// Verify integration exists.
	if _, err := c.repo.GetByID(r.Context(), id); err != nil {
		http.NotFound(w, r)
		return
	}

	items, err := c.gameStore.GetGamesByIntegrationID(r.Context(), id, 500)
	if err != nil {
		c.logger.Error("GetGamesByIntegrationID failed", err, "integration_id", id)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if items == nil {
		items = []core.GameListItem{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// IntegrationEnrichedGames returns canonical games enriched by a metadata integration's plugin.
func (c *PluginController) IntegrationEnrichedGames(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	// Resolve integration → plugin_id.
	integration, err := c.repo.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	items, err := c.gameStore.GetEnrichedGamesByPluginID(r.Context(), integration.PluginID, 500)
	if err != nil {
		c.logger.Error("GetEnrichedGamesByPluginID failed", err, "plugin_id", integration.PluginID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if items == nil {
		items = []core.GameListItem{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (c *PluginController) findDuplicateIntegration(ctx context.Context, pluginID string, configMap map[string]any, sourceIdentity, excludeIntegrationID string) (*core.Integration, string, error) {
	existing, err := c.repo.ListByPluginID(ctx, pluginID)
	if err != nil {
		return nil, "", err
	}

	candidateJSON, err := json.Marshal(sourcescope.NormalizeConfig(pluginID, configMap))
	if err != nil {
		return nil, "", err
	}

	for _, ex := range existing {
		if ex.ID == excludeIntegrationID {
			continue
		}
		existingConfig, err := decodeIntegrationConfig(ex.ConfigJSON)
		if err != nil {
			continue
		}
		existingJSON, err := json.Marshal(sourcescope.NormalizeConfig(pluginID, existingConfig))
		if err != nil {
			continue
		}
		if configJSONObjectDeepEqual(string(candidateJSON), string(existingJSON)) {
			return ex, duplicateIntegrationMessage(pluginID, ex.Label, false), nil
		}
	}

	if !sourcescope.IsFilesystemBackedPlugin(pluginID) || strings.TrimSpace(sourceIdentity) == "" {
		return nil, "", nil
	}

	for _, ex := range existing {
		if ex.ID == excludeIntegrationID {
			continue
		}
		existingConfig, err := decodeIntegrationConfig(ex.ConfigJSON)
		if err != nil {
			continue
		}
		_, existingCheck, _, err := c.validateIntegrationConfig(ctx, pluginID, existingConfig)
		if err != nil {
			continue
		}
		if strings.TrimSpace(existingCheck.SourceIdentity) == "" {
			continue
		}
		if existingCheck.SourceIdentity == sourceIdentity {
			return ex, duplicateIntegrationMessage(pluginID, ex.Label, true), nil
		}
	}

	return nil, "", nil
}

func duplicateIntegrationMessage(pluginID, label string, sourceIdentityMatch bool) string {
	existingLabel := strings.TrimSpace(label)
	if existingLabel == "" {
		existingLabel = "existing integration"
	}
	if !sourceIdentityMatch {
		return fmt.Sprintf("An integration with identical configuration already exists: %q.", existingLabel)
	}

	switch pluginID {
	case "game-source-smb":
		return fmt.Sprintf("An SMB integration for this backend/share already exists: %q. Edit the existing integration and add include paths there.", existingLabel)
	case "game-source-google-drive":
		return fmt.Sprintf("A Google Drive integration for this account already exists: %q. Edit the existing integration and add include paths there.", existingLabel)
	default:
		return fmt.Sprintf("An integration for this source already exists: %q.", existingLabel)
	}
}

func (c *PluginController) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PluginID        string          `json:"plugin_id"`
		Label           string          `json:"label"`
		IntegrationType string          `json:"integration_type"`
		Config          json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.PluginID == "" {
		http.Error(w, "plugin_id is required", http.StatusBadRequest)
		return
	}
	if body.Label == "" {
		http.Error(w, "label is required", http.StatusBadRequest)
		return
	}
	if body.IntegrationType == "" {
		http.Error(w, "integration_type is required", http.StatusBadRequest)
		return
	}
	var configMap map[string]any
	if len(body.Config) > 0 {
		if err := json.Unmarshal(body.Config, &configMap); err != nil {
			http.Error(w, "config must be a JSON object", http.StatusBadRequest)
			return
		}
	}
	if configMap == nil {
		configMap = map[string]any{}
	}
	_, checkResult, normalizedConfig, err := c.validateIntegrationConfig(r.Context(), body.PluginID, configMap)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// OAuth consent required — return 202 with authorize URL for frontend to open.
	if checkResult.Status == "oauth_required" {
		c.writeOAuthRequired(w, body.PluginID, checkResult)
		return
	}

	if checkResult.Status != "" && checkResult.Status != "ok" {
		msg := checkResult.Message
		if msg == "" {
			msg = checkResult.Status
		}
		http.Error(w, "plugin validation failed: "+msg, http.StatusBadRequest)
		return
	}
	configBytes, err := json.Marshal(normalizedConfig)
	if err != nil {
		http.Error(w, "invalid integration config", http.StatusBadRequest)
		return
	}
	duplicateIntegration, duplicateMessage, err := c.findDuplicateIntegration(r.Context(), body.PluginID, normalizedConfig, checkResult.SourceIdentity, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if duplicateIntegration != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":          "duplicate_integration",
			"message":        duplicateMessage,
			"integration_id": duplicateIntegration.ID,
			"integration":    duplicateIntegration,
		})
		return
	}
	integration := &core.Integration{
		ID:              uuid.New().String(),
		PluginID:        body.PluginID,
		Label:           body.Label,
		IntegrationType: body.IntegrationType,
		ConfigJSON:      string(configBytes),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := c.repo.Create(r.Context(), integration); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.publishNotification("integration_created", map[string]any{
		"integration_id":   integration.ID,
		"plugin_id":        integration.PluginID,
		"label":            integration.Label,
		"integration_type": integration.IntegrationType,
	})
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(integration)
}

func (c *PluginController) UpdateIntegration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	existing, err := c.repo.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.NotFound(w, r)
		return
	}

	var body struct {
		Label           string          `json:"label"`
		IntegrationType string          `json:"integration_type"`
		Config          json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Merge: only update fields that were provided
	if body.Label != "" {
		existing.Label = body.Label
	}
	if body.IntegrationType != "" {
		existing.IntegrationType = body.IntegrationType
	}

	// If config was provided, validate it against the plugin schema
	if len(body.Config) > 0 {
		var configMap map[string]any
		if err := json.Unmarshal(body.Config, &configMap); err != nil {
			http.Error(w, "config must be a JSON object", http.StatusBadRequest)
			return
		}

		_, checkResult, normalizedConfig, err := c.validateIntegrationConfig(r.Context(), existing.PluginID, configMap)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if checkResult.Status == "oauth_required" {
			c.writeOAuthRequired(w, existing.PluginID, checkResult)
			return
		}
		if checkResult.Status != "" && checkResult.Status != "ok" {
			msg := checkResult.Message
			if msg == "" {
				msg = checkResult.Status
			}
			http.Error(w, "plugin validation failed: "+msg, http.StatusBadRequest)
			return
		}

		duplicateIntegration, duplicateMessage, err := c.findDuplicateIntegration(r.Context(), existing.PluginID, normalizedConfig, checkResult.SourceIdentity, existing.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if duplicateIntegration != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":          "duplicate_integration",
				"message":        duplicateMessage,
				"integration_id": duplicateIntegration.ID,
				"integration":    duplicateIntegration,
			})
			return
		}

		configBytes, err := json.Marshal(normalizedConfig)
		if err != nil {
			http.Error(w, "invalid config", http.StatusBadRequest)
			return
		}
		existing.ConfigJSON = string(configBytes)
	}

	existing.UpdatedAt = time.Now()
	if err := c.repo.Update(r.Context(), existing); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c.publishNotification("integration_updated", map[string]any{
		"integration_id":   existing.ID,
		"plugin_id":        existing.PluginID,
		"label":            existing.Label,
		"integration_type": existing.IntegrationType,
	})
	json.NewEncoder(w).Encode(existing)
}

// Browse proxies a source.browse IPC call to the specified plugin.
// POST /api/plugins/{plugin_id}/browse  — body: {"path": "..."}
func (c *PluginController) Browse(w http.ResponseWriter, r *http.Request) {
	pluginID := chi.URLParam(r, "plugin_id")
	if pluginID == "" {
		http.Error(w, "plugin_id is required", http.StatusBadRequest)
		return
	}

	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var result any
	if err := c.pluginHost.Call(r.Context(), pluginID, "source.browse", body, &result); err != nil {
		http.Error(w, "browse failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (c *PluginController) DeleteIntegration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	existing, err := c.repo.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.NotFound(w, r)
		return
	}

	// Cascade-delete source games if this is a source integration.
	if plugin, ok := c.pluginHost.GetPlugin(existing.PluginID); ok {
		for _, cap := range plugin.Manifest.Capabilities {
			if cap == "source" {
				if err := c.gameStore.DeleteGamesByIntegrationID(r.Context(), id); err != nil {
					c.logger.Error("cascade delete games failed", err, "integration_id", id)
					http.Error(w, "failed to delete associated games: "+err.Error(), http.StatusInternalServerError)
					return
				}
				break
			}
		}
	}

	if err := c.repo.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c.publishNotification("integration_deleted", map[string]any{
		"integration_id": existing.ID,
		"plugin_id":      existing.PluginID,
		"label":          existing.Label,
	})
	w.WriteHeader(http.StatusNoContent)
}

// AchievementController serves GET /api/games/{id}/achievements.
type AchievementController struct {
	gameStore          core.GameStore
	pluginHost         plugins.PluginHost
	integrationRepo    core.IntegrationRepository
	achievementFetcher *scan.AchievementFetchService
	logger             core.Logger
	eventBus           *events.EventBus
}

func NewAchievementController(gameStore core.GameStore, pluginHost plugins.PluginHost, integrationRepo core.IntegrationRepository, logger core.Logger, eventBus *events.EventBus) *AchievementController {
	return &AchievementController{
		gameStore:          gameStore,
		pluginHost:         pluginHost,
		integrationRepo:    integrationRepo,
		achievementFetcher: scan.NewAchievementFetchService(gameStore, pluginHost, logger),
		logger:             logger,
		eventBus:           eventBus,
	}
}

type AchievementDTO struct {
	ExternalID   string  `json:"external_id"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	LockedIcon   string  `json:"locked_icon,omitempty"`
	UnlockedIcon string  `json:"unlocked_icon,omitempty"`
	Points       int     `json:"points,omitempty"`
	Rarity       float64 `json:"rarity,omitempty"`
	Unlocked     bool    `json:"unlocked"`
	UnlockedAt   string  `json:"unlocked_at,omitempty"`
}

type AchievementSetDTO struct {
	Source           string           `json:"source"`
	ExternalGameID   string           `json:"external_game_id"`
	SourceGameID     string           `json:"source_game_id,omitempty"`
	SourceTitle      string           `json:"source_title,omitempty"`
	Platform         string           `json:"platform,omitempty"`
	IntegrationID    string           `json:"integration_id,omitempty"`
	IntegrationLabel string           `json:"integration_label,omitempty"`
	PluginID         string           `json:"plugin_id,omitempty"`
	TotalCount       int              `json:"total_count"`
	UnlockedCount    int              `json:"unlocked_count"`
	TotalPoints      int              `json:"total_points,omitempty"`
	EarnedPoints     int              `json:"earned_points,omitempty"`
	Achievements     []AchievementDTO `json:"achievements"`
}

func achievementSetToDTO(set core.AchievementSet) AchievementSetDTO {
	dto := AchievementSetDTO{
		Source:           set.Source,
		ExternalGameID:   set.ExternalGameID,
		SourceGameID:     set.SourceGameID,
		SourceTitle:      set.SourceTitle,
		Platform:         set.Platform,
		IntegrationID:    set.IntegrationID,
		IntegrationLabel: set.IntegrationLabel,
		PluginID:         set.PluginID,
		TotalCount:       set.TotalCount,
		UnlockedCount:    set.UnlockedCount,
		TotalPoints:      set.TotalPoints,
		EarnedPoints:     set.EarnedPoints,
		Achievements:     make([]AchievementDTO, 0, len(set.Achievements)),
	}
	for _, achievement := range set.Achievements {
		item := AchievementDTO{
			ExternalID:   achievement.ExternalID,
			Title:        achievement.Title,
			Description:  achievement.Description,
			LockedIcon:   achievement.LockedIcon,
			UnlockedIcon: achievement.UnlockedIcon,
			Points:       achievement.Points,
			Rarity:       achievement.Rarity,
			Unlocked:     achievement.Unlocked,
		}
		if !achievement.UnlockedAt.IsZero() {
			item.UnlockedAt = achievement.UnlockedAt.UTC().Format(time.RFC3339)
		}
		dto.Achievements = append(dto.Achievements, item)
	}
	return dto
}

type rawAchievementPluginEntry struct {
	ExternalID   string  `json:"external_id"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	LockedIcon   string  `json:"locked_icon"`
	UnlockedIcon string  `json:"unlocked_icon"`
	Points       int     `json:"points"`
	Rarity       float64 `json:"rarity"`
	Unlocked     bool    `json:"unlocked"`
	UnlockedAt   any     `json:"unlocked_at"`
}

type rawAchievementPluginResult struct {
	Source         string                      `json:"source"`
	ExternalGameID string                      `json:"external_game_id"`
	TotalCount     int                         `json:"total_count"`
	UnlockedCount  int                         `json:"unlocked_count"`
	TotalPoints    int                         `json:"total_points"`
	EarnedPoints   int                         `json:"earned_points"`
	Achievements   []rawAchievementPluginEntry `json:"achievements"`
}

type achievementQueryCandidate struct {
	ExternalGameID string
	SourceGameID   string
}

func parseAchievementUnlockedAt(raw any) (time.Time, bool) {
	switch v := raw.(type) {
	case nil:
		return time.Time{}, false
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return time.Time{}, false
		}
		if numeric, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return unixAchievementTime(numeric)
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return parsed.UTC(), true
		}
		if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return parsed.UTC(), true
		}
		if parsed, err := time.Parse("2006-01-02 15:04:05", trimmed); err == nil {
			return parsed.UTC(), true
		}
		return time.Time{}, false
	case float64:
		return unixAchievementTime(int64(v))
	case int64:
		return unixAchievementTime(v)
	case int:
		return unixAchievementTime(int64(v))
	default:
		return time.Time{}, false
	}
}

func unixAchievementTime(raw int64) (time.Time, bool) {
	if raw <= 0 {
		return time.Time{}, false
	}
	seconds := raw
	if raw > 9999999999 {
		seconds = raw / 1000
	}
	return time.Unix(seconds, 0).UTC(), true
}

func normalizeAchievementResult(pluginID, externalGameID string, raw rawAchievementPluginResult, fetchedAt time.Time) (*core.AchievementSet, AchievementSetDTO) {
	source := raw.Source
	if source == "" {
		source = pluginID
	}
	canonicalExternalGameID := raw.ExternalGameID
	if canonicalExternalGameID == "" {
		canonicalExternalGameID = externalGameID
	}

	set := &core.AchievementSet{
		Source:         source,
		ExternalGameID: canonicalExternalGameID,
		Achievements:   make([]core.Achievement, 0, len(raw.Achievements)),
		FetchedAt:      fetchedAt.UTC(),
	}
	dto := AchievementSetDTO{
		Source:         source,
		ExternalGameID: canonicalExternalGameID,
		Achievements:   make([]AchievementDTO, 0, len(raw.Achievements)),
	}

	for _, a := range raw.Achievements {
		unlockedAt, hasUnlockedAt := parseAchievementUnlockedAt(a.UnlockedAt)
		if !a.Unlocked {
			unlockedAt = time.Time{}
			hasUnlockedAt = false
		}

		achievement := core.Achievement{
			ExternalID:   a.ExternalID,
			Title:        a.Title,
			Description:  a.Description,
			LockedIcon:   a.LockedIcon,
			UnlockedIcon: a.UnlockedIcon,
			Points:       a.Points,
			Rarity:       a.Rarity,
			Unlocked:     a.Unlocked,
		}
		if hasUnlockedAt {
			achievement.UnlockedAt = unlockedAt
		}
		set.Achievements = append(set.Achievements, achievement)

		dtoAchievement := AchievementDTO{
			ExternalID:   a.ExternalID,
			Title:        a.Title,
			Description:  a.Description,
			LockedIcon:   a.LockedIcon,
			UnlockedIcon: a.UnlockedIcon,
			Points:       a.Points,
			Rarity:       a.Rarity,
			Unlocked:     a.Unlocked,
		}
		if hasUnlockedAt {
			dtoAchievement.UnlockedAt = unlockedAt.Format(time.RFC3339)
		}
		dto.Achievements = append(dto.Achievements, dtoAchievement)

		set.TotalCount++
		dto.TotalCount++
		if a.Points > 0 {
			set.TotalPoints += a.Points
			dto.TotalPoints += a.Points
		}
		if a.Unlocked {
			set.UnlockedCount++
			dto.UnlockedCount++
			if a.Points > 0 {
				set.EarnedPoints += a.Points
				dto.EarnedPoints += a.Points
			}
		}
	}

	return set, dto
}

func findAchievementCacheSourceGameID(game *core.CanonicalGame, source, externalGameID string) string {
	for _, sg := range game.SourceGames {
		if sg == nil || sg.Status != "found" {
			continue
		}
		if sg.PluginID == source && sg.ExternalID == externalGameID {
			return sg.ID
		}
	}
	for _, sg := range game.SourceGames {
		if sg == nil || sg.Status != "found" {
			continue
		}
		for _, match := range sg.ResolverMatches {
			if match.Outvoted {
				continue
			}
			if match.PluginID == source && match.ExternalID == externalGameID {
				return sg.ID
			}
		}
	}
	return ""
}

func buildAchievementQueryCandidates(game *core.CanonicalGame, pluginIDs []string) map[string]achievementQueryCandidate {
	candidates := make(map[string]achievementQueryCandidate, len(pluginIDs))
	if game == nil || len(pluginIDs) == 0 {
		return candidates
	}

	wanted := make(map[string]struct{}, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		if strings.TrimSpace(pluginID) == "" {
			continue
		}
		wanted[pluginID] = struct{}{}
	}

	// Source plugins map directly from source games in source-game order.
	for _, sg := range game.SourceGames {
		if sg == nil || sg.Status != "found" {
			continue
		}
		if _, ok := wanted[sg.PluginID]; !ok || sg.ExternalID == "" {
			continue
		}
		if _, exists := candidates[sg.PluginID]; exists {
			continue
		}
		candidates[sg.PluginID] = achievementQueryCandidate{
			ExternalGameID: sg.ExternalID,
			SourceGameID:   sg.ID,
		}
	}

	type scoredCandidate struct {
		candidate achievementQueryCandidate
		priority  int
	}

	scored := make(map[string]scoredCandidate, len(pluginIDs))
	for _, sg := range game.SourceGames {
		if sg == nil || sg.Status != "found" {
			continue
		}
		for _, match := range sg.ResolverMatches {
			if _, ok := wanted[match.PluginID]; !ok || match.ExternalID == "" {
				continue
			}
			priority := 1
			if match.ManualSelection {
				priority = 3
			} else if !match.Outvoted {
				priority = 2
			}
			existing, exists := scored[match.PluginID]
			if exists && existing.priority >= priority {
				continue
			}
			scored[match.PluginID] = scoredCandidate{
				candidate: achievementQueryCandidate{
					ExternalGameID: match.ExternalID,
					SourceGameID:   sg.ID,
				},
				priority: priority,
			}
		}
	}

	for pluginID, candidate := range scored {
		candidates[pluginID] = candidate.candidate
	}

	return candidates
}

// GetAchievements fetches achievements on-demand from all capable plugins.
func (c *AchievementController) GetAchievements(w http.ResponseWriter, r *http.Request) {
	gameID, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if gameID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	game, err := c.gameStore.GetCanonicalGameByID(ctx, gameID)
	if err != nil {
		c.logger.Error("get game for achievements", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}

	achievementSources := c.configuredAchievementSources(ctx)
	if len(achievementSources) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}

	fetchedSets, errs := c.achievementFetcher.FetchAndCacheForSources(ctx, game, achievementSources)
	c.enrichAchievementSetContext(ctx, game, fetchedSets)
	sets := make([]AchievementSetDTO, 0, len(fetchedSets))
	for _, set := range fetchedSets {
		if set == nil {
			continue
		}
		sets = append(sets, achievementSetToDTO(*set))
	}
	for errKey, callErr := range errs {
		pluginID := strings.SplitN(errKey, "|", 2)[0]
		c.logger.Error("achievements.game.get failed", callErr, "plugin_id", pluginID, "game_id", gameID)
		events.PublishJSON(c.eventBus, "operation_error", map[string]any{
			"scope":     "achievements",
			"plugin_id": pluginID,
			"game_id":   gameID,
			"error":     callErr.Error(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sets)
}

func (c *AchievementController) enrichAchievementSetContext(ctx context.Context, game *core.CanonicalGame, sets []*core.AchievementSet) {
	if game == nil || len(sets) == 0 {
		return
	}
	labels := map[string]string{}
	if c.integrationRepo != nil {
		if integrations, err := c.integrationRepo.List(ctx); err == nil {
			for _, integration := range integrations {
				if integration != nil {
					labels[integration.ID] = integration.Label
				}
			}
		}
	}
	sourceByID := make(map[string]*core.SourceGame, len(game.SourceGames))
	for _, sourceGame := range game.SourceGames {
		if sourceGame != nil {
			sourceByID[sourceGame.ID] = sourceGame
		}
	}
	for _, set := range sets {
		if set == nil {
			continue
		}
		sourceGame := sourceByID[set.SourceGameID]
		if sourceGame != nil {
			if set.SourceTitle == "" {
				set.SourceTitle = sourceGame.RawTitle
			}
			if set.Platform == "" {
				set.Platform = string(sourceGame.Platform)
			}
			if set.IntegrationID == "" {
				set.IntegrationID = sourceGame.IntegrationID
			}
			if set.PluginID == "" {
				set.PluginID = set.Source
			}
		}
		if set.IntegrationLabel == "" && set.IntegrationID != "" {
			set.IntegrationLabel = labels[set.IntegrationID]
		}
	}
}

func (c *AchievementController) configuredAchievementSources(ctx context.Context) []scan.AchievementSource {
	pluginIDs := c.pluginHost.GetPluginIDsProviding("achievements.game.get")
	if len(pluginIDs) == 0 {
		return nil
	}
	provides := make(map[string]struct{}, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		if strings.TrimSpace(pluginID) != "" {
			provides[pluginID] = struct{}{}
		}
	}

	if c.integrationRepo == nil {
		sources := make([]scan.AchievementSource, 0, len(provides))
		for pluginID := range provides {
			sources = append(sources, scan.AchievementSource{PluginID: pluginID})
		}
		return sources
	}

	integrations, err := c.integrationRepo.List(ctx)
	if err != nil {
		c.logger.Error("list achievement integrations", err)
		return nil
	}
	sources := make([]scan.AchievementSource, 0, len(integrations))
	seen := map[string]struct{}{}
	for _, integration := range integrations {
		if integration == nil {
			continue
		}
		if _, ok := provides[integration.PluginID]; !ok {
			continue
		}
		configMap, err := decodeIntegrationConfig(integration.ConfigJSON)
		if err != nil {
			c.logger.Error("decode achievement integration config", err, "integration_id", integration.ID, "plugin_id", integration.PluginID)
			continue
		}
		key := integration.ID
		if key == "" {
			key = integration.PluginID
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		sources = append(sources, scan.AchievementSource{
			IntegrationID: integration.ID,
			Label:         integration.Label,
			PluginID:      integration.PluginID,
			Config:        configMap,
		})
	}
	return sources
}
