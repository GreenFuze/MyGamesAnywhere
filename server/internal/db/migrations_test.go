package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

func TestMigration29AddsCredentialTicketsWithoutChangingCredentials(t *testing.T) {
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
		if migration.Version > 28 {
			break
		}
		if err := dbSvc.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}
	now := time.Now().Unix()
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO profiles(id, display_name, role, created_at, updated_at)
		VALUES ('profile-29','Player','player',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO profile_credentials(profile_id, kind, hash, must_change, updated_at)
		VALUES ('profile-29','pin','existing-hash',0,?)`, now); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := dbSvc.GetDB().QueryRow(`SELECT hash FROM profile_credentials WHERE profile_id='profile-29'`).Scan(&hash); err != nil || hash != "existing-hash" {
		t.Fatalf("existing credential changed: hash=%q err=%v", hash, err)
	}
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO profile_credential_tickets
		(id, profile_id, token_hash, created_by_profile_id, created_at, expires_at)
		VALUES ('ticket-29','profile-29','token-hash','profile-29',?,?)`, now, now+600); err != nil {
		t.Fatalf("insert credential ticket: %v", err)
	}
}

func TestMigration30AddsEmptySaveCompatibilityRegistryWithoutChangingProfiles(t *testing.T) {
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
		if migration.Version > 29 {
			break
		}
		if err := dbSvc.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}
	now := time.Now().Unix()
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO profiles(id, display_name, role, created_at, updated_at)
		VALUES ('profile-30','Player','player',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	assertLatestMigrationVersion(t, dbSvc.GetDB())

	var profiles, converters, rules int
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM profiles WHERE id='profile-30'`).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM save_converter_registry`).Scan(&converters); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM save_compatibility_rules`).Scan(&rules); err != nil {
		t.Fatal(err)
	}
	if profiles != 1 || converters != 0 || rules != 0 {
		t.Fatalf("migration 30 state: profiles=%d converters=%d rules=%d", profiles, converters, rules)
	}

	rows, err := dbSvc.GetDB().Query(`PRAGMA table_info(save_compatibility_rules)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(name), "title") {
			t.Fatalf("title-derived compatibility column was added: %s", name)
		}
	}
}

func TestMigration27PreservesExistingInstallationsAsManaged(t *testing.T) {
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
		if migration.Version > 26 {
			break
		}
		if err := dbSvc.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}
	now := time.Now().Unix()
	statements := []string{
		`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES ('profile-27','Player','admin_player',` + fmt.Sprint(now) + `,` + fmt.Sprint(now) + `)`,
		`INSERT INTO canonical_games (id, created_at) VALUES ('game-27',` + fmt.Sprint(now) + `)`,
		`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at) VALUES ('source-27','profile-27','i','p','e','Game','windows_pc','base_game','packed','found','matched',` + fmt.Sprint(now) + `)`,
		`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, execution_mode, client_version, protocol_version, capabilities_json, status, created_at, updated_at) VALUES ('endpoint-27','instance-27','key','PC','pc','user','windows','amd64','standard','dev',1,'[]','offline',` + fmt.Sprint(now) + `,` + fmt.Sprint(now) + `)`,
		`INSERT INTO device_game_installations (endpoint_id, game_id, source_game_id, profile_id, install_root, install_path, archive_sha256, archive_bytes, installed_at, updated_at) VALUES ('endpoint-27','game-27','source-27','profile-27','C:\Games','C:\Games\Game','hash',1,` + fmt.Sprint(now) + `,` + fmt.Sprint(now) + `)`,
	}
	for _, statement := range statements {
		if _, err := dbSvc.GetDB().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	var localID, authority string
	if err := dbSvc.GetDB().QueryRow(`SELECT COALESCE(local_installation_id,''), authority_mode FROM device_game_installations WHERE endpoint_id='endpoint-27'`).Scan(&localID, &authority); err != nil {
		t.Fatal(err)
	}
	if localID != "" || authority != "managed" {
		t.Fatalf("migration 27 defaults = %q %q", localID, authority)
	}
}

