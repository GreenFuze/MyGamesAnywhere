package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan/scanner"
)

const (
	sourceFilesystemListMethod = "source.filesystem.list"
	sourceGamesListMethod      = "source.games.list"
)

// PluginCaller is the subset of PluginHost that the orchestrator needs.
type PluginCaller interface {
	Call(ctx context.Context, pluginID string, method string, params any, result any) error
}

// PluginDiscovery lets the orchestrator find which plugins provide which methods.
type PluginDiscovery interface {
	GetPluginIDs() []string
	GetPlugin(pluginID string) (*core.Plugin, bool)
	GetPluginIDsProviding(method string) []string
}

// Orchestrator coordinates a game scan: it fetches files from each
// source plugin, runs the scanner pipeline, enriches with metadata,
// and persists results via GameStore.
type Orchestrator struct {
	pluginCaller     PluginCaller
	pluginDiscovery  PluginDiscovery
	integrationRepo  core.IntegrationRepository
	gameStore        core.GameStore
	scanner          *scanner.Scanner
	metadataResolver *MetadataResolver
	logger           core.Logger
	eventBus         *events.EventBus
}

func NewOrchestrator(
	caller PluginCaller,
	discovery PluginDiscovery,
	integrationRepo core.IntegrationRepository,
	gameStore core.GameStore,
	logger core.Logger,
) *Orchestrator {
	return &Orchestrator{
		pluginCaller:     caller,
		pluginDiscovery:  discovery,
		integrationRepo:  integrationRepo,
		gameStore:        gameStore,
		scanner:          scanner.New(logger),
		metadataResolver: NewMetadataResolver(caller, logger),
		logger:           logger,
	}
}

// SetEventBus attaches an optional event bus for scan progress SSE.
func (o *Orchestrator) SetEventBus(bus *events.EventBus) {
	o.eventBus = bus
	if bus == nil {
		o.metadataResolver.SetScanEventPublisher(nil)
		return
	}
	o.metadataResolver.SetScanEventPublisher(func(typ string, payload any) {
		o.publishEvent(typ, payload)
	})
}

func (o *Orchestrator) publishEvent(eventType string, payload any) {
	if o.eventBus == nil {
		return
	}
	if m, ok := payload.(map[string]any); ok {
		events.PublishJSON(o.eventBus, eventType, m)
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		o.logger.Warn("orchestrator: event marshal failed", "error", err)
		return
	}
	o.eventBus.Publish(events.Event{Type: eventType, Data: data})
}

func (o *Orchestrator) publishScanError(integrationID string, err error) {
	if err == nil {
		return
	}
	m := map[string]any{"error": err.Error()}
	if integrationID != "" {
		m["integration_id"] = integrationID
	}
	o.publishEvent("scan_error", m)
}

