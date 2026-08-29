package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestMigration40AddsEmptyRuntimeArtifactRegistry(t *testing.T) {
	database := NewSQLiteDatabaseWithMigrationOptions(
		testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "mga.sqlite")},
		core.MigrationOptions{BackupBeforeMigrate: false},
	).(*sqliteDatabase)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.ensureSchemaMigrationsTable(); err != nil {
		t.Fatal(err)
	}
	for _, migration := range database.orderedMigrations() {
		if migration.Version > 39 {
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
	var count int
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM runtime_artifacts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("migration invented %d runtime artifacts", count)
	}
}
