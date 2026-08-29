package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestMigration39AddsEmptyCatalogWithoutInventingEntitlement(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	database := NewSQLiteDatabaseWithMigrationOptions(
		testLogger{}, testDBConfig{dbPath: dbPath}, core.MigrationOptions{BackupBeforeMigrate: false},
	).(*sqliteDatabase)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.ensureSchemaMigrationsTable(); err != nil {
		t.Fatal(err)
	}
	for _, migration := range database.orderedMigrations() {
		if migration.Version > 38 {
			break
		}
		if err := database.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}

	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC).Unix()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO profiles(id, display_name, role, created_at, updated_at) VALUES (?,?,?,?,?)`, []any{"profile-39", "Player", "player", now, now}},
		{`INSERT INTO integrations(id, profile_id, plugin_id, label, config_json, integration_type, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?)`, []any{"integration-39", "profile-39", "game-source-steam", "Steam", `{}`, "source", now, now}},
		{`INSERT INTO source_games(id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`, []any{"source-39", "profile-39", "integration-39", "game-source-steam", "123", "Existing Game", "windows_pc", "base_game", "unknown", "found", now}},
		{`INSERT INTO canonical_games(id, created_at) VALUES (?,?)`, []any{"game-39", now}},
		{`INSERT INTO canonical_source_games_link(canonical_id, source_game_id) VALUES (?,?)`, []any{"game-39", "source-39"}},
	}
	for _, statement := range statements {
		if _, err := database.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed migration-38 database: %v", err)
		}
	}

	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	assertLatestMigrationVersion(t, database.GetDB())

	for _, table := range []string{
		"catalog_offers", "catalog_package_versions", "catalog_offer_observations", "catalog_offer_events", "catalog_refresh_states",
	} {
		var count int
		if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("migration invented %d rows in %s", count, table)
		}
	}
	var sourceTitle, canonicalID string
	if err := database.GetDB().QueryRow(`SELECT raw_title FROM source_games WHERE id='source-39'`).Scan(&sourceTitle); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().QueryRow(`SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id='source-39'`).Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}
	if sourceTitle != "Existing Game" || canonicalID != "game-39" {
		t.Fatalf("migration changed existing identity: title=%q canonical=%q", sourceTitle, canonicalID)
	}
}
