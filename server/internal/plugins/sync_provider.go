package plugins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// pluginSyncProvider implements core.SettingsSyncProvider by calling
// storage.backup / storage.restore on a plugin via IPC.
//
// For Restore, the provider needs the DB connected (to read which integration
// provides storage), then must close the DB before the plugin replaces the
// file on disk, and reopen it afterward.
type pluginSyncProvider struct {
	pluginHost PluginHost
	repo       core.IntegrationRepository
	db         core.Database
	config     core.Configuration
	logger     core.Logger
}

func NewPluginSyncProvider(
	pluginHost PluginHost,
	repo core.IntegrationRepository,
	db core.Database,
	config core.Configuration,
	logger core.Logger,
) core.SettingsSyncProvider {
	return &pluginSyncProvider{pluginHost: pluginHost, repo: repo, db: db, config: config, logger: logger}
}

func (p *pluginSyncProvider) Backup(ctx context.Context, _ map[string]any) error {
	pluginID, cfg, err := p.findStorageIntegration(ctx)
	if err != nil {
		return err
	}
	if pluginID == "" {
		return nil
	}

	p.db.Close()

	params := map[string]any{"config": cfg, "db_path": p.config.Get("DB_PATH")}
	var result map[string]any
	callErr := p.pluginHost.Call(ctx, pluginID, "storage.backup", params, &result)

	if err := p.db.Connect(); err != nil {
		return fmt.Errorf("reopen db after backup: %w", err)
	}
	return callErr
}

func (p *pluginSyncProvider) Restore(ctx context.Context, _ map[string]any) error {
	pluginID, cfg, err := p.findStorageIntegration(ctx)
	if err != nil {
		return err
	}
	if pluginID == "" {
		return nil
	}

	// Close DB so the plugin can replace the file on disk.
	p.db.Close()

	params := map[string]any{"config": cfg, "db_path": p.config.Get("DB_PATH")}
	var result map[string]any
	return p.pluginHost.Call(ctx, pluginID, "storage.restore", params, &result)
}

// findStorageIntegration returns the plugin ID and parsed config for the first
// integration whose plugin provides "storage.backup". Returns ("", nil, nil)
// when no storage integration is configured. Requires the DB to be connected.
func (p *pluginSyncProvider) findStorageIntegration(ctx context.Context) (string, map[string]any, error) {
	storagePlugins := p.pluginHost.GetPluginIDsProviding("storage.backup")
	if len(storagePlugins) == 0 {
		return "", nil, nil
	}

	integrations, err := p.repo.List(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("list integrations: %w", err)
	}

	storageSet := make(map[string]bool, len(storagePlugins))
	for _, id := range storagePlugins {
		storageSet[id] = true
	}

	for _, ig := range integrations {
		if !storageSet[ig.PluginID] {
			continue
		}
		var cfg map[string]any
		if err := json.Unmarshal([]byte(ig.ConfigJSON), &cfg); err != nil {
			p.logger.Warn("Bad config in storage integration, skipping", "id", ig.ID, "error", err)
			continue
		}
		return ig.PluginID, cfg, nil
	}
	return "", nil, nil
}
