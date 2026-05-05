package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePortableLayout(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "app")
	layout, err := Resolve(Options{Mode: ModePortable, AppDir: appDir})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if layout.ConfigPath != filepath.Join(appDir, "config.json") {
		t.Fatalf("ConfigPath = %q", layout.ConfigPath)
	}
	if layout.DBPath != filepath.Join(appDir, "data", "db.sqlite") {
		t.Fatalf("DBPath = %q", layout.DBPath)
	}
}

func TestResolveUserLayoutUsesSeparateDataDir(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "app")
	dataDir := filepath.Join(t.TempDir(), "data")
	layout, err := Resolve(Options{Mode: ModeUser, AppDir: appDir, DataDir: dataDir})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if layout.ConfigPath != filepath.Join(dataDir, "config.json") {
		t.Fatalf("ConfigPath = %q", layout.ConfigPath)
	}
	if layout.PluginsDir != filepath.Join(appDir, "plugins") {
		t.Fatalf("PluginsDir = %q", layout.PluginsDir)
	}
	if layout.MediaRoot != filepath.Join(dataDir, "media") {
		t.Fatalf("MediaRoot = %q", layout.MediaRoot)
	}
	if layout.LogFile != filepath.Join(dataDir, "logs", "mga_server.log") {
		t.Fatalf("LogFile = %q", layout.LogFile)
	}
}

func TestEnsureConfigWritesAbsoluteInstalledPaths(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "app")
	dataDir := filepath.Join(t.TempDir(), "data")
	layout, err := Resolve(Options{Mode: ModeMachine, AppDir: appDir, DataDir: dataDir})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if err := layout.EnsureConfig(); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	data, err := os.ReadFile(layout.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]string
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	wants := map[string]string{
		"DB_PATH":         layout.DBPath,
		"MEDIA_ROOT":      layout.MediaRoot,
		"PLUGINS_DIR":     layout.PluginsDir,
		"FRONTEND_DIST":   layout.Frontend,
		"LOG_FILE":        layout.LogFile,
		"LOG_MAX_SIZE_MB": "50",
		"LOG_MAX_BACKUPS": "5",
	}
	for key, want := range wants {
		if key != "LOG_MAX_SIZE_MB" && key != "LOG_MAX_BACKUPS" && !filepath.IsAbs(want) {
			t.Fatalf("test path is not absolute: %q", want)
		}
		if cfg[key] != want {
			t.Fatalf("%s = %q, want %q", key, cfg[key], want)
		}
	}
}

func TestServiceDefaultsToMachineMode(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "app")
	dataDir := filepath.Join(t.TempDir(), "data")
	layout, err := Resolve(Options{AppDir: appDir, DataDir: dataDir, Service: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if layout.Mode != ModeMachine {
		t.Fatalf("Mode = %q", layout.Mode)
	}
}
