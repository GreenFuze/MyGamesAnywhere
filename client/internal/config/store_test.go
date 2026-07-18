package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testBinding(server, instance string) Binding {
	ids := map[string]string{"one": "11111111-1111-4111-8111-111111111111", "two": "22222222-2222-4222-8222-222222222222"}
	id := ids[instance]
	if id == "" {
		id = "33333333-3333-4333-8333-333333333333"
	}
	return Binding{BindingID: id, ServerURL: server, WebSocketURL: "ws://" + instance + "/connect", EndpointID: "endpoint-" + instance, ClientInstanceID: instance, DisplayName: "PC / Alice"}
}

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()
	store, err := NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := Document{SchemaVersion: SchemaVersion, Bindings: []Binding{testBinding("http://127.0.0.1:8900", "one"), testBinding("http://tv2:8900", "two")}}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.MigrationFrom != 0 || len(got.Document.Bindings) != 2 || got.Document.Bindings[1] != want.Bindings[1] {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNotPaired) {
		t.Fatalf("Load() after Clear error = %v", err)
	}
}

func TestStoreLoadsLegacyWithoutWritingIt(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"schema_version":1,"server_url":"http://localhost:8900","websocket_url":"ws://localhost:8900/connect","endpoint_id":"old-endpoint","client_instance_id":"old-instance","display_name":"PC / Alice"}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store, _ := NewStore(path)
	result, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if result.MigrationFrom != LegacySchemaVersion || len(result.Document.Bindings) != 1 || !result.Document.Bindings[0].LegacyIdentity || result.Document.Bindings[0].BindingID == "" {
		t.Fatalf("legacy result = %+v", result)
	}
	data, _ := os.ReadFile(path)
	if string(data) != legacy {
		t.Fatal("Load modified legacy config before identity verification")
	}
}

func TestDocumentRejectsDuplicateAndExcessBindings(t *testing.T) {
	doc := Document{SchemaVersion: SchemaVersion, Bindings: []Binding{testBinding("http://mga", "one"), testBinding("http://mga", "two")}}
	if err := doc.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate server_url") {
		t.Fatalf("duplicate error = %v", err)
	}
	doc.Bindings = nil
	for i := 0; i <= MaxBindings; i++ {
		doc.Bindings = append(doc.Bindings, testBinding("http://mga"+string(rune('a'+i)), string(rune('a'+i))))
	}
	if err := doc.Validate(); err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("limit error = %v", err)
	}
}

func TestStoreFailsFastOnUnknownSchemaOrFields(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	store, _ := NewStore(path)
	for _, data := range []string{
		`{"schema_version":4,"bindings":[]}`,
		`{"schema_version":3,"bindings":[],"unknown":true}`,
		`{"schema_version":1,"server_url":"x","websocket_url":"x","endpoint_id":"x","client_instance_id":"x","display_name":"x","unknown":true}`,
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(); err == nil {
			t.Fatalf("Load accepted %s", data)
		}
	}
}

func TestStoreLoadsSchemaTwoWithStableBindingMigrationWithoutWriting(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	old := `{"schema_version":2,"bindings":[{"server_url":"http://tv2:8900","websocket_url":"ws://tv2:8900/connect","endpoint_id":"endpoint","client_instance_id":"instance","display_name":"PC / Alice"}]}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	store, _ := NewStore(path)
	result, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if result.MigrationFrom != BindingsSchemaVersion || result.Document.Bindings[0].BindingID == "" {
		t.Fatalf("migration result = %+v", result)
	}
	data, _ := os.ReadFile(path)
	if string(data) != old {
		t.Fatal("Load modified schema-2 config before identity verification")
	}
}
