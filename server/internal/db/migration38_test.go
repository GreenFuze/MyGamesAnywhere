package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestMigration38AddsDurableCommandReplayEvidence(t *testing.T) {
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
		if migration.Version > 37 {
			break
		}
		if err := dbSvc.runMigration(context.Background(), migration); err != nil {
			t.Fatalf("run migration %d: %v", migration.Version, err)
		}
	}
	now := time.Now().Unix()
	fixtures := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO profiles(id, display_name, role, created_at, updated_at) VALUES ('profile-38','Player','player',?,?)`, []any{now, now}},
		{`INSERT INTO device_endpoints
			(id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, execution_mode,
			 client_version, protocol_version, capabilities_json, status, created_at, updated_at)
			VALUES ('endpoint-38','instance-38','key','PC','pc','user','windows','amd64','standard','dev',1,'[]','offline',?,?)`, []any{now, now}},
		{`INSERT INTO device_commands
			(id, endpoint_id, profile_id, name, schema_version, idempotency_key, status, payload_json, created_at, updated_at, expires_at)
			VALUES ('legacy-command','endpoint-38','profile-38','endpoint.ping',1,'legacy-idem','failed','{}',?,?,?)`, []any{now, now, now + 60}},
	}
	for _, fixture := range fixtures {
		if _, err := dbSvc.GetDB().Exec(fixture.query, fixture.args...); err != nil {
			t.Fatalf("seed pre-migration fixture: %v", err)
		}
	}
	if err := dbSvc.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	assertLatestMigrationVersion(t, dbSvc.GetDB())
	columns := map[string]bool{}
	rows, err := dbSvc.GetDB().Query(`PRAGMA table_info(device_commands)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var position, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&position, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	for _, name := range []string{"request_fingerprint", "result_fingerprint", "replay_count", "last_replayed_at"} {
		if !columns[name] {
			t.Fatalf("migration 38 did not add %s", name)
		}
	}
	var status, requestFingerprint, resultFingerprint string
	var replayCount int
	var lastReplayedAt any
	if err := dbSvc.GetDB().QueryRow(`SELECT status, request_fingerprint, result_fingerprint, replay_count, last_replayed_at
		FROM device_commands WHERE id='legacy-command'`).Scan(&status, &requestFingerprint, &resultFingerprint, &replayCount, &lastReplayedAt); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || requestFingerprint != "" || resultFingerprint != "" || replayCount != 0 || lastReplayedAt != nil {
		t.Fatalf("legacy command changed by migration: status=%q request=%q result=%q replay_count=%d last_replayed_at=%v",
			status, requestFingerprint, resultFingerprint, replayCount, lastReplayedAt)
	}
}
