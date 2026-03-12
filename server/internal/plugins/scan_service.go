package plugins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const sourceLibraryListMethod = "source.library.list"

func providesMethod(plugin *core.Plugin, method string) bool {
	for _, p := range plugin.Manifest.Provides {
		if p == method {
			return true
		}
	}
	return false
}

// PluginGamePersistence persists games from source.library.list so they appear in GET /api/games. Optional; if nil, plugin games are not persisted.
type PluginGamePersistence interface {
	PersistPluginGames(ctx context.Context, integrationID, sourceLabel string, entries []core.GameEntry) error
}

// ScanService scans game sources by calling each source plugin.
type ScanService interface {
	RunScan(ctx context.Context, integrationIDs []string) error
}

type scanService struct {
	pluginHost        PluginHost
	integrationRepo   core.IntegrationRepository
	pluginPersistence PluginGamePersistence
	logger            core.Logger
}

func NewScanService(pluginHost PluginHost, integrationRepo core.IntegrationRepository, pluginPersistence PluginGamePersistence, logger core.Logger) ScanService {
	return &scanService{
		pluginHost:        pluginHost,
		integrationRepo:   integrationRepo,
		pluginPersistence: pluginPersistence,
		logger:            logger,
	}
}

func (s *scanService) RunScan(ctx context.Context, integrationIDs []string) error {
	s.logger.Info("Starting scan", "requested_sources", len(integrationIDs))

	integrations, err := s.integrationRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("list integrations: %w", err)
	}

	allowedIDs := make(map[string]bool)
	for _, id := range integrationIDs {
		allowedIDs[id] = true
	}
	filterByRequest := len(allowedIDs) > 0

	for _, integration := range integrations {
		if filterByRequest && !allowedIDs[integration.ID] {
			continue
		}
		plugin, ok := s.pluginHost.GetPlugin(integration.PluginID)
		if !ok {
			s.logger.Warn("scan skip: plugin not found", "integration_id", integration.ID, "plugin_id", integration.PluginID)
			continue
		}
		if !providesMethod(plugin, sourceLibraryListMethod) {
			continue
		}

		var config map[string]any
		if integration.ConfigJSON != "" {
			if err := json.Unmarshal([]byte(integration.ConfigJSON), &config); err != nil {
				s.logger.Warn("scan skip: invalid config", "integration_id", integration.ID, "error", err)
				continue
			}
		}
		if config == nil {
			config = map[string]any{}
		}

		provider := NewPluginGameSourceAdapter(s.pluginHost, integration.PluginID)
		entries, err := provider.ListGames(ctx, config)
		if err != nil {
			return fmt.Errorf("discovery failed for integration %q (plugin %q): %w", integration.ID, integration.PluginID, err)
		}
		s.logger.Info("plugin scan completed", "integration_id", integration.ID, "label", integration.Label, "games_count", len(entries))

		if s.pluginPersistence != nil {
			if err := s.pluginPersistence.PersistPluginGames(ctx, integration.ID, integration.Label, entries); err != nil {
				return fmt.Errorf("persist plugin games for integration %q: %w", integration.ID, err)
			}
		}
	}

	return nil
}
