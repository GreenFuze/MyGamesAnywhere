// Package http implements the server's public HTTP API.
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ListGamesResponse is the response for GET /api/games.
type ListGamesResponse struct {
	Games []GameSummary `json:"games"`
}

// GameSummary is one game in the list response.
type GameSummary struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	Platform      string         `json:"platform"`
	Kind          string         `json:"kind"`
	ParentGameID  string         `json:"parent_game_id,omitempty"`
	GroupKind     string         `json:"group_kind"`
	RootPath      string         `json:"root_path,omitempty"`
	Confidence    string         `json:"confidence,omitempty"`
	Files         []GameFileDTO  `json:"files,omitempty"`
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
	gameRepo core.GameRepository
	logger   core.Logger
}

func NewGameController(gameRepo core.GameRepository, logger core.Logger) *GameController {
	return &GameController{gameRepo: gameRepo, logger: logger}
}

// ListGames returns all games (GET /api/games).
func (c *GameController) ListGames(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	games, err := c.gameRepo.GetGames(ctx)
	if err != nil {
		c.logger.Error("get games", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	summaries := make([]GameSummary, 0, len(games))
	for _, g := range games {
		if g == nil {
			continue
		}
		files, _ := c.gameRepo.GetGameFiles(ctx, g.ID)
		summaries = append(summaries, gameToSummary(g, files))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ListGamesResponse{Games: summaries})
}

// Get returns one game by ID (GET /api/games/{id}). 404 if not found.
func (c *GameController) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	game, err := c.gameRepo.GetGameByID(ctx, id)
	if err != nil {
		c.logger.Error("get game", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	files, _ := c.gameRepo.GetGameFiles(ctx, id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gameToSummary(game, files))
}

// DeleteAll removes all games and their files (DELETE /api/games).
func (c *GameController) DeleteAll(w http.ResponseWriter, r *http.Request) {
	if err := c.gameRepo.DeleteAllGames(r.Context()); err != nil {
		c.logger.Error("delete all games", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "all games deleted"})
}

func gameToSummary(g *core.Game, files []*core.GameFile) GameSummary {
	s := GameSummary{
		ID:          g.ID,
		Title:       g.Title,
		Platform:    string(g.Platform),
		Kind:        string(g.Kind),
		ParentGameID: g.ParentGameID,
		GroupKind:   string(g.GroupKind),
		RootPath:    g.RootPath,
		Confidence:  g.Confidence,
	}
	for _, f := range files {
		if f != nil {
			s.Files = append(s.Files, GameFileDTO{
				Path:     f.Path,
				Role:     string(f.Role),
				FileKind: f.FileKind,
				Size:     f.Size,
			})
		}
	}
	return s
}

type DiscoveryController struct {
	scanSvc plugins.ScanService
	logger  core.Logger
}

func NewDiscoveryController(scanSvc plugins.ScanService, logger core.Logger) *DiscoveryController {
	return &DiscoveryController{scanSvc: scanSvc, logger: logger}
}

type ScanRequest struct {
	GameSources []string `json:"game_sources"`
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
	if err := c.scanSvc.RunScan(r.Context(), integrationIDs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "scan completed"})
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
