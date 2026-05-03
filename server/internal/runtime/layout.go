package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Mode string

const (
	ModePortable Mode = "portable"
	ModeUser     Mode = "user"
	ModeMachine  Mode = "machine"
)

type Options struct {
	AppDir     string
	DataDir    string
	ConfigPath string
	Mode       Mode
	Service    bool
}

type Layout struct {
	Mode       Mode
	AppDir     string
	DataDir    string
	ConfigPath string
	DBPath     string
	MediaRoot  string
	PluginsDir string
	Frontend   string
	CacheRoot  string
	UpdatesDir string
}

func Resolve(opts Options) (*Layout, error) {
	mode := opts.Mode
	if mode == "" {
		if opts.Service {
			mode = ModeMachine
		} else {
			mode = ModePortable
		}
	}
	if mode != ModePortable && mode != ModeUser && mode != ModeMachine {
		return nil, fmt.Errorf("unsupported runtime mode %q", mode)
	}

	appDir := strings.TrimSpace(opts.AppDir)
	if appDir == "" {
		exePath, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("get executable path: %w", err)
		}
		appDir = filepath.Dir(exePath)
	}
	appDir, err := filepath.Abs(appDir)
	if err != nil {
		return nil, fmt.Errorf("resolve app dir: %w", err)
	}

	dataDir := strings.TrimSpace(opts.DataDir)
	if dataDir == "" {
		dataDir, err = defaultDataDir(mode, appDir)
		if err != nil {
			return nil, err
		}
	}
	dataDir, err = filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data dir: %w", err)
	}

	configPath := strings.TrimSpace(opts.ConfigPath)
	if configPath == "" {
		if mode == ModePortable {
			configPath = filepath.Join(appDir, "config.json")
		} else {
			configPath = filepath.Join(dataDir, "config.json")
		}
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	layout := &Layout{
		Mode:       mode,
		AppDir:     appDir,
		DataDir:    dataDir,
		ConfigPath: configPath,
		PluginsDir: filepath.Join(appDir, "plugins"),
		Frontend:   filepath.Join(appDir, "frontend", "dist"),
	}
	if mode == ModePortable {
		layout.DBPath = filepath.Join(appDir, "data", "db.sqlite")
		layout.MediaRoot = filepath.Join(appDir, "media")
		layout.CacheRoot = filepath.Join(appDir, "source_cache")
		layout.UpdatesDir = filepath.Join(appDir, "updates")
	} else {
		layout.DBPath = filepath.Join(dataDir, "data", "db.sqlite")
		layout.MediaRoot = filepath.Join(dataDir, "media")
		layout.CacheRoot = filepath.Join(dataDir, "source_cache")
		layout.UpdatesDir = filepath.Join(dataDir, "updates")
	}
	return layout, nil
}

func (l *Layout) EnsureConfig() error {
	if l == nil {
		return errors.New("runtime layout is nil")
	}
	if err := os.MkdirAll(filepath.Dir(l.ConfigPath), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	for _, dir := range []string{
		filepath.Dir(l.DBPath),
		l.MediaRoot,
		l.CacheRoot,
		l.UpdatesDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create data directory %s: %w", dir, err)
		}
	}
	if _, err := os.Stat(l.ConfigPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat config file: %w", err)
	}
	cfg := map[string]string{
		"PORT":                "8900",
		"LISTEN_IP":           "127.0.0.1",
		"DB_PATH":             l.DBPath,
		"PLUGINS_DIR":         l.PluginsDir,
		"FRONTEND_DIST":       l.Frontend,
		"MEDIA_ROOT":          l.MediaRoot,
		"SOURCE_CACHE_ROOT":   l.CacheRoot,
		"UPDATES_DIR":         l.UpdatesDir,
		"APP_INSTALL_TYPE":    string(l.Mode),
		"UPDATE_MANIFEST_URL": "https://github.com/GreenFuze/MyGamesAnywhere/releases/latest/download/mga-update.json",
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(l.ConfigPath, data, 0o644); err != nil {
		return fmt.Errorf("write default config %s: %w", l.ConfigPath, err)
	}
	return nil
}
