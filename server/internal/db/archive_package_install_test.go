package db

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
)

func TestCompleteArchivePackageCommandPersistsGogInnoSuccess(t *testing.T) {
	store, endpoint, profile, now := newArchivePackageStore(t)
	command := createRunningArchivePackageCommand(t, store, endpoint.ID, profile.ID, now)
	exitCode := 0
	result := archivePackageGogResult(now, &exitCode, devicev1.GogInnoCompletionExitZero, "")
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteCommand(context.Background(), endpoint.ID, devicev1.CommandResult{
		CommandID: command.ID, Status: devicev1.CommandSucceeded, Payload: payload,
	}, now.Add(3*time.Second)); err != nil {
		t.Fatalf("CompleteCommand() error = %v", err)
	}

	installations, err := store.ListInstallations(context.Background(), endpoint.ID, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 1 || installations[0].InstallKind != devicev1.InstallKindGogInno ||
		installations[0].InstallState != devicev1.InstallStateInstalled ||
		installations[0].ArchiveSHA256 != strings.Repeat("b", 64) || installations[0].ArchiveBytes != 128 {
		t.Fatalf("compressed installer persistence = %+v", installations)
	}
}

func TestCompleteArchivePackageCommandPersistsGogInnoFailureEvidence(t *testing.T) {
	store, endpoint, profile, now := newArchivePackageStore(t)
	command := createRunningArchivePackageCommand(t, store, endpoint.ID, profile.ID, now)
	marker := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	result := archivePackageGogResult(now, nil, "", marker)
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteCommand(context.Background(), endpoint.ID, devicev1.CommandResult{
		CommandID: command.ID, Status: devicev1.CommandFailed, Payload: payload,
		Error: &devicev1.ProtocolError{Code: "installer_failed", Message: "Installer stopped before it finished"},
	}, now.Add(3*time.Second)); err != nil {
		t.Fatalf("CompleteCommand() error = %v", err)
	}

	installations, err := store.ListInstallations(context.Background(), endpoint.ID, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 1 || installations[0].InstallState != devicev1.InstallStateCleanupRequired ||
		installations[0].CleanupMarkerID != marker || !strings.Contains(installations[0].StateReason, "Installer stopped") {
		t.Fatalf("compressed installer failure persistence = %+v", installations)
	}
}

func newArchivePackageStore(t *testing.T) (*DeviceStore, devices.Endpoint, *core.Profile, time.Time) {
	t.Helper()
	ctx := context.Background()
	database := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "archive-package.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	profile := &core.Profile{ID: "profile-archive-package", DisplayName: "Player", Role: core.ProfileRolePlayer, CreatedAt: now, UpdatedAt: now}
	if err := NewProfileRepository(database).Create(ctx, profile); err != nil {
		t.Fatal(err)
	}
	store := NewDeviceStore(database)
	challenge := devices.PairingChallenge{ID: "challenge-archive-package", CodeHash: "archive-package-code", ProfileID: profile.ID, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := store.CreatePairingChallenge(ctx, challenge); err != nil {
		t.Fatal(err)
	}
	endpoint := devices.Endpoint{
		ID: "endpoint-archive-package", ClientInstanceID: "instance-archive-package",
		PublicKey: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), DisplayName: "PC / Player",
		HostName: "pc", OSUser: "player", Platform: "windows", Arch: "amd64", ClientVersion: "test",
		ProtocolVersion: devicev1.Version, Capabilities: []string{devicev1.CapabilityGameInstallArchivePackage},
		Status: devicev1.EndpointOffline, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := store.PairEndpoint(ctx, challenge.CodeHash, now, endpoint); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetDB().ExecContext(ctx, `INSERT INTO canonical_games(id, created_at) VALUES ('game-package', ?)`, now.Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetDB().ExecContext(ctx, `INSERT INTO source_games
		(id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, root_path, status, created_at)
		VALUES ('source-package', ?, 'integration-package', 'game-source-google-drive', 'archive-package', 'Package Game', 'windows_pc', 'base_game', 'packed', 'Games/Installers', 'found', ?)`, profile.ID, now.Unix()); err != nil {
		t.Fatal(err)
	}
	return store, endpoint, profile, now
}

func createRunningArchivePackageCommand(t *testing.T, store *DeviceStore, endpointID, profileID string, now time.Time) devices.Command {
	t.Helper()
	request := devicev1.ArchivePackageInstallRequest{
		GameID: "game-package", SourceGameID: "source-package", Title: "Package Game",
		ArchiveName: "package.zip", ArchiveFormat: devicev1.ArchiveFormatZIP, ArchiveSize: 128,
		DownloadURL: "/api/device-transfers/archive", DownloadToken: "secret", DestinationName: "Package Game",
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	command := devices.Command{
		ID: "command-archive-package", EndpointID: endpointID, ProfileID: profileID,
		Name: devicev1.CapabilityGameInstallArchivePackage, SchemaVersion: devicev1.ArchivePackageInstallSchemaVersion,
		IdempotencyKey: "archive-package-idempotency", Status: devicev1.CommandDispatched, Payload: payload,
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	if err := store.CreateCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateCommandStatus(context.Background(), endpointID, command.ID, devicev1.CommandAccepted, nil, nil, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateCommandStatus(context.Background(), endpointID, command.ID, devicev1.CommandRunning, nil, nil, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	return command
}

func archivePackageGogResult(now time.Time, exitCode *int, completionBasis, marker string) devicev1.ArchivePackageInstallResult {
	installerHash := strings.Repeat("a", 64)
	native := devicev1.GogInnoInstallResult{
		GameID: "game-package", SourceGameID: "source-package", InstallRoot: `C:\Games`, InstallPath: `C:\Games\Package Game`,
		InstallerFamily: devicev1.GogInnoInstallerFamily, PrimarySHA256: installerHash, TotalPackageBytes: 64,
		PackageFiles:  []devicev1.GogInnoPackageFile{{FileName: "setup_package_game.exe", Role: devicev1.PackageTransferRoleInstaller, SizeBytes: 64, SHA256: installerHash}},
		SignerSubject: "GOG Sp. z o.o.", SignerThumbprint: "thumbprint", InvocationMode: devicev1.GogInnoInvocationFixedSilent,
		UninstallTarget: "unins000.exe", LaunchTarget: "Package Game.exe", LaunchCandidates: []string{"Package Game.exe"},
		ExitCode: exitCode, InstalledAt: now, CompletionBasis: completionBasis, CleanupMarkerID: marker,
	}
	if completionBasis == "" {
		native.UninstallTarget, native.LaunchTarget, native.LaunchCandidates, native.InstalledAt = "", "", nil, time.Time{}
	}
	return devicev1.ArchivePackageInstallResult{
		ResolvedKind: devicev1.ArchivePackageKindGogInno,
		GogInno: &devicev1.GogInnoArchiveInstallResult{
			GogInnoInstallResult: native,
			Container:            devicev1.ArchiveContainerEvidence{FileName: "package.zip", Format: devicev1.ArchiveFormatZIP, SizeBytes: 128, SHA256: strings.Repeat("b", 64)},
		},
	}
}
