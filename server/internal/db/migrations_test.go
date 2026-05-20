package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestMigrationsFreshDBReachLatestAndAreIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: dbPath})
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()

	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	assertLatestMigrationVersion(t, dbSvc.GetDB())
	firstSchema := schemaSnapshot(t, dbSvc.GetDB())

	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() second error = %v", err)
	}
	secondSchema := schemaSnapshot(t, dbSvc.GetDB())
	if firstSchema != secondSchema {
		t.Fatalf("schema changed after idempotent migration\nfirst:\n%s\nsecond:\n%s", firstSchema, secondSchema)
	}
}

func TestMigrationsRejectChecksumMismatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: dbPath})
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if _, err := dbSvc.GetDB().Exec(`UPDATE schema_migrations SET checksum='bad' WHERE version=1`); err != nil {
		t.Fatalf("corrupt checksum: %v", err)
	}
	if err := dbSvc.EnsureSchema(); err == nil {
		t.Fatal("EnsureSchema() succeeded with checksum mismatch")
	}
}

func TestMigrationsAcceptKnownLegacyInitialChecksum(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: dbPath})
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if _, err := dbSvc.GetDB().Exec(`UPDATE schema_migrations SET checksum='3a75c8a20ded02e0892d85f70e29da25b2d65e3bcd2201f4d92cf5b430fe8a7d' WHERE version=1`); err != nil {
		t.Fatalf("set legacy checksum: %v", err)
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() with known legacy checksum error = %v", err)
	}
}

func TestMigrationsRejectNewerDatabaseVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: dbPath})
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO schema_migrations (version, name, checksum, applied_at, duration_ms, success) VALUES (999, 'future', 'x', 1, 0, 1)`); err != nil {
		t.Fatalf("insert future migration: %v", err)
	}
	if err := dbSvc.EnsureSchema(); err == nil {
		t.Fatal("EnsureSchema() succeeded with newer DB version")
	}
}

func TestMigrationBackupCopiesSQLiteTriplet(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "mga.sqlite")
	backupDir := filepath.Join(root, "backups")
	for _, path := range sqliteFileTriplet(dbPath) {
		if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	dbSvc := &sqliteDatabase{config: testDBConfig{dbPath: dbPath}, logger: testLogger{}}
	backupPath, err := dbSvc.backupSQLiteFiles(backupDir)
	if err != nil {
		t.Fatalf("backupSQLiteFiles() error = %v", err)
	}
	for _, path := range sqliteFileTriplet(dbPath) {
		if _, err := os.Stat(filepath.Join(backupPath, filepath.Base(path))); err != nil {
			t.Fatalf("backup missing %s: %v", path, err)
		}
	}
}

func TestMigrationsCanSkipStartupBackupForTests(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabaseWithMigrationOptions(testLogger{}, testDBConfig{dbPath: dbPath}, core.MigrationOptions{BackupBeforeMigrate: false})
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	assertLatestMigrationVersion(t, dbSvc.GetDB())
}

func TestLegacyFixtureMigratesToFreshSchemaAndPreservesProfileOwnedData(t *testing.T) {
	freshPath := filepath.Join(t.TempDir(), "fresh.sqlite")
	freshSvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: freshPath})
	if err := freshSvc.Connect(); err != nil {
		t.Fatalf("fresh Connect() error = %v", err)
	}
	defer freshSvc.Close()
	if err := freshSvc.EnsureSchema(); err != nil {
		t.Fatalf("fresh EnsureSchema() error = %v", err)
	}
	freshSchema := logicalSchemaSnapshot(t, freshSvc.GetDB())

	ctx := context.Background()
	legacyPath := filepath.Join(t.TempDir(), "legacy.sqlite")
	legacySvc := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: legacyPath})
	if err := legacySvc.Connect(); err != nil {
		t.Fatalf("legacy Connect() error = %v", err)
	}
	defer legacySvc.Close()

	now := time.Now().Unix()
	if _, err := legacySvc.GetDB().ExecContext(ctx, `CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, updated_at INTEGER)`); err != nil {
		t.Fatalf("create legacy settings: %v", err)
	}
	if _, err := legacySvc.GetDB().ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES ('frontend', '{}', ?)`, now); err != nil {
		t.Fatalf("insert legacy setting: %v", err)
	}
	if _, err := legacySvc.GetDB().ExecContext(ctx, `CREATE TABLE integrations (
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
	if _, err := legacySvc.GetDB().ExecContext(ctx, `INSERT INTO integrations (id, plugin_id, label, config_json, integration_type, created_at, updated_at)
		VALUES ('legacy-source', 'game-source-steam', 'Steam', '{}', 'source', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert legacy integration: %v", err)
	}
	createLegacyCanonicalTables(t, ctx, legacySvc.GetDB())
	if _, err := legacySvc.GetDB().ExecContext(ctx, `INSERT INTO source_games
		(id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, created_at)
		VALUES ('scan:legacy', 'legacy-source', 'game-source-steam', '123', 'Legacy Game', 'windows_pc', 'base_game', 'unknown', 'found', ?)`, now); err != nil {
		t.Fatalf("insert legacy source game: %v", err)
	}

	if err := legacySvc.EnsureSchema(); err != nil {
		t.Fatalf("legacy EnsureSchema() error = %v", err)
	}
	if legacySchema := logicalSchemaSnapshot(t, legacySvc.GetDB()); legacySchema != freshSchema {
		t.Fatalf("legacy migrated schema differs from fresh schema\nlegacy:\n%s\nfresh:\n%s", legacySchema, freshSchema)
	}
	var profileCount int
	if err := legacySvc.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE role='admin_player'`).Scan(&profileCount); err != nil {
		t.Fatalf("count admin profiles: %v", err)
	}
	if profileCount != 1 {
		t.Fatalf("admin profile count = %d, want 1", profileCount)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM integrations WHERE profile_id IS NULL OR profile_id=''`,
		`SELECT COUNT(*) FROM source_games WHERE profile_id IS NULL OR profile_id=''`,
	} {
		var missing int
		if err := legacySvc.GetDB().QueryRowContext(ctx, query).Scan(&missing); err != nil {
			t.Fatalf("query profile assignment: %v", err)
		}
		if missing != 0 {
			t.Fatalf("missing profile assignments for query %q: %d", query, missing)
		}
	}
}

