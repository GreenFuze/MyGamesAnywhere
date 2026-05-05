package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigService_Get(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"PORT": "8080", "LISTEN_IP": "127.0.0.1", "DB_PATH": "test.db", "TEST_KEY": "test_value"}`), 0644); err != nil {
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
	if Current() != cfg {
		t.Error("expected loaded config to be the current config")
	}
}

func TestConfigService_AllowsUTF8BOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"PORT": "8080", "LISTEN_IP": "127.0.0.1", "DB_PATH": "test.db"}`)...)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := NewConfigService(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("PORT") != "8080" {
		t.Errorf("expected 8080, got %s", cfg.Get("PORT"))
	}
}

func TestConfigService_Validate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"PORT": "8080", "LISTEN_IP": "127.0.0.1", "DB_PATH": "test.db"}`), 0644); err != nil {
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

func TestConfigService_Validate_MissingRequiredPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"LISTEN_IP": "127.0.0.1", "DB_PATH": "test.db"}`), 0644); err != nil {
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

func TestConfigService_Validate_MissingListenIP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"PORT": "8080", "DB_PATH": "test.db"}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := NewConfigService(path)
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing LISTEN_IP")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q did not include config path %q", err, path)
	}
}

func TestConfigService_FileNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	_, err := NewConfigService(path)
	if err == nil {
		t.Error("expected error for missing config file")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q did not include config path %q", err, path)
	}
}

func TestNormalizeListenIP(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "loopback", in: "127.0.0.1", want: "127.0.0.1"},
		{name: "localhost", in: "localhost", want: "127.0.0.1"},
		{name: "any ipv4", in: "0.0.0.0", want: "0.0.0.0"},
		{name: "lan ipv4", in: "192.168.1.25", want: "192.168.1.25"},
		{name: "ipv6 loopback", in: "::1", want: "::1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeListenIP(tc.in)
			if err != nil {
				t.Fatalf("NormalizeListenIP(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeListenIP(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeListenIPRejectsInvalidValue(t *testing.T) {
	if _, err := NormalizeListenIP("not-a-host"); err == nil {
		t.Fatal("expected invalid LISTEN_IP error")
	}
}

func TestListenAddrAndLocalBaseURL(t *testing.T) {
	cfg := &configService{values: map[string]any{
		"PORT":      "8900",
		"LISTEN_IP": "0.0.0.0",
	}}
	addr, err := ListenAddr(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "0.0.0.0:8900" {
		t.Fatalf("ListenAddr = %q, want 0.0.0.0:8900", addr)
	}
	baseURL, err := LocalBaseURL(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if baseURL != "http://127.0.0.1:8900" {
		t.Fatalf("LocalBaseURL = %q, want http://127.0.0.1:8900", baseURL)
	}
}

func TestOAuthCallbackURLUsesGoogleCallbackPathForDrivePlugins(t *testing.T) {
	cfg := &configService{values: map[string]any{
		"PORT":      "8900",
		"LISTEN_IP": "127.0.0.1",
	}}

	got, err := OAuthCallbackURL(cfg, "sync-settings-google-drive")
	if err != nil {
		t.Fatal(err)
	}
	want := "http://127.0.0.1:8900/auth/google/callback/sync-settings-google-drive"
	if got != want {
		t.Fatalf("OAuthCallbackURL = %q, want %q", got, want)
	}

	got, err = OAuthCallbackURL(cfg, "game-source-google-drive")
	if err != nil {
		t.Fatal(err)
	}
	want = "http://127.0.0.1:8900/auth/google/callback/game-source-google-drive"
	if got != want {
		t.Fatalf("OAuthCallbackURL normal plugin = %q, want %q", got, want)
	}

	got, err = OAuthCallbackURL(cfg, "game-source-xbox")
	if err != nil {
		t.Fatal(err)
	}
	want = "http://127.0.0.1:8900/api/auth/callback/game-source-xbox"
	if got != want {
		t.Fatalf("OAuthCallbackURL non-Google plugin = %q, want %q", got, want)
	}
}

func TestLocalBaseURLKeepsLoopbackBindForNonGoogleProviders(t *testing.T) {
	cfg := &configService{values: map[string]any{
		"PORT":      "8900",
		"LISTEN_IP": "127.0.0.1",
	}}
	got, err := LocalBaseURL(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:8900" {
		t.Fatalf("LocalBaseURL = %q, want http://127.0.0.1:8900", got)
	}
}
