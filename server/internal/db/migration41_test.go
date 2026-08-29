package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestMigration41AddsEmptyFrontendAPIClientTables(t *testing.T) {
	database := NewSQLiteDatabaseWithMigrationOptions(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "mga.sqlite")}, core.MigrationOptions{BackupBeforeMigrate: false}).(*sqliteDatabase)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.ensureSchemaMigrationsTable(); err != nil {
		t.Fatal(err)
	}
	for _, migration := range database.orderedMigrations() {
		if migration.Version > 40 {
			break
		}
		if err := database.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	assertLatestMigrationVersion(t, database.GetDB())
	for _, table := range []string{"frontend_api_clients", "frontend_api_client_audit"} {
		var count int
		if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("migration invented %d rows in %s", count, table)
		}
	}
}
