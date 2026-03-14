package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// PluginInfo is a read-only descriptor for the API (e.g. GET /api/plugins).
type PluginInfo struct {
	PluginID     string         `json:"plugin_id"`
	Version      string         `json:"plugin_version"`
	Provides     []string       `json:"provides"`
	Capabilities []string        `json:"capabilities"`
	ConfigSchema map[string]any `json:"config,omitempty"`
}

type PluginHost interface {
	Discover(ctx context.Context) error
	Call(ctx context.Context, pluginID string, method string, params any, result any) error
	Close() error
	GetPluginIDs() []string
	// GetPlugin returns the plugin descriptor for the given ID, or (nil, false) if not found.
	GetPlugin(pluginID string) (*core.Plugin, bool)
	// ListPlugins returns all discovered plugins for the API.
	ListPlugins() []PluginInfo
	// GetPluginIDsProviding returns plugin IDs that declare the given method in manifest provides (e.g. metadata.release.lookup).
	GetPluginIDsProviding(method string) []string
}

func (h *pluginHost) GetPluginIDs() []string {
	ids := make([]string, 0, len(h.plugins))
	for id := range h.plugins {
		ids = append(ids, id)
	}
	return ids
}

func (h *pluginHost) GetPlugin(pluginID string) (*core.Plugin, bool) {
	p, ok := h.plugins[pluginID]
	return p, ok
}

func (h *pluginHost) ListPlugins() []PluginInfo {
	out := make([]PluginInfo, 0, len(h.plugins))
	for _, p := range h.plugins {
		out = append(out, PluginInfo{
			PluginID:     p.Manifest.ID,
			Version:     p.Manifest.Version,
			Provides:     p.Manifest.Provides,
			Capabilities: p.Manifest.Capabilities,
			ConfigSchema: p.Manifest.ConfigSchema,
		})
	}
	return out
}

func (h *pluginHost) GetPluginIDsProviding(method string) []string {
	var ids []string
	for id, p := range h.plugins {
		for _, prov := range p.Manifest.Provides {
			if prov == method {
				ids = append(ids, id)
				break
			}
		}
	}
	return ids
}

type pluginHost struct {
	logger         core.Logger
	config         core.Configuration
	processManager ProcessManager
	plugins        map[string]*core.Plugin
	mu             sync.Mutex
	clients        map[string]IpcClient
}

func NewPluginHost(logger core.Logger, config core.Configuration, processManager ProcessManager) PluginHost {
	return &pluginHost{
		logger:         logger,
		config:         config,
		processManager: processManager,
		plugins:        make(map[string]*core.Plugin),
		clients:        make(map[string]IpcClient),
	}
}

func (h *pluginHost) Discover(ctx context.Context) error {
	pluginsDir := h.config.Get("PLUGINS_DIR")
	if pluginsDir == "" {
		pluginsDir = "plugins"
	}
	absPluginsDir, err := filepath.Abs(pluginsDir)
	if err != nil {
		return fmt.Errorf("resolve plugins dir: %w", err)
	}
	pluginsDir = absPluginsDir
	h.logger.Info("Discovering plugins", "dir", pluginsDir)

	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(pluginsDir, entry.Name())
		// Multi-manifest: one directory can have plugin.json and/or *.plugin.json (same binary, multiple plugin IDs).
		manifestFiles, _ := filepath.Glob(filepath.Join(dirPath, "*.plugin.json"))
		singlePath := filepath.Join(dirPath, "plugin.json")
		if _, err := os.Stat(singlePath); err == nil {
			manifestFiles = append(manifestFiles, singlePath)
		}
		for _, manifestPath := range manifestFiles {
			manifestData, err := os.ReadFile(manifestPath)
			if err != nil {
				h.logger.Warn("Failed to read plugin manifest", "path", manifestPath, "error", err)
				continue
			}
			var manifest core.PluginManifest
			if err := json.Unmarshal(manifestData, &manifest); err != nil {
				h.logger.Warn("Failed to unmarshal plugin manifest", "path", manifestPath, "error", err)
				continue
			}
		if manifest.Enabled != nil && !*manifest.Enabled {
			h.logger.Info("Plugin disabled by manifest", "id", manifest.ID, "path", manifestPath)
			continue
		}
		if _, exists := h.plugins[manifest.ID]; exists {
			h.logger.Warn("Duplicate plugin id, skipping", "id", manifest.ID, "path", manifestPath)
			continue
		}
			// Plugin ID convention: lowercase, hyphenated; no reverse-DNS (no dots). E.g. game-source-smb, sync-settings-google-drive.
			if !validPluginID(manifest.ID) {
				h.logger.Warn("Invalid plugin id (use lowercase hyphenated, e.g. game-source-smb), skipping", "id", manifest.ID, "path", manifestPath)
				continue
			}
			h.plugins[manifest.ID] = &core.Plugin{
				Manifest: manifest,
				Path:     dirPath,
				Enabled:  true,
			}
			h.logger.Info("Plugin discovered", "id", manifest.ID, "version", manifest.Version)
		}
	}

	return nil
}

// validPluginID returns true if id matches the convention: lowercase, hyphenated, no dots (no reverse-DNS).
// Pattern: ^[a-z][a-z0-9-]*$
var pluginIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func validPluginID(id string) bool {
	return id != "" && pluginIDRe.MatchString(id)
}

// Call executes a method on a plugin.
func (h *pluginHost) Call(ctx context.Context, pluginID string, method string, params any, result any) error {
	client, err := h.getClient(ctx, pluginID)
	if err != nil {
		return err
	}
	return client.Call(ctx, method, params, result)
}

func (h *pluginHost) getClient(ctx context.Context, pluginID string) (IpcClient, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if client, ok := h.clients[pluginID]; ok {
		return client, nil
	}

	plugin, ok := h.plugins[pluginID]
	if !ok {
		return nil, fmt.Errorf("plugin not found: %s", pluginID)
	}

	execPath := filepath.Join(plugin.Path, plugin.Manifest.Exec)
	process, err := h.processManager.Spawn(context.Background(), execPath, nil, plugin.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn plugin process: %w", err)
	}

	client := NewIpcClient(process, h.logger, pluginID)

	initTimeout := time.Duration(plugin.Manifest.DefaultTimeout) * time.Millisecond
	if initTimeout <= 0 {
		initTimeout = 30 * time.Second
	}
	initCtx, cancel := context.WithTimeout(context.Background(), initTimeout)
	defer cancel()

	var initResult json.RawMessage
	if err := client.Call(initCtx, "plugin.init", map[string]any{}, &initResult); err != nil {
		h.logger.Warn("plugin.init failed, continuing anyway", "plugin_id", pluginID, "error", err)
	} else {
		h.logger.Info("plugin.init completed", "plugin_id", pluginID)
	}

	h.clients[pluginID] = client
	return client, nil
}

func (h *pluginHost) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var lastErr error
	for id, client := range h.clients {
		h.logger.Info("Closing plugin client", "plugin_id", id)
		if err := client.Close(); err != nil {
			h.logger.Error("close plugin client", err, "plugin_id", id)
			lastErr = err
		}
	}
	return lastErr
}