func assertLatestMigrationVersion(t *testing.T, db *sql.DB) {
	t.Helper()
	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE success=1`).Scan(&version); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != latestMigrationVersion {
		t.Fatalf("migration version = %d, want %d", version, latestMigrationVersion)
	}
}

func schemaSnapshot(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT type, name, tbl_name, sql FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		t.Fatalf("schema query: %v", err)
	}
	defer rows.Close()
	var out string
	for rows.Next() {
		var typ, name, table string
		var sqlText sql.NullString
		if err := rows.Scan(&typ, &name, &table, &sqlText); err != nil {
			t.Fatalf("schema scan: %v", err)
		}
		out += typ + "|" + name + "|" + table + "|" + sqlText.String + "\n"
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("schema rows: %v", err)
	}
	return out
}

func logicalSchemaSnapshot(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("table rows: %v", err)
	}
	rows.Close()

	var out string
	for _, table := range tables {
		out += "table|" + table + "\n"
		columnRows, err := db.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatalf("table_info %s: %v", table, err)
		}
		var columns []string
		for columnRows.Next() {
			var cid int
			var name, typ string
			var notNull, pk int
			var defaultValue sql.NullString
			if err := columnRows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
				columnRows.Close()
				t.Fatalf("scan column %s: %v", table, err)
			}
			columns = append(columns, "column|"+table+"|"+name+"|"+typ+"|"+defaultValue.String+"|"+fmtInt(notNull)+"|"+fmtInt(pk))
		}
		if err := columnRows.Err(); err != nil {
			columnRows.Close()
			t.Fatalf("column rows %s: %v", table, err)
		}
		columnRows.Close()
		sort.Strings(columns)
		for _, column := range columns {
			out += column + "\n"
		}
	}
	indexRows, err := db.Query(`SELECT type, name, tbl_name FROM sqlite_schema WHERE type='index' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	defer indexRows.Close()
	for indexRows.Next() {
		var typ, name, table string
		if err := indexRows.Scan(&typ, &name, &table); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		out += typ + "|" + name + "|" + table + "\n"
	}
	if err := indexRows.Err(); err != nil {
		t.Fatalf("index rows: %v", err)
	}
	return out
}

func fmtInt(value int) string {
	if value == 0 {
		return "0"
	}
	return "1"
}
