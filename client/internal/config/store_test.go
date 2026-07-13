package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	want := Config{
		SchemaVersion: SchemaVersion, ServerURL: "http://127.0.0.1:8900", WebSocketURL: "ws://127.0.0.1:8900/api/devices/connect",
		EndpointID: "endpoint-1", ClientInstanceID: "instance-1", DisplayName: "PC / Alice",
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNotPaired) {
		t.Fatalf("Load() after Clear error = %v, want ErrNotPaired", err)
	}
}

func TestStoreFailsFastOnUnknownSchemaOrFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	data := `{"schema_version":2,"server_url":"x","websocket_url":"x","endpoint_id":"x","client_instance_id":"x","display_name":"x"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("Load() error = %v, want unsupported schema", err)
	}
	data = `{"schema_version":1,"server_url":"x","websocket_url":"x","endpoint_id":"x","client_instance_id":"x","display_name":"x","unknown":true}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Load() error = %v, want unknown field", err)
	}
}

func TestStoreReturnsNotPaired(t *testing.T) {
	t.Parallel()

	store, err := NewStore(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNotPaired) {
		t.Fatalf("Load() error = %v, want ErrNotPaired", err)
	}
}