// RunScan scans all (or selected) integrations, enriches metadata,
// persists via GameStore, and returns the canonical game views.
func (o *Orchestrator) RunScan(ctx context.Context, integrationIDs []string) ([]*core.CanonicalGame, error) {
	o.logger.Info("orchestrator: starting scan", "requested", len(integrationIDs))

	integrations, err := o.integrationRepo.List(ctx)
	if err != nil {
		o.publishScanError("", err)
		return nil, fmt.Errorf("list integrations: %w", err)
	}

	filter := make(map[string]bool, len(integrationIDs))
	for _, id := range integrationIDs {
		filter[id] = true
	}
	filterActive := len(filter) > 0

	integrationCount := 0
	for _, integ := range integrations {
		if filterActive && !filter[integ.ID] {
			continue
		}
		integrationCount++
	}
	o.publishEvent("scan_started", map[string]any{"integration_count": integrationCount})

	scanStart := time.Now()
	metaSources := o.findMetadataSources(integrations)

	for _, integ := range integrations {
		if filterActive && !filter[integ.ID] {
			continue
		}
		plugin, ok := o.pluginDiscovery.GetPlugin(integ.PluginID)
		if !ok {
			o.logger.Warn("orchestrator: plugin not found", "integration_id", integ.ID, "plugin_id", integ.PluginID)
			o.publishEvent("scan_integration_skipped", map[string]any{
				"integration_id": integ.ID,
				"plugin_id":      integ.PluginID,
				"label":          integ.Label,
				"reason":         "plugin_not_found",
			})
			continue
		}

		config, err := parseConfig(integ.ConfigJSON)
		if err != nil {
			o.logger.Warn("orchestrator: bad config", "integration_id", integ.ID, "error", err)
			o.publishEvent("scan_integration_skipped", map[string]any{
				"integration_id": integ.ID,
				"plugin_id":      integ.PluginID,
				"label":          integ.Label,
				"reason":         "invalid_config",
				"error":          err.Error(),
			})
			continue
		}

		o.publishEvent("scan_integration_started", map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
			"label":          integ.Label,
		})

		var games []*core.Game

		switch {
		case pluginProvides(plugin, sourceFilesystemListMethod):
			o.publishEvent("scan_source_list_started", map[string]any{
				"integration_id": integ.ID,
				"plugin_id":      integ.PluginID,
			})
			files, err := o.fetchFiles(ctx, integ.PluginID, config)
			if err != nil {
				o.publishScanError(integ.ID, err)
				return nil, fmt.Errorf("fetch files from integration %q: %w", integ.ID, err)
			}
			o.logger.Info("orchestrator: fetched files", "integration_id", integ.ID, "count", len(files))
			o.publishEvent("scan_source_list_complete", map[string]any{
				"integration_id": integ.ID,
				"plugin_id":      integ.PluginID,
				"file_count":     len(files),
			})

			o.publishEvent("scan_scanner_started", map[string]any{
				"integration_id": integ.ID,
				"file_count":     len(files),
			})
			groups, err := o.scanner.ScanFiles(ctx, files)
			if err != nil {
				o.publishScanError(integ.ID, err)
				return nil, fmt.Errorf("scan files for integration %q: %w", integ.ID, err)
			}
			o.publishEvent("scan_scanner_complete", map[string]any{
				"integration_id": integ.ID,
				"group_count":    len(groups),
			})

			games = buildGames(integ.ID, integ.PluginID, groups)
			o.logger.Info("orchestrator: built games", "integration_id", integ.ID, "games", len(games))

		case pluginProvides(plugin, sourceGamesListMethod):
			o.publishEvent("scan_source_list_started", map[string]any{
				"integration_id": integ.ID,
				"plugin_id":      integ.PluginID,
			})
			games, err = o.fetchGames(ctx, integ.ID, integ.PluginID, config)
			if err != nil {
				o.publishScanError(integ.ID, err)
				return nil, fmt.Errorf("fetch games from integration %q: %w", integ.ID, err)
			}
			o.logger.Info("orchestrator: fetched storefront games", "integration_id", integ.ID, "count", len(games))
			o.publishEvent("scan_source_list_complete", map[string]any{
				"integration_id": integ.ID,
				"plugin_id":      integ.PluginID,
				"game_count":     len(games),
			})

		default:
			o.publishEvent("scan_integration_skipped", map[string]any{
				"integration_id": integ.ID,
				"plugin_id":      integ.PluginID,
				"label":          integ.Label,
				"reason":         "no_source_capability",
			})
			continue
		}

		if len(games) == 0 {
			o.publishEvent("scan_integration_skipped", map[string]any{
				"integration_id": integ.ID,
				"plugin_id":      integ.PluginID,
				"label":          integ.Label,
				"reason":         "no_games",
			})
			continue
		}

		// Metadata enrichment per-integration.
		if len(metaSources) > 0 {
			o.publishEvent("scan_metadata_started", map[string]any{
				"integration_id": integ.ID,
				"game_count":     len(games),
				"resolver_count": len(metaSources),
			})
			o.metadataResolver.Enrich(ctx, integ.ID, games, metaSources)
		}

		// Convert enriched games → ScanBatch and persist.
		o.publishEvent("scan_persist_started", map[string]any{
			"integration_id":    integ.ID,
			"source_game_count": len(games),
		})
		batch := gamesToScanBatch(integ.ID, integ.PluginID, games)
		if err := o.gameStore.PersistScanResults(ctx, batch); err != nil {
			o.publishScanError(integ.ID, err)
			return nil, fmt.Errorf("persist scan results for integration %q: %w", integ.ID, err)
		}
		o.logger.Info("orchestrator: persisted", "integration_id", integ.ID, "source_games", len(batch.SourceGames))
		o.publishEvent("scan_integration_complete", map[string]any{
			"integration_id": integ.ID,
			"games_found":    len(batch.SourceGames),
		})
	}

	// Return the canonical game views.
	result, err := o.gameStore.GetCanonicalGames(ctx)
	if err != nil {
		o.publishScanError("", err)
		return nil, fmt.Errorf("get canonical games: %w", err)
	}

	o.logger.Info("orchestrator: scan complete", "canonical_games", len(result))
	o.publishEvent("scan_complete", map[string]any{
		"canonical_games": len(result),
		"duration_ms":     time.Since(scanStart).Milliseconds(),
	})
	return result, nil
}

