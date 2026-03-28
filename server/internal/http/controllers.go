// Package http implements the server's public HTTP API.
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
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
	gameStore core.GameStore
	logger    core.Logger
}

func NewGameController(gameStore core.GameStore, logger core.Logger) *GameController {
	return &GameController{gameStore: gameStore, logger: logger}
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

	out := make([]GameDetailResponse, 0, len(games))
	for _, cg := range games {
		if cg == nil {
			continue
		}
		out = append(out, canonicalToGameDetail(cg))
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
	json.NewEncoder(w).Encode(canonicalToGameDetail(game))
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
	json.NewEncoder(w).Encode(canonicalToGameDetail(game))
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
	orchestrator *scan.Orchestrator
	gameStore    core.GameStore
	logger       core.Logger
}

func NewDiscoveryController(orchestrator *scan.Orchestrator, gameStore core.GameStore, logger core.Logger) *DiscoveryController {
	return &DiscoveryController{orchestrator: orchestrator, gameStore: gameStore, logger: logger}
}

type ScanRequest struct {
	GameSources  []string `json:"game_sources"`
	MetadataOnly bool     `json:"metadata_only"`
}

// ScanResultDTO is the response for POST /api/scan.
type ScanResultDTO struct {
	Status string        `json:"status"`
	Games  []GameSummary `json:"games"`
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

	// Detach from the HTTP request timeout — scans can take many minutes
	// due to metadata enrichment across multiple resolvers.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	var canonical []*core.CanonicalGame
	var err error

	if body.MetadataOnly {
		canonical, err = c.orchestrator.RunMetadataRefresh(ctx, integrationIDs)
	} else {
		canonical, err = c.orchestrator.RunScan(ctx, integrationIDs)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	summaries := make([]GameSummary, 0, len(canonical))
	for _, cg := range canonical {
		summaries = append(summaries, canonicalToSummary(cg))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ScanResultDTO{
		Status: "scan completed",
		Games:  summaries,
	})
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
	port := c.config.Get("PORT")
	if port == "" {
		port = "8900"
	}
	return fmt.Sprintf("http://localhost:%s/api/auth/callback/%s", port, pluginID)
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
		var checkResult struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		}
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

	var checkResult struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
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
	plugin, ok := c.pluginHost.GetPlugin(body.PluginID)
	if !ok {
		http.Error(w, "unknown plugin_id: "+body.PluginID, http.StatusBadRequest)
		return
	}
	if err := plugins.ValidateConfig(configMap, plugin.Manifest.ConfigSchema); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	checkParams := map[string]any{
		"config":       configMap,
		"redirect_uri": c.oauthRedirectURI(body.PluginID),
	}
	var checkResult struct {
		Status       string `json:"status"`
		Message      string `json:"message"`
		AuthorizeURL string `json:"authorize_url,omitempty"`
		State        string `json:"state,omitempty"`
	}
	if err := c.pluginHost.Call(r.Context(), body.PluginID, "plugin.check_config", checkParams, &checkResult); err != nil {
		http.Error(w, "plugin validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// OAuth consent required — return 202 with authorize URL for frontend to open.
	if checkResult.Status == "oauth_required" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"status":        "oauth_required",
			"plugin_id":     body.PluginID,
			"authorize_url": checkResult.AuthorizeURL,
			"state":         checkResult.State,
		})
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
	configBytes, err := json.Marshal(configMap)
	if err != nil {
		http.Error(w, "invalid integration config", http.StatusBadRequest)
		return
	}
	existing, err := c.repo.ListByPluginID(r.Context(), body.PluginID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, ex := range existing {
		if configJSONObjectDeepEqual(string(configBytes), ex.ConfigJSON) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":          "duplicate_integration",
				"integration_id": ex.ID,
				"integration":    ex,
			})
			return
		}
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

		plugin, ok := c.pluginHost.GetPlugin(existing.PluginID)
		if !ok {
			http.Error(w, "plugin not found: "+existing.PluginID, http.StatusBadRequest)
			return
		}
		if err := plugins.ValidateConfig(configMap, plugin.Manifest.ConfigSchema); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validate via IPC
		var checkResult struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		}
		if err := c.pluginHost.Call(r.Context(), existing.PluginID, "plugin.check_config", map[string]any{
			"config":       configMap,
			"redirect_uri": c.oauthRedirectURI(existing.PluginID),
		}, &checkResult); err != nil {
			http.Error(w, "plugin validation failed: "+err.Error(), http.StatusBadRequest)
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

		configBytes, err := json.Marshal(configMap)
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
	gameStore  core.GameStore
	pluginHost plugins.PluginHost
	logger     core.Logger
	eventBus   *events.EventBus
}

func NewAchievementController(gameStore core.GameStore, pluginHost plugins.PluginHost, logger core.Logger, eventBus *events.EventBus) *AchievementController {
	return &AchievementController{gameStore: gameStore, pluginHost: pluginHost, logger: logger, eventBus: eventBus}
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
	Source         string           `json:"source"`
	ExternalGameID string           `json:"external_game_id"`
	TotalCount     int              `json:"total_count"`
	UnlockedCount  int              `json:"unlocked_count"`
	TotalPoints    int              `json:"total_points,omitempty"`
	EarnedPoints   int              `json:"earned_points,omitempty"`
	Achievements   []AchievementDTO `json:"achievements"`
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

	achPlugins := c.pluginHost.GetPluginIDsProviding("achievements.game.get")
	if len(achPlugins) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}

	externalBySource := make(map[string]string) // plugin_id -> external_id
	for _, eid := range game.ExternalIDs {
		externalBySource[eid.Source] = eid.ExternalID
	}

	toQuery := 0
	for _, pluginID := range achPlugins {
		if _, ok := externalBySource[pluginID]; ok {
			toQuery++
		}
	}
	checked := 0

	var sets []AchievementSetDTO
	for _, pluginID := range achPlugins {
		extID, ok := externalBySource[pluginID]
		if !ok {
			continue
		}

		var result rawAchievementPluginResult

		params := map[string]any{"external_game_id": extID}
		checked++
		if err := c.pluginHost.Call(ctx, pluginID, "achievements.game.get", params, &result); err != nil {
			c.logger.Error("achievements.game.get failed", err, "plugin_id", pluginID, "game_id", gameID)
			events.PublishJSON(c.eventBus, "operation_error", map[string]any{
				"scope":     "achievements",
				"plugin_id": pluginID,
				"game_id":   gameID,
				"error":     err.Error(),
				"index":     checked,
				"total":     toQuery,
			})
			continue
		}

		set, dto := normalizeAchievementResult(pluginID, extID, result, time.Now())
		if sourceGameID := findAchievementCacheSourceGameID(game, set.Source, set.ExternalGameID); sourceGameID != "" {
			if err := c.gameStore.CacheAchievements(ctx, sourceGameID, set); err != nil {
				c.logger.Error("cache achievements", err, "plugin_id", pluginID, "game_id", gameID, "source_game_id", sourceGameID)
			}
		}
		sets = append(sets, dto)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sets)
}
