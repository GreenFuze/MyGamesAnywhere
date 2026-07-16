package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
)

func TestDeviceStoreFailedInstallCleanupIgnoreAndEventsRoundTrip(t *testing.T) {
	t.Parallel()
	database := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "failed-install.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES (?, ?, 'admin_player', ?, ?)`, []any{"profile-1", "Admin", now.Unix(), now.Unix()}},
		{`INSERT INTO canonical_games (id, created_at) VALUES (?, ?)`, []any{"game-1", now.Unix()}},
		{`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at)
			VALUES (?, ?, 'integration-1', 'game-source-google-drive', 'source-1', 'Game', 'windows_pc', 'base_game', 'packed', 'found', 'matched', ?)`, []any{"source-1", "profile-1", now.Unix()}},
		{`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, client_version, protocol_version, capabilities_json, status, created_at, updated_at)
			VALUES (?, 'instance-1', 'key', 'PC', 'pc', 'user', 'windows', 'amd64', 'dev', 1, '[]', 'offline', ?, ?)`, []any{"endpoint-1", now.Unix(), now.Unix()}},
	} {
		if _, err := database.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed database: %v", err)
		}
	}

	store := NewDeviceStore(database)
	installRequest := devicev1.GogInnoInstallRequest{
		GameID: "game-1", SourceGameID: "source-1", Title: "Game", DestinationName: "Game",
		Installer: devicev1.PackageTransferDescriptor{FileName: "setup_game.exe", Role: devicev1.PackageTransferRoleInstaller,
			SizeBytes: 42, DownloadURL: "/api/device-transfers/package", DownloadToken: "token"},
	}
	installPayload, _ := json.Marshal(installRequest)
	installCommand := devices.Command{
		ID: "install-failed", EndpointID: "endpoint-1", ProfileID: "profile-1", Name: devicev1.CapabilityGameInstallGogInno,
		SchemaVersion: 1, IdempotencyKey: "install-idem", Status: devicev1.CommandDispatched, Payload: installPayload,
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	createRunningCommand(t, store, installCommand, now)
	exitCode := 7
	markerID := strings.Repeat("A", 43)
	failed := devicev1.GogInnoInstallResult{
		GameID: "game-1", SourceGameID: "source-1", InstallRoot: `C:\Games`, InstallPath: `C:\Games\Game`,
		InstallerFamily: devicev1.GogInnoInstallerFamily, PrimarySHA256: strings.Repeat("a", 64), TotalPackageBytes: 42,
		PackageFiles:  []devicev1.GogInnoPackageFile{{FileName: "setup_game.exe", Role: devicev1.PackageTransferRoleInstaller, SizeBytes: 42, SHA256: strings.Repeat("a", 64)}},
		SignerSubject: "GOG Sp. z o.o.", SignerThumbprint: "thumb", InvocationMode: devicev1.GogInnoInvocationFixedSilent,
		UninstallTarget: "unins000.exe", ProcessID: 4242, ExitCode: &exitCode, DiagnosticRef: ".mga/staging/install-failed/installer.log",
		CleanupMarkerID: markerID,
	}
	failedPayload, _ := json.Marshal(failed)
	if err := store.CompleteCommand(context.Background(), "endpoint-1", devicev1.CommandResult{
		CommandID: installCommand.ID, Status: devicev1.CommandFailed, Payload: failedPayload,
		Error: &devicev1.ProtocolError{Code: "installer_exit_nonzero", Message: "installer failed"},
	}, now.Add(3*time.Second)); err != nil {
		t.Fatalf("CompleteCommand(failed install) error = %v", err)
	}
	installations, err := store.ListInstallations(context.Background(), "endpoint-1", "profile-1")
	if err != nil || len(installations) != 1 {
		t.Fatalf("ListInstallations() = %+v, error = %v", installations, err)
	}
	installation := installations[0]
	if installation.InstallState != devicev1.InstallStateCleanupRequired || installation.CleanupMarkerID != markerID || installation.UninstallTarget != "unins000.exe" {
		t.Fatalf("failed installation = %#v", installation)
	}

	ignoredAt := now.Add(4 * time.Second)
	if err := store.SetInstallationFailureState(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1",
		devicev1.InstallStateIgnoredFailure, "", markerID, &ignoredAt, "profile-1", "failure_ignored", json.RawMessage(`{}`), ignoredAt); err != nil {
		t.Fatalf("SetInstallationFailureState(ignore) error = %v", err)
	}
	installations, err = store.ListInstallations(context.Background(), "endpoint-1", "profile-1")
	if err != nil || len(installations) != 1 || installations[0].InstallState != devicev1.InstallStateIgnoredFailure ||
		installations[0].CleanupIgnoredAt == nil || !installations[0].CleanupIgnoredAt.Equal(ignoredAt) || installations[0].CleanupIgnoredByProfileID != "profile-1" {
		t.Fatalf("ignored installation = %+v, error = %v", installations, err)
	}

	reopenedAt := now.Add(5 * time.Second)
	if err := store.SetInstallationFailureState(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1",
		devicev1.InstallStateCleanupRequired, "", markerID, nil, "", "failure_reopened", json.RawMessage(`{}`), reopenedAt); err != nil {
		t.Fatalf("SetInstallationFailureState(reopen) error = %v", err)
	}
	cleanupStartedAt := reopenedAt.Add(time.Second)
	if err := store.SetInstallationFailureState(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1",
		devicev1.InstallStateCleanupRunning, "Cleanup requested", markerID, nil, "", "cleanup_started", json.RawMessage(`{}`), cleanupStartedAt); err != nil {
		t.Fatalf("SetInstallationFailureState(cleanup start) error = %v", err)
	}
	cleanupRequest := devicev1.GogInnoFailedCleanupRequest{
		GameID: "game-1", SourceGameID: "source-1", InstallRoot: `C:\Games`, InstallPath: `C:\Games\Game`,
		InstallerFamily: devicev1.GogInnoInstallerFamily, CleanupMarkerID: markerID,
		PrimarySHA256: strings.Repeat("a", 64), UninstallTarget: "unins000.exe",
	}
	cleanupPayload, _ := json.Marshal(cleanupRequest)
	cleanupCommand := devices.Command{
		ID: "cleanup-declined", EndpointID: "endpoint-1", ProfileID: "profile-1", Name: devicev1.CapabilityGameCleanupGogInnoFailed,
		SchemaVersion: 1, IdempotencyKey: "cleanup-declined-idem", Status: devicev1.CommandDispatched, Payload: cleanupPayload,
		CreatedAt: cleanupStartedAt, UpdatedAt: cleanupStartedAt, ExpiresAt: cleanupStartedAt.Add(time.Hour),
	}
	createRunningCommand(t, store, cleanupCommand, cleanupStartedAt)
	if err := store.CompleteCommand(context.Background(), "endpoint-1", devicev1.CommandResult{
		CommandID: cleanupCommand.ID, Status: devicev1.CommandFailed,
		Error: &devicev1.ProtocolError{Code: "local_confirmation_declined", Message: "local confirmation declined"},
	}, cleanupStartedAt.Add(3*time.Second)); err != nil {
		t.Fatalf("CompleteCommand(declined cleanup) error = %v", err)
	}
	installations, err = store.ListInstallations(context.Background(), "endpoint-1", "profile-1")
	if err != nil || len(installations) != 1 || installations[0].InstallState != devicev1.InstallStateCleanupRequired {
		t.Fatalf("declined cleanup state = %+v, error = %v", installations, err)
	}

	secondCleanupStartedAt := cleanupStartedAt.Add(4 * time.Second)
	if err := store.SetInstallationFailureState(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1",
		devicev1.InstallStateCleanupRunning, "Cleanup requested", markerID, nil, "", "cleanup_started", json.RawMessage(`{}`), secondCleanupStartedAt); err != nil {
		t.Fatalf("SetInstallationFailureState(second cleanup start) error = %v", err)
	}
	cleanupCommand = devices.Command{
		ID: "cleanup-succeeded", EndpointID: "endpoint-1", ProfileID: "profile-1", Name: devicev1.CapabilityGameCleanupGogInnoFailed,
		SchemaVersion: 1, IdempotencyKey: "cleanup-succeeded-idem", Status: devicev1.CommandDispatched, Payload: cleanupPayload,
		CreatedAt: secondCleanupStartedAt, UpdatedAt: secondCleanupStartedAt, ExpiresAt: secondCleanupStartedAt.Add(time.Hour),
	}
	createRunningCommand(t, store, cleanupCommand, secondCleanupStartedAt)
	cleanedPayload, _ := json.Marshal(devicev1.GogInnoFailedCleanupResult{
		GameID: "game-1", SourceGameID: "source-1", Removed: true, PublisherUninstallerUsed: true,
		BoundedDeleteUsed: true, SystemChangesMayRemain: true,
	})
	if err := store.CompleteCommand(context.Background(), "endpoint-1", devicev1.CommandResult{
		CommandID: cleanupCommand.ID, Status: devicev1.CommandSucceeded, Payload: cleanedPayload,
	}, secondCleanupStartedAt.Add(3*time.Second)); err != nil {
		t.Fatalf("CompleteCommand(cleanup) error = %v", err)
	}
	installations, err = store.ListInstallations(context.Background(), "endpoint-1", "profile-1")
	if err != nil || len(installations) != 0 {
		t.Fatalf("cleaned installations = %+v, error = %v", installations, err)
	}

	rows, err := database.GetDB().Query(`SELECT event_type, actor_profile_id, details_json FROM device_installation_events
		WHERE endpoint_id='endpoint-1' AND game_id='game-1' AND source_game_id='source-1' ORDER BY created_at, rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var eventTypes []string
	for rows.Next() {
		var eventType, details string
		var actor sql.NullString
		if err := rows.Scan(&eventType, &actor, &details); err != nil {
			t.Fatal(err)
		}
		if !actor.Valid || actor.String != "profile-1" || !json.Valid([]byte(details)) {
			t.Fatalf("invalid event actor/details: actor=%#v details=%q", actor, details)
		}
		eventTypes = append(eventTypes, eventType)
	}
	wantEvents := []string{"failure_detected", "failure_ignored", "failure_reopened", "cleanup_started", "cleanup_failed", "cleanup_started", "cleanup_succeeded"}
	if strings.Join(eventTypes, ",") != strings.Join(wantEvents, ",") {
		t.Fatalf("events = %v, want %v", eventTypes, wantEvents)
	}
}

func createRunningCommand(t *testing.T, store *DeviceStore, command devices.Command, now time.Time) {
	t.Helper()
	if err := store.CreateCommand(context.Background(), command); err != nil {
		t.Fatalf("CreateCommand(%s) error = %v", command.ID, err)
	}
	if err := store.UpdateCommandStatus(context.Background(), command.EndpointID, command.ID, devicev1.CommandAccepted, nil, nil, now.Add(time.Second)); err != nil {
		t.Fatalf("accept command %s: %v", command.ID, err)
	}
	if err := store.UpdateCommandStatus(context.Background(), command.EndpointID, command.ID, devicev1.CommandRunning, nil, nil, now.Add(2*time.Second)); err != nil {
		t.Fatalf("run command %s: %v", command.ID, err)
	}
}
