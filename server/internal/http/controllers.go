// Package http implements the server's public HTTP API.
package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ListGamesResponse is the response for GET /api/games.
type ListGamesResponse struct {
	Games []GameSummary `json:"games"`
}

// GameSummary is one game in the list response.
type GameSummary struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	Platform     string           `json:"platform"`
	Kind         string           `json:"kind"`
	ParentGameID string           `json:"parent_game_id,omitempty"`
	GroupKind    string           `json:"group_kind"`
	RootPath     string           `json:"root_path,omitempty"`
	Files        []GameFileDTO    `json:"files,omitempty"`
	ExternalIDs  []ExternalIDDTO  `json:"external_ids,omitempty"`
}

// ExternalIDDTO is a reference to an external metadata database.
type ExternalIDDTO struct {
	Source     string `json:"source"`
	ExternalID string `json:"external_id"`
	URL        string `json:"url,omitempty"`
}

// GameFileDTO is a file belonging to a game.
type GameFileDTO struct {
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

// ListGames returns all canonical games (GET /api/games).
func (c *GameController) ListGames(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	games, err := c.gameStore.GetCanonicalGames(ctx)
	if err != nil {
		c.logger.Error("get games", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	summaries := make([]GameSummary, 0, len(games))
	for _, cg := range games {
		if cg == nil {
			continue
		}
		summaries = append(summaries, canonicalToSummary(cg))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ListGamesResponse{Games: summaries})
}

// Get returns one canonical game by ID (GET /api/games/{id}). 404 if not found.
func (c *GameController) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
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
	json.NewEncoder(w).Encode(canonicalToSummary(game))
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
		ID:        cg.ID,
		Title:     cg.Title,
		Platform:  string(cg.Platform),
		Kind:      string(cg.Kind),
		GroupKind: "",
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
	logger       core.Logger
}

func NewDiscoveryController(orchestrator *scan.Orchestrator, logger core.Logger) *DiscoveryController {
	return &DiscoveryController{orchestrator: orchestrator, logger: logger}
}

type ScanRequest struct {
	GameSources []string `json:"game_sources"`
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
	canonical, err := c.orchestrator.RunScan(r.Context(), integrationIDs)
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
	logger     core.Logger
}

func NewPluginController(repo core.IntegrationRepository, pluginHost plugins.PluginHost, logger core.Logger) *PluginController {
	return &PluginController{repo: repo, pluginHost: pluginHost, logger: logger}
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
	results := make([]IntegrationStatusEntry, 0, len(integrations))
	for _, integration := range integrations {
		var configMap map[string]any
		if integration.ConfigJSON != "" {
			if err := json.Unmarshal([]byte(integration.ConfigJSON), &configMap); err != nil {
				results = append(results, IntegrationStatusEntry{
					IntegrationID: integration.ID,
					PluginID:     integration.PluginID,
					Label:        integration.Label,
					Status:       "error",
					Message:      "Invalid config JSON",
				})
				continue
			}
		}
		if configMap == nil {
			configMap = map[string]any{}
		}
		_, pluginOk := c.pluginHost.GetPlugin(integration.PluginID)
		if !pluginOk {
			results = append(results, IntegrationStatusEntry{
				IntegrationID: integration.ID,
				PluginID:     integration.PluginID,
				Label:        integration.Label,
				Status:       "unavailable",
				Message:      "plugin not found",
			})
			continue
		}
		var checkResult struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		}
		callErr := c.pluginHost.Call(r.Context(), integration.PluginID, "plugin.check_config", configMap, &checkResult)
		status := "ok"
		message := checkResult.Message
		if callErr != nil {
			status = "error"
			message = callErr.Error()
		} else if checkResult.Status != "" && checkResult.Status != "ok" {
			status = checkResult.Status
		}
		results = append(results, IntegrationStatusEntry{
			IntegrationID: integration.ID,
			PluginID:     integration.PluginID,
			Label:        integration.Label,
			Status:       status,
			Message:      message,
		})
	}
	json.NewEncoder(w).Encode(results)
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
	var checkResult struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := c.pluginHost.Call(r.Context(), body.PluginID, "plugin.check_config", configMap, &checkResult); err != nil {
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
		http.Error(w, "invalid integration config", http.StatusBadRequest)
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
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(integration)
}

// AchievementController serves GET /api/games/{id}/achievements.
type AchievementController struct {
	gameStore  core.GameStore
	pluginHost plugins.PluginHost
	logger     core.Logger
}

func NewAchievementController(gameStore core.GameStore, pluginHost plugins.PluginHost, logger core.Logger) *AchievementController {
	return &AchievementController{gameStore: gameStore, pluginHost: pluginHost, logger: logger}
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
	ExternalGameID string          `json:"external_game_id"`
	TotalCount     int             `json:"total_count"`
	UnlockedCount  int             `json:"unlocked_count"`
	TotalPoints    int             `json:"total_points,omitempty"`
	EarnedPoints   int             `json:"earned_points,omitempty"`
	Achievements   []AchievementDTO `json:"achievements"`
}

// GetAchievements fetches achievements on-demand from all capable plugins.
func (c *AchievementController) GetAchievements(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "id")
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

	var sets []AchievementSetDTO
	for _, pluginID := range achPlugins {
		extID, ok := externalBySource[pluginID]
		if !ok {
			continue
		}

		var result struct {
			Source         string `json:"source"`
			ExternalGameID string `json:"external_game_id"`
			TotalCount     int    `json:"total_count"`
			UnlockedCount  int    `json:"unlocked_count"`
			TotalPoints    int    `json:"total_points"`
			EarnedPoints   int    `json:"earned_points"`
			Achievements   []struct {
				ExternalID   string  `json:"external_id"`
				Title        string  `json:"title"`
				Description  string  `json:"description"`
				LockedIcon   string  `json:"locked_icon"`
				UnlockedIcon string  `json:"unlocked_icon"`
				Points       int     `json:"points"`
				Rarity       float64 `json:"rarity"`
				Unlocked     bool    `json:"unlocked"`
				UnlockedAt   any     `json:"unlocked_at"`
			} `json:"achievements"`
		}

		params := map[string]any{"external_game_id": extID}
		if err := c.pluginHost.Call(ctx, pluginID, "achievements.game.get", params, &result); err != nil {
			c.logger.Error("achievements.game.get failed", err, "plugin_id", pluginID, "game_id", gameID)
			continue
		}

		dto := AchievementSetDTO{
			Source:         result.Source,
			ExternalGameID: result.ExternalGameID,
			TotalCount:     result.TotalCount,
			UnlockedCount:  result.UnlockedCount,
			TotalPoints:    result.TotalPoints,
			EarnedPoints:   result.EarnedPoints,
		}
		for _, a := range result.Achievements {
			unlockedAt := ""
			switch v := a.UnlockedAt.(type) {
			case string:
				unlockedAt = v
			case float64:
				if v > 0 {
					unlockedAt = fmt.Sprintf("%d", int64(v))
				}
			}
			dto.Achievements = append(dto.Achievements, AchievementDTO{
				ExternalID:   a.ExternalID,
				Title:        a.Title,
				Description:  a.Description,
				LockedIcon:   a.LockedIcon,
				UnlockedIcon: a.UnlockedIcon,
				Points:       a.Points,
				Rarity:       a.Rarity,
				Unlocked:     a.Unlocked,
				UnlockedAt:   unlockedAt,
			})
		}
		sets = append(sets, dto)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sets)
}
