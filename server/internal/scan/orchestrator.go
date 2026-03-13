package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan/scanner"
)

const sourceFilesystemListMethod = "source.filesystem.list"

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
// source plugin, runs the scanner pipeline, and converts the results
// into core.Game entities. No database interaction — results are
// returned in-memory for the caller to inspect or persist.
type Orchestrator struct {
	pluginCaller     PluginCaller
	pluginDiscovery  PluginDiscovery
	integrationRepo  core.IntegrationRepository
	scanner          *scanner.Scanner
	metadataResolver *MetadataResolver
	logger           core.Logger
}

func NewOrchestrator(
	caller PluginCaller,
	discovery PluginDiscovery,
	integrationRepo core.IntegrationRepository,
	logger core.Logger,
) *Orchestrator {
	return &Orchestrator{
		pluginCaller:     caller,
		pluginDiscovery:  discovery,
		integrationRepo:  integrationRepo,
		scanner:          scanner.New(logger),
		metadataResolver: NewMetadataResolver(caller, logger),
		logger:           logger,
	}
}

// RunScan scans all (or selected) integrations and returns the discovered games.
func (o *Orchestrator) RunScan(ctx context.Context, integrationIDs []string) ([]*core.Game, error) {
	o.logger.Info("orchestrator: starting scan", "requested", len(integrationIDs))

	integrations, err := o.integrationRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}

	filter := make(map[string]bool, len(integrationIDs))
	for _, id := range integrationIDs {
		filter[id] = true
	}
	filterActive := len(filter) > 0

	var allGames []*core.Game

	for _, integ := range integrations {
		if filterActive && !filter[integ.ID] {
			continue
		}
		plugin, ok := o.pluginDiscovery.GetPlugin(integ.PluginID)
		if !ok {
			o.logger.Warn("orchestrator: plugin not found", "integration_id", integ.ID, "plugin_id", integ.PluginID)
			continue
		}
		if !pluginProvides(plugin, sourceFilesystemListMethod) {
			continue
		}

		config, err := parseConfig(integ.ConfigJSON)
		if err != nil {
			o.logger.Warn("orchestrator: bad config", "integration_id", integ.ID, "error", err)
			continue
		}

		files, err := o.fetchFiles(ctx, integ.PluginID, config)
		if err != nil {
			return nil, fmt.Errorf("fetch files from integration %q: %w", integ.ID, err)
		}
		o.logger.Info("orchestrator: fetched files", "integration_id", integ.ID, "count", len(files))

		groups, err := o.scanner.ScanFiles(ctx, files)
		if err != nil {
			return nil, fmt.Errorf("scan files for integration %q: %w", integ.ID, err)
		}

		games := buildGames(integ.ID, groups)
		o.logger.Info("orchestrator: built games", "integration_id", integ.ID, "games", len(games))

		allGames = append(allGames, games...)
	}

	// Metadata enrichment: find all metadata plugin integrations and enrich.
	metaSources := o.findMetadataSources(integrations)
	if len(metaSources) > 0 {
		o.metadataResolver.Enrich(ctx, allGames, metaSources)
	}

	o.logger.Info("orchestrator: scan complete", "total_games", len(allGames))
	return allGames, nil
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

// buildGames converts scanner GameGroups into core.Game entities.
// Each group becomes one Game; files carry over with their roles.
func buildGames(integrationID string, groups []scanner.GameGroup) []*core.Game {
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
