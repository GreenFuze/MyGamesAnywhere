package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestMigration37AddsEmptyProfileScopedStorefrontProductsAndPreservesProfiles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabaseWithMigrationOptions(testLogger{}, testDBConfig{dbPath: dbPath}, core.MigrationOptions{BackupBeforeMigrate: false}).(*sqliteDatabase)
	if err := dbSvc.Connect(); err != nil {
		t.Fatal(err)
	}
	defer dbSvc.Close()
	if err := dbSvc.ensureSchemaMigrationsTable(); err != nil {
		t.Fatal(err)
	}
	for _, migration := range dbSvc.orderedMigrations() {
		if migration.Version > 36 {
			break
		}
		if err := dbSvc.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}
	now := time.Now().Unix()
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO profiles(id, display_name, role, created_at, updated_at) VALUES ('profile-37','Player','player',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	assertLatestMigrationVersion(t, dbSvc.GetDB())
	var profiles, products int
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM profiles WHERE id='profile-37'`).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM device_storefront_products`).Scan(&products); err != nil {
		t.Fatal(err)
	}
	if profiles != 1 || products != 0 {
		t.Fatalf("migration 37 state: profiles=%d storefront_products=%d", profiles, products)
	}
}
