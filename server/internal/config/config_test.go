package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigService_Get(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"PORT": "8080", "DB_PATH": "test.db", "TEST_KEY": "test_value"}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := NewConfigService(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("TEST_KEY") != "test_value" {
		t.Errorf("expected test_value, got %s", cfg.Get("TEST_KEY"))
	}
	if cfg.Get("PORT") != "8080" {
		t.Errorf("expected 8080, got %s", cfg.Get("PORT"))
	}
}

func TestConfigService_Validate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"PORT": "8080", "DB_PATH": "test.db"}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := NewConfigService(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfigService_Validate_MissingRequired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"DB_PATH": "test.db"}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := NewConfigService(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing PORT")
	}
}

func TestConfigService_FileNotFound(t *testing.T) {
	_, err := NewConfigService(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Error("expected error for missing config file")
	}
}
