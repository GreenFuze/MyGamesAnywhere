package db

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
)

func TestCompleteValidationTransitionsMissingAndRestoredWithIdempotentEvents(t *testing.T) {
	database := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "reconciliation.sqlite")}).(*sqliteDatabase)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	seedReconciliationIdentity(t, database, now)
	store := NewDeviceStore(database)

	missing := devicev1.InstallationValidationResult{Items: []devicev1.InstallationValidationResultItem{{
		GameID: "game-1", SourceGameID: "source-1", State: devicev1.InstallStateMissing,
		ReasonCode: devicev1.ValidationReasonInstallPathMissing, CheckedAt: now,
	}}, Missing: 1}
	completeValidationCommand(t, database, store, "validation-1", "idem-1", now, missing)
	installation := readSingleReconciledInstallation(t, store)
	if installation.InstallState != devicev1.InstallStateMissing || installation.VerificationReasonCode != devicev1.ValidationReasonInstallPathMissing || installation.LastVerifiedAt == nil {
		t.Fatalf("missing installation = %#v", installation)
	}
	var firstEvents int
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM device_installation_events WHERE event_type='installation_missing'`).Scan(&firstEvents); err != nil || firstEvents != 1 {
		t.Fatalf("missing events = %d, error = %v", firstEvents, err)
	}
	command, err := store.GetCommand(context.Background(), "endpoint-1", "validation-1")
	if err != nil {
		t.Fatal(err)
	}
	var enriched devicev1.InstallationValidationResult
	if err := json.Unmarshal(command.Result, &enriched); err != nil || enriched.ChangedMissing != 1 {
		t.Fatalf("enriched result = %#v, error = %v", enriched, err)
	}

	completeValidationCommand(t, database, store, "validation-2", "idem-2", now.Add(time.Minute), missing)
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM device_installation_events WHERE event_type='installation_missing'`).Scan(&firstEvents); err != nil || firstEvents != 1 {
		t.Fatalf("repeated missing created event: count=%d error=%v", firstEvents, err)
	}

	healthy := devicev1.InstallationValidationResult{Items: []devicev1.InstallationValidationResultItem{{
		GameID: "game-1", SourceGameID: "source-1", State: devicev1.InstallStateInstalled,
		ReasonCode: devicev1.ValidationReasonHealthy, CheckedAt: now.Add(2 * time.Minute), ManifestSchema: 2,
	}}, Installed: 1}
	completeValidationCommand(t, database, store, "validation-3", "idem-3", now.Add(2*time.Minute), healthy)
	installation = readSingleReconciledInstallation(t, store)
	if installation.InstallState != devicev1.InstallStateInstalled || installation.StateReason != "" || installation.VerificationReasonCode != devicev1.ValidationReasonHealthy {
		t.Fatalf("restored installation = %#v", installation)
	}
	var restoredEvents int
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM device_installation_events WHERE event_type='installation_restored'`).Scan(&restoredEvents); err != nil || restoredEvents != 1 {
		t.Fatalf("restored events = %d, error = %v", restoredEvents, err)
	}
}

func seedReconciliationIdentity(t *testing.T, database *sqliteDatabase, now time.Time) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES ('profile-1','Player','admin_player',?,?)`, []any{now.Unix(), now.Unix()}},
		{`INSERT INTO canonical_games (id, created_at) VALUES ('game-1',?)`, []any{now.Unix()}},
		{`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at)
		 VALUES ('source-1','profile-1','integration-1','game-source-google-drive','source-1','Game','windows_pc','base_game','packed','found','matched',?)`, []any{now.Unix()}},
		{`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, execution_mode, client_version, protocol_version, capabilities_json, status, created_at, updated_at)
		 VALUES ('endpoint-1','instance-1','key','PC','pc','user','windows','amd64','standard','dev',1,'[]','ready',?,?)`, []any{now.Unix(), now.Unix()}},
		{`INSERT INTO device_grants (endpoint_id, profile_id, access_level, created_at, updated_at) VALUES ('endpoint-1','profile-1','owner',?,?)`, []any{now.Unix(), now.Unix()}},
		{`INSERT INTO device_game_installations (endpoint_id, game_id, source_game_id, profile_id, install_root, install_path, archive_sha256, archive_bytes, installed_at, updated_at, launch_target, launch_candidates_json, install_kind, install_state, state_changed_at)
		 VALUES ('endpoint-1','game-1','source-1','profile-1','C:\Games','C:\Games\Game','hash',1,?,?,'Game.exe','["Game.exe"]','managed_archive','installed',?)`, []any{now.Unix(), now.Unix(), now.Unix()}},
	}
	for _, statement := range statements {
		if _, err := database.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed reconciliation identity: %v", err)
		}
	}
}

func completeValidationCommand(t *testing.T, database *sqliteDatabase, store *DeviceStore, id, idempotency string, now time.Time, validation devicev1.InstallationValidationResult) {
	t.Helper()
	request := devicev1.InstallationValidationRequest{Trigger: "manual", Items: []devicev1.InstallationValidationRequestItem{{
		GameID: "game-1", SourceGameID: "source-1", InstallKind: devicev1.InstallKindManagedArchive,
		InstallRoot: `C:\Games`, InstallPath: `C:\Games\Game`, LaunchTarget: "Game.exe",
	}}}
	payload, _ := json.Marshal(request)
	if _, err := database.GetDB().Exec(`INSERT INTO device_commands (id, endpoint_id, profile_id, name, schema_version, idempotency_key, status, payload_json, created_at, updated_at, expires_at)
		VALUES (?, 'endpoint-1','profile-1',?,1,?,'running',?,?,?,?)`, id, devicev1.CapabilityGameValidateInstallations, idempotency, string(payload), now.Unix(), now.Unix(), now.Add(time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	resultPayload, _ := json.Marshal(validation)
	if err := store.CompleteCommand(context.Background(), "endpoint-1", devicev1.CommandResult{CommandID: id, Status: devicev1.CommandSucceeded, Payload: resultPayload}, now); err != nil {
		t.Fatalf("CompleteCommand(%s): %v", id, err)
	}
}

func readSingleReconciledInstallation(t *testing.T, store *DeviceStore) devices.GameInstallation {
	t.Helper()
	installations, err := store.ListInstallations(context.Background(), "endpoint-1", "profile-1")
	if err != nil || len(installations) != 1 {
		t.Fatalf("ListInstallations() = %#v, error = %v", installations, err)
	}
	return installations[0]
}