// fetchFiles calls source.filesystem.list on the plugin and parses the response.
func (o *Orchestrator) fetchFiles(ctx context.Context, pluginID string, config map[string]any) ([]core.FileEntry, error) {
	var result struct {
		Files []struct {
			Path    string `json:"path"`
			Name    string `json:"name"`
			IsDir   bool   `json:"is_dir"`
			Size    int64  `json:"size"`
			ModTime string `json:"mod_time"`
		} `json:"files"`
	}
	if err := o.pluginCaller.Call(ctx, pluginID, sourceFilesystemListMethod, config, &result); err != nil {
		return nil, err
	}

	entries := make([]core.FileEntry, 0, len(result.Files))
	for _, f := range result.Files {
		var modTime time.Time
		if f.ModTime != "" {
			modTime, _ = time.Parse(time.RFC3339, f.ModTime)
		}
		entries = append(entries, core.FileEntry{
			Path:    f.Path,
			Name:    f.Name,
			IsDir:   f.IsDir,
			Size:    f.Size,
			ModTime: modTime,
		})
	}
	return entries, nil
}

// fetchGames calls source.games.list on a storefront plugin and converts
// the result directly into core.Game entities. Storefront games arrive
// pre-identified with title, platform, and external IDs, so they skip
// the file scanner entirely.
func (o *Orchestrator) fetchGames(ctx context.Context, integrationID, pluginID string, config map[string]any) ([]*core.Game, error) {
	type ipcMedia struct {
		Type     string `json:"type"`
		URL      string `json:"url"`
		Width    int    `json:"width,omitempty"`
		Height   int    `json:"height,omitempty"`
		MimeType string `json:"mime_type,omitempty"`
	}
	var result struct {
		Games []struct {
			ExternalID      string     `json:"external_id"`
			Title           string     `json:"title"`
			Platform        string     `json:"platform,omitempty"`
			URL             string     `json:"url,omitempty"`
			Description     string     `json:"description,omitempty"`
			ReleaseDate     string     `json:"release_date,omitempty"`
			Genres          []string   `json:"genres,omitempty"`
			Developer       string     `json:"developer,omitempty"`
			Publisher       string     `json:"publisher,omitempty"`
			Media           []ipcMedia `json:"media,omitempty"`
			PlaytimeMinutes int        `json:"playtime_minutes,omitempty"`
			IsGamePass      bool       `json:"is_game_pass,omitempty"`
			XcloudAvailable bool       `json:"xcloud_available,omitempty"`
			StoreProductID  string     `json:"store_product_id,omitempty"`
			XcloudURL       string     `json:"xcloud_url,omitempty"`
		} `json:"games"`
	}
	if err := o.pluginCaller.Call(ctx, pluginID, sourceGamesListMethod, config, &result); err != nil {
		return nil, err
	}

	now := time.Now()
	games := make([]*core.Game, 0, len(result.Games))
	for _, sg := range result.Games {
		if sg.Title == "" || sg.ExternalID == "" {
			continue
		}

		gameID := deterministicID(integrationID, pluginID, sg.ExternalID)
		platform := core.Platform("windows_pc")
		if sg.Platform != "" {
			platform = core.Platform(sg.Platform)
		}

		media := make([]core.MediaItem, 0, len(sg.Media))
		for _, mi := range sg.Media {
			media = append(media, core.MediaItem{
				Type:     core.MediaType(mi.Type),
				URL:      mi.URL,
				Width:    mi.Width,
				Height:   mi.Height,
				MimeType: mi.MimeType,
				Source:   pluginID,
			})
		}

		g := &core.Game{
			ID:            gameID,
			Title:         sg.Title,
			RawTitle:      sg.Title,
			Platform:      platform,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			IntegrationID: integrationID,
			Status:        "identified",
			LastSeenAt:    &now,
			ExternalIDs: []core.ExternalID{{
				Source:     pluginID,
				ExternalID: sg.ExternalID,
				URL:        sg.URL,
			}},
			ResolverMatches: []core.ResolverMatch{{
				PluginID:        pluginID,
				Title:           sg.Title,
				Platform:        string(platform),
				ExternalID:      sg.ExternalID,
				URL:             sg.URL,
				Description:     sg.Description,
				ReleaseDate:     sg.ReleaseDate,
				Genres:          sg.Genres,
				Developer:       sg.Developer,
				Publisher:       sg.Publisher,
				Media:           media,
				IsGamePass:      sg.IsGamePass,
				XcloudAvailable: sg.XcloudAvailable,
				StoreProductID:  sg.StoreProductID,
				XcloudURL:       sg.XcloudURL,
			}},
			Description: sg.Description,
			ReleaseDate: sg.ReleaseDate,
			Genres:      sg.Genres,
			Developer:   sg.Developer,
			Publisher:   sg.Publisher,
			Media:       media,
		}
		games = append(games, g)
	}
	return games, nil
}

