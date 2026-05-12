package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestEnsureSchemaDoesNotCreateProfileForGlobalSettingsOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: dbPath})
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if _, err := dbSvc.GetDB().Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, updated_at INTEGER)`); err != nil {
		t.Fatalf("create legacy settings: %v", err)
	}
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)`, "frontend", "{}", time.Now().Unix()); err != nil {
		t.Fatalf("insert setting: %v", err)
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
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
	if _, err := dbSvc.GetDB().Exec(`CREATE TABLE integrations (
		id TEXT PRIMARY KEY,
		plugin_id TEXT NOT NULL,
		label TEXT NOT NULL,
		config_json TEXT,
		integration_type TEXT NOT NULL,
		created_at INTEGER,
		updated_at INTEGER
	)`); err != nil {
		t.Fatalf("create legacy integrations: %v", err)
	}
	now := time.Now().Unix()
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO integrations (id, plugin_id, label, config_json, integration_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "integration-1", "plugin", "Plugin", "{}", "source", now, now); err != nil {
		t.Fatalf("insert integration: %v", err)
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
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

func TestEnsureSchemaNormalizesLegacySettingsSyncIntegrationType(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: dbPath})
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if _, err := dbSvc.GetDB().Exec(`CREATE TABLE integrations (
		id TEXT PRIMARY KEY,
		plugin_id TEXT NOT NULL,
		label TEXT NOT NULL,
		config_json TEXT,
		integration_type TEXT NOT NULL,
		created_at INTEGER,
		updated_at INTEGER
	)`); err != nil {
		t.Fatalf("create legacy integrations: %v", err)
	}
	now := time.Now().Unix()
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO integrations (id, plugin_id, label, config_json, integration_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "sync-1", "sync-settings-google-drive", "Google Drive Sync", "{}", "storage", now, now); err != nil {
		t.Fatalf("insert integration: %v", err)
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}

	var integrationType string
	if err := dbSvc.GetDB().QueryRow(`SELECT integration_type FROM integrations WHERE id = ?`, "sync-1").Scan(&integrationType); err != nil {
		t.Fatalf("get integration type: %v", err)
	}
	if integrationType != "sync" {
		t.Fatalf("integration_type = %q, want sync", integrationType)
	}
}

func TestMigration8BackfillsFavoritesPerVisibleProfile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: dbPath})
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()

	sqliteDB := dbSvc.(*sqliteDatabase)
	if err := sqliteDB.ensureSchemaMigrationsTable(); err != nil {
		t.Fatalf("ensure schema migrations table: %v", err)
	}
	for _, migration := range sqliteDB.orderedMigrations() {
		if migration.Version >= 8 {
			break
		}
		if err := sqliteDB.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}

	now := time.Now().Unix()
	if _, err := sqliteDB.GetDB().Exec(`INSERT INTO profiles (id, display_name, avatar_key, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?)`,
		"profile-1", "Profile One", "player-1", string(core.ProfileRoleAdminPlayer), now, now,
		"profile-2", "Profile Two", "player-2", string(core.ProfileRolePlayer), now, now); err != nil {
		t.Fatalf("insert profiles: %v", err)
	}
	if _, err := sqliteDB.GetDB().Exec(`INSERT INTO canonical_games (id, created_at) VALUES (?, ?)`, "canonical-1", now); err != nil {
		t.Fatalf("insert canonical game: %v", err)
	}
	if _, err := sqliteDB.GetDB().Exec(`INSERT INTO source_games
		(id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"source-1", "profile-1", "integration-1", "game-source-steam", "external-1", "Shared", "windows_pc", "base_game", "self_contained", "found", now,
		"source-2", "profile-2", "integration-2", "game-source-steam", "external-2", "Shared", "windows_pc", "base_game", "self_contained", "found", now); err != nil {
		t.Fatalf("insert source games: %v", err)
	}
	if _, err := sqliteDB.GetDB().Exec(`INSERT INTO canonical_source_games_link (canonical_id, source_game_id)
		VALUES (?, ?), (?, ?)`, "canonical-1", "source-1", "canonical-1", "source-2"); err != nil {
		t.Fatalf("insert canonical links: %v", err)
	}
	if _, err := sqliteDB.GetDB().Exec(`INSERT INTO canonical_game_favorites (canonical_id, updated_at) VALUES (?, ?)`, "canonical-1", now); err != nil {
		t.Fatalf("insert legacy favorite: %v", err)
	}

	if err := sqliteDB.Migrate(core.MigrationOptions{}); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := sqliteDB.Migrate(core.MigrationOptions{}); err != nil {
		t.Fatalf("Migrate() idempotency error = %v", err)
	}

	for _, profileID := range []string{"profile-1", "profile-2"} {
		var count int
		if err := sqliteDB.GetDB().QueryRow(`SELECT COUNT(*) FROM canonical_game_favorites WHERE profile_id = ? AND canonical_id = ?`, profileID, "canonical-1").Scan(&count); err != nil {
			t.Fatalf("count favorites for %s: %v", profileID, err)
		}
		if count != 1 {
			t.Fatalf("favorite count for %s = %d, want 1", profileID, count)
		}
	}
}

func TestEnsureSchemaCreatesProfileScopedFavoritesTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: dbPath})
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}

	rows, err := dbSvc.GetDB().Query(`PRAGMA table_info(canonical_game_favorites)`)
	if err != nil {
		t.Fatalf("table info: %v", err)
	}
	defer rows.Close()
	primaryKeys := map[string]int{}
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table info: %v", err)
		}
		primaryKeys[name] = pk
	}
	if primaryKeys["profile_id"] != 1 || primaryKeys["canonical_id"] != 2 {
		t.Fatalf("primary keys = %+v, want profile_id/canonical_id composite key", primaryKeys)
	}
}