func TestMigrations25To28AddEmptyExtendedInventory(t *testing.T) {
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
		if migration.Version > 24 {
			break
		}
		if err := dbSvc.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}
	now := time.Now().Unix()
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, execution_mode, client_version, protocol_version, capabilities_json, status, created_at, updated_at) VALUES ('endpoint-25','instance-25','key','PC','pc','user','windows','amd64','standard','dev',1,'[]','offline',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO device_inventories (endpoint_id, schema_version, captured_at, storage_json, runtimes_json, package_managers_json, updated_at) VALUES ('endpoint-25',2,?,'[]','[]','[]',?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	var adapters string
	if err := dbSvc.GetDB().QueryRow(`SELECT save_adapters_json FROM device_inventories WHERE endpoint_id='endpoint-25'`).Scan(&adapters); err != nil || adapters != "[]" {
		t.Fatalf("migrated save adapters = %q, error = %v", adapters, err)
	}
	var installations string
	if err := dbSvc.GetDB().QueryRow(`SELECT managed_installations_json FROM device_inventories WHERE endpoint_id='endpoint-25'`).Scan(&installations); err != nil || installations != "[]" {
		t.Fatalf("migrated managed installations = %q, error = %v", installations, err)
	}
	var saveDomains string
	if err := dbSvc.GetDB().QueryRow(`SELECT save_domains_json FROM device_inventories WHERE endpoint_id='endpoint-25'`).Scan(&saveDomains); err != nil || saveDomains != "[]" {
		t.Fatalf("migrated save domains = %q, error = %v", saveDomains, err)
	}
	var tableName string
	if err := dbSvc.GetDB().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='device_save_domain_links'`).Scan(&tableName); err != nil || tableName != "device_save_domain_links" {
		t.Fatalf("save domain link table = %q, error = %v", tableName, err)
	}
	statements := []string{
		`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES ('profile-28','Player','admin_player',?,?)`,
		`INSERT INTO canonical_games (id, created_at) VALUES ('game-28',?)`,
		`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at) VALUES ('source-28','profile-28','i','p','e','Game','scummvm','base_game','self_contained','found','matched',?)`,
	}
	for _, statement := range statements {
		arguments := []any{now}
		if strings.Contains(statement, "profiles") {
			arguments = []any{now, now}
		}
		if _, err := dbSvc.GetDB().Exec(statement, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO device_save_domain_links (endpoint_id, game_id, source_game_id, route_kind, emulator_id, local_save_domain_id, adapter_id, authority_state, created_by_profile_id, created_at, updated_at) VALUES ('endpoint-25','game-28','source-28','emulator','scummvm','local-28','scummvm','owned_here','profile-28',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	var syncState string
	if err := dbSvc.GetDB().QueryRow(`SELECT sync_state FROM device_save_domain_links WHERE local_save_domain_id='local-28'`).Scan(&syncState); err != nil || syncState != "never_backed_up" {
		t.Fatalf("save domain sync default = %q, error = %v", syncState, err)
	}
}

func TestMigration16UpgradesVersion15RowsWithSafeLaunchDefaults(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabaseWithMigrationOptions(
		testLogger{},
		testDBConfig{dbPath: dbPath},
		core.MigrationOptions{BackupBeforeMigrate: false},
	).(*sqliteDatabase)
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if err := dbSvc.ensureSchemaMigrationsTable(); err != nil {
		t.Fatalf("ensure migrations table: %v", err)
	}
	for _, migration := range dbSvc.orderedMigrations() {
		if migration.Version > 15 {
			break
		}
		if err := dbSvc.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}
	var version int
	if err := dbSvc.GetDB().QueryRow(`SELECT MAX(version) FROM schema_migrations WHERE success=1`).Scan(&version); err != nil || version != 15 {
		t.Fatalf("pre-upgrade version = %d, error = %v", version, err)
	}

	now := time.Now().Unix()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, []any{"profile-1", "Player", "admin_player", now, now}},
		{`INSERT INTO canonical_games (id, created_at) VALUES (?, ?)`, []any{"game-1", now}},
		{`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"source-1", "profile-1", "integration-1", "game-source-google-drive", "source-1", "Game", "windows_pc", "base_game", "packed", "found", "matched", now}},
		{`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, client_version, protocol_version, capabilities_json, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"endpoint-1", "instance-1", "key", "PC", "pc", "user", "windows", "amd64", "dev", 1, `[]`, "offline", now, now}},
		{`INSERT INTO device_commands (id, endpoint_id, profile_id, name, schema_version, idempotency_key, status, payload_json, created_at, updated_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"command-1", "endpoint-1", "profile-1", "game.install_archive", 1, "idem-1", "succeeded", `{}`, now, now, now + 60}},
		{`INSERT INTO device_game_installations (endpoint_id, game_id, source_game_id, profile_id, install_root, install_path, archive_sha256, archive_bytes, installed_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"endpoint-1", "game-1", "source-1", "profile-1", `C:\Games`, `C:\Games\Game`, strings.Repeat("a", 64), 42, now, now}},
	} {
		if _, err := dbSvc.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed version 15 row: %v", err)
		}
	}

	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("apply migration 16: %v", err)
	}
	assertLatestMigrationVersion(t, dbSvc.GetDB())
	var progressStage sql.NullString
	var progressStagePercent sql.NullInt64
	if err := dbSvc.GetDB().QueryRow(`SELECT progress_stage, progress_stage_percent FROM device_commands WHERE id='command-1'`).Scan(&progressStage, &progressStagePercent); err != nil {
		t.Fatalf("read migrated command columns: %v", err)
	}
	if progressStage.Valid || progressStagePercent.Valid {
		t.Fatalf("migrated command stage = %#v / %#v, want NULL defaults", progressStage, progressStagePercent)
	}
	var launchTarget sql.NullString
	var launchCandidates string
	if err := dbSvc.GetDB().QueryRow(`SELECT launch_target, launch_candidates_json FROM device_game_installations WHERE endpoint_id='endpoint-1'`).Scan(&launchTarget, &launchCandidates); err != nil {
		t.Fatalf("read migrated installation columns: %v", err)
	}
	if launchTarget.Valid || launchCandidates != "[]" {
		t.Fatalf("migrated launch fields = %#v / %q", launchTarget, launchCandidates)
	}
}

func TestMigrations18And19PreserveLegacyAttentionRowAndAddStandardExecutionMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabaseWithMigrationOptions(
		testLogger{}, testDBConfig{dbPath: dbPath}, core.MigrationOptions{BackupBeforeMigrate: false},
	).(*sqliteDatabase)
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if err := dbSvc.ensureSchemaMigrationsTable(); err != nil {
		t.Fatalf("ensure migrations table: %v", err)
	}
	for _, migration := range dbSvc.orderedMigrations() {
		if migration.Version > 17 {
			break
		}
		if err := dbSvc.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}
	var migration17 migration
	for _, candidate := range dbSvc.orderedMigrations() {
		if candidate.Version == 17 {
			migration17 = candidate
			break
		}
	}
	if got := migrationChecksum(migration17); got != "c15af1fda922e7eedbede9fcb802ce19fe7994178d1f04252de8b5cafe249dd6" {
		t.Fatalf("migration 17 checksum changed: %s", got)
	}

	now := time.Now().Unix()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, []any{"profile-1", "Player", "admin_player", now, now}},
		{`INSERT INTO canonical_games (id, created_at) VALUES (?, ?)`, []any{"game-1", now}},
		{`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"source-1", "profile-1", "integration-1", "game-source-google-drive", "source-1", "Duke", "windows_pc", "base_game", "packed", "found", "matched", now}},
		{`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, client_version, protocol_version, capabilities_json, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"endpoint-1", "instance-1", "key", "PC", "pc", "user", "windows", "amd64", "dev", 1, `[]`, "offline", now, now}},
		{`INSERT INTO device_game_installations
			(endpoint_id, game_id, source_game_id, profile_id, install_root, install_path, archive_sha256, archive_bytes, installed_at, updated_at, install_kind, installer_family, install_state, state_reason)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"endpoint-1", "game-1", "source-1", "profile-1", `C:\Games`, `C:\Games\Legacy Duke`, strings.Repeat("a", 64), 42, now, now, "gog_inno", "gog_inno", "attention_required", "installer_exit_nonzero"}},
	} {
		if _, err := dbSvc.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed version 17 row: %v", err)
		}
	}

	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("apply migration 18: %v", err)
	}
	assertLatestMigrationVersion(t, dbSvc.GetDB())
	var state, reason string
	var marker, ignoredBy sql.NullString
	var ignoredAt sql.NullInt64
	if err := dbSvc.GetDB().QueryRow(`SELECT install_state, state_reason, cleanup_marker_id, cleanup_ignored_at, cleanup_ignored_by_profile_id
		FROM device_game_installations WHERE endpoint_id='endpoint-1'`).Scan(&state, &reason, &marker, &ignoredAt, &ignoredBy); err != nil {
		t.Fatalf("read migrated failed installation: %v", err)
	}
	if state != "attention_required" || reason != "installer_exit_nonzero" || marker.Valid || ignoredAt.Valid || ignoredBy.Valid {
		t.Fatalf("migration changed legacy row: state=%q reason=%q marker=%#v ignoredAt=%#v ignoredBy=%#v", state, reason, marker, ignoredAt, ignoredBy)
	}
	var executionMode string
	if err := dbSvc.GetDB().QueryRow(`SELECT execution_mode FROM device_endpoints WHERE id='endpoint-1'`).Scan(&executionMode); err != nil {
		t.Fatalf("read migrated endpoint execution mode: %v", err)
	}
	if executionMode != "standard" {
		t.Fatalf("migrated endpoint execution mode = %q, want standard", executionMode)
	}
	var eventCount int
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM device_installation_events`).Scan(&eventCount); err != nil || eventCount != 0 {
		t.Fatalf("migration synthesized events: count=%d error=%v", eventCount, err)
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

func TestMigration20PreservesEventsAndAddsVerificationDefaults(t *testing.T) {
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
		if migration.Version > 19 {
			break
		}
		if err := dbSvc.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}
	now := time.Now().Unix()
	for _, statement := range []string{
		`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES ('profile-1','Player','admin_player',` + fmt.Sprint(now) + `,` + fmt.Sprint(now) + `)`,
		`INSERT INTO canonical_games (id, created_at) VALUES ('game-1',` + fmt.Sprint(now) + `)`,
		`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at) VALUES ('source-1','profile-1','i','p','e','Game','windows_pc','base_game','packed','found','matched',` + fmt.Sprint(now) + `)`,
		`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, execution_mode, client_version, protocol_version, capabilities_json, status, created_at, updated_at) VALUES ('endpoint-1','instance-1','key','PC','pc','user','windows','amd64','standard','dev',1,'[]','offline',` + fmt.Sprint(now) + `,` + fmt.Sprint(now) + `)`,
		`INSERT INTO device_game_installations (endpoint_id, game_id, source_game_id, profile_id, install_root, install_path, archive_sha256, archive_bytes, installed_at, updated_at, install_kind, install_state) VALUES ('endpoint-1','game-1','source-1','profile-1','C:\Games','C:\Games\Game','hash',1,` + fmt.Sprint(now) + `,` + fmt.Sprint(now) + `,'managed_archive','installed')`,
		`INSERT INTO device_installation_events (id, endpoint_id, game_id, source_game_id, actor_profile_id, event_type, reason, details_json, created_at) VALUES ('event-1','endpoint-1','game-1','source-1','profile-1','failure_ignored','old','{}',` + fmt.Sprint(now) + `)`,
	} {
		if _, err := dbSvc.GetDB().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	var reason, details string
	if err := dbSvc.GetDB().QueryRow(`SELECT COALESCE(verification_reason_code,''), verification_details_json FROM device_game_installations`).Scan(&reason, &details); err != nil || reason != "" || details != "{}" {
		t.Fatalf("verification defaults = %q %q, error = %v", reason, details, err)
	}
	var preserved int
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM device_installation_events WHERE id='event-1' AND event_type='failure_ignored'`).Scan(&preserved); err != nil || preserved != 1 {
		t.Fatalf("preserved events = %d, error = %v", preserved, err)
	}
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO device_installation_events (id, endpoint_id, game_id, source_game_id, actor_profile_id, event_type, details_json, created_at) VALUES ('event-2','endpoint-1','game-1','source-1','profile-1','installation_missing','{}',?)`, now); err != nil {
		t.Fatalf("new reconciliation event rejected: %v", err)
	}
}

func TestMigration21AddsEmptyCascadingDeviceInstallPreferences(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabaseWithMigrationOptions(testLogger{}, testDBConfig{dbPath: dbPath}, core.MigrationOptions{BackupBeforeMigrate: false}).(*sqliteDatabase)
	if err := dbSvc.Connect(); err != nil {
		t.Fatal(err)
	}
	defer dbSvc.Close()
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	for _, statement := range []string{
		`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES ('profile-21','Player','admin_player',` + fmt.Sprint(now) + `,` + fmt.Sprint(now) + `)`,
		`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, execution_mode, client_version, protocol_version, capabilities_json, status, created_at, updated_at) VALUES ('endpoint-21','instance-21','key','PC','pc','user','windows','amd64','standard','dev',1,'[]','offline',` + fmt.Sprint(now) + `,` + fmt.Sprint(now) + `)`,
	} {
		if _, err := dbSvc.GetDB().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM device_install_preferences`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("migration backfilled preferences: count=%d error=%v", count, err)
	}
	if _, err := dbSvc.GetDB().Exec(`INSERT INTO device_install_preferences (endpoint_id, install_root_template, updated_by_profile_id, updated_at) VALUES ('endpoint-21','C:\Games','profile-21',?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := dbSvc.GetDB().Exec(`DELETE FROM device_endpoints WHERE id='endpoint-21'`); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM device_install_preferences`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("endpoint preference did not cascade: count=%d error=%v", count, err)
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

func TestVersionAwareIdentityMigrationBackfillsExistingCanonicalIDs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	dbSvc := NewSQLiteDatabaseWithMigrationOptions(testLogger{}, testDBConfig{dbPath: dbPath}, core.MigrationOptions{BackupBeforeMigrate: false}).(*sqliteDatabase)
	if err := dbSvc.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dbSvc.Close()
	if err := dbSvc.ensureSchemaMigrationsTable(); err != nil {
		t.Fatalf("ensure migrations table: %v", err)
	}
	for _, migration := range dbSvc.orderedMigrations() {
		if migration.Version >= 13 {
			break
		}
		if err := dbSvc.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run pre-identity migration %d: %v", migration.Version, err)
		}
	}

	now := time.Now().Unix()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, []any{"profile-identity", "Player", "admin_player", now, now}},
		{`INSERT INTO canonical_games (id, created_at) VALUES (?, ?)`, []any{"canonical-preserved", now}},
		{`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"source-preserved", "profile-identity", "integration-1", "game-source-steam", "source-1", "Double Dragon", "windows_pc", "base_game", "self_contained", "found", "matched", now}},
		{`INSERT INTO canonical_source_games_link (canonical_id, source_game_id) VALUES (?, ?)`, []any{"canonical-preserved", "source-preserved"}},
		{`INSERT INTO metadata_resolver_matches (source_game_id, plugin_id, external_id, title, platform, outvoted, created_at)
			VALUES (?, ?, ?, ?, ?, 0, ?)`, []any{"source-preserved", "metadata-steam", "provider-1", "Double Dragon", "windows_pc", now}},
	} {
		if _, err := dbSvc.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed pre-identity database: %v", err)
		}
	}

	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatalf("apply identity migration: %v", err)
	}
	var editionID, titleID, displayTitle, state string
	if err := dbSvc.GetDB().QueryRow(`SELECT e.id, e.title_id, t.display_title, e.identity_state
		FROM game_editions e JOIN game_titles t ON t.id=e.title_id WHERE e.id=?`, "canonical-preserved").Scan(&editionID, &titleID, &displayTitle, &state); err != nil {
		t.Fatalf("read migrated identity: %v", err)
	}
	if editionID != "canonical-preserved" {
		t.Fatalf("edition id = %q, want preserved canonical id", editionID)
	}
	if titleID == "" || displayTitle != "Double Dragon" || state != "provider_confirmed" {
		t.Fatalf("migrated identity = title_id:%q title:%q state:%q", titleID, displayTitle, state)
	}
	var evidenceCount int
	if err := dbSvc.GetDB().QueryRow(`SELECT COUNT(*) FROM game_title_external_ids WHERE title_id=? AND provider=? AND external_id=?`, titleID, "metadata-steam", "provider-1").Scan(&evidenceCount); err != nil {
		t.Fatalf("count migrated evidence: %v", err)
	}
	if evidenceCount != 1 {
		t.Fatalf("migrated evidence count = %d, want 1", evidenceCount)
	}
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
