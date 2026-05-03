package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureSchemaDoesNotCreateProfileForGlobalSettingsOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: dbPath})
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() initial error = %v", err)
	}
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)`, "frontend", "{}", time.Now().Unix()); err != nil {
		t.Fatalf("insert setting: %v", err)
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() second error = %v", err)
	}

	var profiles int
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM profiles`).Scan(&profiles); err != nil {
		t.Fatalf("count profiles: %v", err)
	}
	if profiles != 0 {
		t.Fatalf("profiles = %d, want 0", profiles)
	}
}

func TestEnsureSchemaCreatesDefaultProfileForExistingIntegrationData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: dbPath})
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() initial error = %v", err)
	}
	now := time.Now().Unix()
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO integrations (id, plugin_id, label, config_json, integration_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "integration-1", "plugin", "Plugin", "{}", "source", now, now); err != nil {
		t.Fatalf("insert integration: %v", err)
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() second error = %v", err)
	}

	var profileID string
	if err := dbSvc.GetDB().QueryRow(`SELECT profile_id FROM integrations WHERE id = ?`, "integration-1").Scan(&profileID); err != nil {
		t.Fatalf("get integration profile: %v", err)
	}
	if profileID == "" {
		t.Fatalf("profileID is empty")
	}
	var profiles int
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM profiles WHERE id = ? AND role = ?`, profileID, "admin_player").Scan(&profiles); err != nil {
		t.Fatalf("count profiles: %v", err)
	}
	if profiles != 1 {
		t.Fatalf("profiles = %d, want 1", profiles)
	}
}