// buildGames converts scanner GameGroups into core.Game entities.
// Each group becomes one Game; files carry over with their roles.
func buildGames(integrationID, pluginID string, groups []scanner.GameGroup) []*core.Game {
	now := time.Now()
	games := make([]*core.Game, 0, len(groups))

	for _, g := range groups {
		gameID := deterministicID(integrationID, g.RootDir, g.Name)

		files := make([]core.GameFile, 0, len(g.Files))
		for _, af := range g.Files {
			files = append(files, core.GameFile{
				GameID:   gameID,
				Path:     af.Path,
				FileName: af.Name,
				Role:     af.Role,
				FileKind: string(af.Kind),
				Size:     af.Size,
				IsDir:    af.IsDir,
			})
		}

		games = append(games, &core.Game{
			ID:            gameID,
			Title:         g.Name,
			RawTitle:      g.Name,
			Platform:      g.Platform,
			Kind:          core.GameKindBaseGame,
			GroupKind:     g.GroupKind,
			RootPath:      g.RootDir,
			IntegrationID: integrationID,
			Status:        "found",
			LastSeenAt:    &now,
			Files:         files,
		})
	}
	return games
}

// gamesToScanBatch converts the in-memory enriched Game list into a ScanBatch
// ready for persistence. Each Game becomes a SourceGame; resolver matches
// and media are split out into their own maps.
func gamesToScanBatch(integrationID, pluginID string, games []*core.Game) *core.ScanBatch {
	batch := &core.ScanBatch{
		IntegrationID:   integrationID,
		SourceGames:     make([]*core.SourceGame, 0, len(games)),
		ResolverMatches: make(map[string][]core.ResolverMatch),
		MediaItems:      make(map[string][]core.MediaRef),
	}

	for _, g := range games {
		extID := g.ID
		if len(g.ExternalIDs) > 0 {
			extID = g.ExternalIDs[0].ExternalID
		}

		sg := &core.SourceGame{
			ID:            g.ID,
			IntegrationID: integrationID,
			PluginID:      pluginID,
			ExternalID:    extID,
			RawTitle:      g.RawTitle,
			Platform:      g.Platform,
			Kind:          g.Kind,
			GroupKind:     g.GroupKind,
			RootPath:      g.RootPath,
			Status:        g.Status,
			LastSeenAt:    g.LastSeenAt,
			Files:         g.Files,
		}
		batch.SourceGames = append(batch.SourceGames, sg)

		if len(g.ResolverMatches) > 0 {
			batch.ResolverMatches[g.ID] = g.ResolverMatches
		}

		// Collect media from all resolver matches + game-level media.
		var refs []core.MediaRef
		seen := map[string]bool{}
		addMedia := func(mi core.MediaItem) {
			if mi.URL == "" || seen[mi.URL] {
				return
			}
			seen[mi.URL] = true
			refs = append(refs, core.MediaRef{
				Type:   mi.Type,
				URL:    mi.URL,
				Source: mi.Source,
				Width:  mi.Width,
				Height: mi.Height,
			})
		}
		for _, mi := range g.Media {
			addMedia(mi)
		}
		for _, m := range g.ResolverMatches {
			for _, mi := range m.Media {
				addMedia(mi)
			}
		}
		if len(refs) > 0 {
			batch.MediaItems[g.ID] = refs
		}
	}
	return batch
}

func deterministicID(integrationID, rootDir, name string) string {
	h := sha256.Sum256([]byte(integrationID + "|" + rootDir + "|" + name))
	return "scan:" + hex.EncodeToString(h[:])[:16]
}

// findMetadataSources returns an ordered list of metadata plugin sources
// by matching integrations whose plugin provides metadata.game.lookup.
func (o *Orchestrator) findMetadataSources(integrations []*core.Integration) []MetadataSource {
	metaPluginIDs := o.pluginDiscovery.GetPluginIDsProviding(metadataGameLookupMethod)
	if len(metaPluginIDs) == 0 {
		return nil
	}
	metaSet := make(map[string]bool, len(metaPluginIDs))
	for _, id := range metaPluginIDs {
		metaSet[id] = true
	}

	var sources []MetadataSource
	for _, integ := range integrations {
		if !metaSet[integ.PluginID] {
			continue
		}
		config, err := parseConfig(integ.ConfigJSON)
		if err != nil {
			o.logger.Warn("orchestrator: bad metadata config", "integration_id", integ.ID, "error", err)
			continue
		}
		sources = append(sources, MetadataSource{
			PluginID: integ.PluginID,
			Config:   config,
		})
	}
	return sources
}

func pluginProvides(plugin *core.Plugin, method string) bool {
	for _, p := range plugin.Manifest.Provides {
		if p == method {
			return true
		}
	}
	return false
}

func parseConfig(configJSON string) (map[string]any, error) {
	if configJSON == "" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(configJSON), &m); err != nil {
		return nil, err
	}
	return m, nil
}
