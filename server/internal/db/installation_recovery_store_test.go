package db

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestCompleteInstallationRecoveryRepairsAndPreservesForgetHistory(t *testing.T) {
	database := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "recovery.sqlite")}).(*sqliteDatabase)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	root := filepath.Join(t.TempDir(), "Games")
	installPath := filepath.Join(root, "MGA", "Game")
	seedRecoveryIdentity(t, database, now, root, installPath)
	store := NewDeviceStore(database)

	repair := devicev1.InstallationRecoveryRequest{
		Action: devicev1.InstallationRecoveryRepair, GameID: "game-1", SourceGameID: "source-1",
		InstallKind: devicev1.InstallKindManagedArchive, InstallRoot: root, InstallPath: installPath,
		InstallState: devicev1.InstallStateNeedsRepair, ReasonCode: devicev1.ValidationReasonLaunchTargetMissing,
	}
	repairResult := devicev1.InstallationRecoveryResult{
		Action: repair.Action, GameID: repair.GameID, SourceGameID: repair.SourceGameID,
		LocalInstallationID: "11111111-1111-1111-1111-111111111111",
		LaunchTarget: "Game.exe", LaunchCandidates: []string{"Game.exe"}, PathPresent: true,
	}
	completeRecoveryCommand(t, database, store, "repair-1", now, repair, repairResult)
	var state, target, localID string
	if err := database.GetDB().QueryRow(`SELECT install_state, launch_target, local_installation_id FROM device_game_installations
		WHERE endpoint_id='endpoint-1' AND game_id='game-1'`).Scan(&state, &target, &localID); err != nil {
		t.Fatal(err)
	}
	if state != devicev1.InstallStateInstalled || target != "Game.exe" || localID != repairResult.LocalInstallationID {
		t.Fatalf("repaired row = state:%q target:%q local:%q", state, target, localID)
	}

	if _, err := database.GetDB().Exec(`UPDATE device_game_installations SET install_state='missing',
		state_reason=?, verification_reason_code=? WHERE endpoint_id='endpoint-1' AND game_id='game-1'`,
		devicev1.ValidationReasonInstallPathMissing, devicev1.ValidationReasonInstallPathMissing); err != nil {
		t.Fatal(err)
	}
	forget := devicev1.InstallationRecoveryRequest{
		Action: devicev1.InstallationRecoveryForget, GameID: "game-1", SourceGameID: "source-1",
		LocalInstallationID: repairResult.LocalInstallationID,
		InstallKind: devicev1.InstallKindManagedArchive, InstallRoot: root, InstallPath: installPath,
		InstallState: devicev1.InstallStateMissing, ReasonCode: devicev1.ValidationReasonInstallPathMissing,
	}
	forgetResult := devicev1.InstallationRecoveryResult{
		Action: forget.Action, GameID: forget.GameID, SourceGameID: forget.SourceGameID,
		LocalInstallationID: repairResult.LocalInstallationID, Released: true, PathPresent: false,
	}
	completeRecoveryCommand(t, database, store, "forget-1", now.Add(time.Minute), forget, forgetResult)
	var installations int
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM device_game_installations WHERE endpoint_id='endpoint-1'`).Scan(&installations); err != nil || installations != 0 {
		t.Fatalf("active installations = %d, error = %v", installations, err)
	}
	var history int
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM device_installation_events
		WHERE endpoint_id='endpoint-1' AND event_type IN ('installation_repair_succeeded','installation_forgotten')`).Scan(&history); err != nil || history != 2 {
		t.Fatalf("recovery history = %d, error = %v", history, err)
	}
}

func seedRecoveryIdentity(t *testing.T, database *sqliteDatabase, now time.Time, root, installPath string) {
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
		{`INSERT INTO device_game_installations
		 (endpoint_id, game_id, source_game_id, profile_id, install_root, install_path, archive_sha256, archive_bytes,
		  installed_at, updated_at, launch_target, launch_candidates_json, install_kind, install_state, state_reason,
		  verification_reason_code, verification_details_json, state_changed_at)
		 VALUES ('endpoint-1','game-1','source-1','profile-1',?,?,'hash',1,?,?,'Old.exe','["Old.exe"]','managed_archive',
		  'needs_repair',?,'launch_target_missing','{}',?)`,
			[]any{root, installPath, now.Unix(), now.Unix(), devicev1.ValidationReasonLaunchTargetMissing, now.Unix()}},
	}
	for _, statement := range statements {
		if _, err := database.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed recovery identity: %v", err)
		}
	}
}

func completeRecoveryCommand(t *testing.T, database *sqliteDatabase, store *DeviceStore, commandID string, now time.Time, request devicev1.InstallationRecoveryRequest, result devicev1.InstallationRecoveryResult) {
	t.Helper()
	payload, _ := json.Marshal(request)
	if _, err := database.GetDB().Exec(`INSERT INTO device_commands
		(id, endpoint_id, profile_id, name, schema_version, idempotency_key, status, payload_json, created_at, updated_at, expires_at)
		VALUES (?, 'endpoint-1','profile-1',?,1,?,'running',?,?,?,?)`,
		commandID, devicev1.CapabilityGameRecoverInstallation, "idem-"+commandID, string(payload), now.Unix(), now.Unix(), now.Add(time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	resultPayload, _ := json.Marshal(result)
	if err := store.CompleteCommand(context.Background(), "endpoint-1", devicev1.CommandResult{
		CommandID: commandID, Status: devicev1.CommandSucceeded, Payload: resultPayload,
	}, now); err != nil {
		t.Fatalf("CompleteCommand(%s): %v", commandID, err)
	}
}
