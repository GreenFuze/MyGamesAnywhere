package db

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
)

func TestDeviceStorePairsListsAndTracksCommands(t *testing.T) {
	t.Parallel()

	database := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "devices.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	profiles := NewProfileRepository(database)
	now := time.Now().Truncate(time.Second)
	profile := &core.Profile{ID: "profile-1", DisplayName: "Admin", Role: core.ProfileRoleAdminPlayer, CreatedAt: now, UpdatedAt: now}
	if err := profiles.Create(context.Background(), profile); err != nil {
		t.Fatalf("Create(profile) error = %v", err)
	}
	secondProfile := &core.Profile{ID: "profile-2", DisplayName: "Player", Role: core.ProfileRolePlayer, CreatedAt: now, UpdatedAt: now}
	if err := profiles.Create(context.Background(), secondProfile); err != nil {
		t.Fatalf("Create(second profile) error = %v", err)
	}
	store := NewDeviceStore(database)
	challenge := devices.PairingChallenge{ID: "challenge-1", CodeHash: "code-hash", ProfileID: profile.ID, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := store.CreatePairingChallenge(context.Background(), challenge); err != nil {
		t.Fatalf("CreatePairingChallenge() error = %v", err)
	}
	endpoint := devices.Endpoint{
		ID:               "endpoint-1",
		ClientInstanceID: "instance-1",
		PublicKey:        base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
		DisplayName:      "PC / Alice",
		HostName:         "pc",
		OSUser:           "alice",
		Platform:         "windows",
		Arch:             "amd64",
		ClientVersion:    "dev",
		ProtocolVersion:  devicev1.Version,
		Capabilities:     []string{devicev1.CapabilityEndpointPing},
		Status:           devicev1.EndpointOffline,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	pairedProfileID, err := store.PairEndpoint(context.Background(), challenge.CodeHash, now, endpoint)
	if err != nil {
		t.Fatalf("PairEndpoint() error = %v", err)
	}
	if pairedProfileID != profile.ID {
		t.Fatalf("paired profile = %q, want %q", pairedProfileID, profile.ID)
	}
	endpoints, err := store.ListEndpoints(context.Background(), profile.ID)
	if err != nil {
		t.Fatalf("ListEndpoints() error = %v", err)
	}
	if len(endpoints) != 1 || endpoints[0].AccessLevel != devicev1.AccessOwner {
		t.Fatalf("endpoints = %+v", endpoints)
	}
	if err := store.SetGrant(context.Background(), endpoint.ID, secondProfile.ID, devicev1.AccessView, now); err != nil {
		t.Fatalf("SetGrant(view) error = %v", err)
	}
	grants, err := store.ListGrants(context.Background(), endpoint.ID)
	if err != nil || len(grants) != 2 {
		t.Fatalf("ListGrants() = %+v, error = %v", grants, err)
	}
	if err := store.SetGrant(context.Background(), endpoint.ID, profile.ID, devicev1.AccessManage, now); !errors.Is(err, devices.ErrLastOwner) {
		t.Fatalf("demote last owner error = %v, want ErrLastOwner", err)
	}
	if err := store.SetGrant(context.Background(), endpoint.ID, secondProfile.ID, devicev1.AccessOwner, now); err != nil {
		t.Fatalf("SetGrant(owner) error = %v", err)
	}
	if err := store.SetGrant(context.Background(), endpoint.ID, profile.ID, devicev1.AccessManage, now); err != nil {
		t.Fatalf("SetGrant(demote with another owner) error = %v", err)
	}
	if err := store.DeleteGrant(context.Background(), endpoint.ID, secondProfile.ID); !errors.Is(err, devices.ErrLastOwner) {
		t.Fatalf("DeleteGrant(last owner) error = %v, want ErrLastOwner", err)
	}
	command := devices.Command{
		ID: "command-1", EndpointID: endpoint.ID, ProfileID: profile.ID, Name: devicev1.CapabilityEndpointPing,
		SchemaVersion: 1, IdempotencyKey: "idem-1", Status: devicev1.CommandDispatched, Payload: json.RawMessage(`{}`),
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	if err := store.CreateCommand(context.Background(), command); err != nil {
		t.Fatalf("CreateCommand() error = %v", err)
	}
	if err := store.UpdateCommandStatus(context.Background(), endpoint.ID, command.ID, devicev1.CommandAccepted, nil, nil, now.Add(time.Second)); err != nil {
		t.Fatalf("UpdateCommandStatus() error = %v", err)
	}
	if err := store.UpdateCommandStatus(context.Background(), endpoint.ID, command.ID, devicev1.CommandRunning, nil, nil, now.Add(2*time.Second)); err != nil {
		t.Fatalf("UpdateCommandStatus(running) error = %v", err)
	}
	result := json.RawMessage(`{"pong":true}`)
	if err := store.UpdateCommandStatus(context.Background(), endpoint.ID, command.ID, devicev1.CommandSucceeded, result, nil, now.Add(3*time.Second)); err != nil {
		t.Fatalf("UpdateCommandStatus(succeeded) error = %v", err)
	}
	if err := store.UpdateCommandStatus(context.Background(), "other-endpoint", command.ID, devicev1.CommandFailed, nil, nil, now.Add(4*time.Second)); !errors.Is(err, devices.ErrCommandNotFound) {
		t.Fatalf("UpdateCommandStatus(other endpoint) error = %v, want ErrCommandNotFound", err)
	}
	commands, err := store.ListCommands(context.Background(), endpoint.ID, profile.ID, 20)
	if err != nil {
		t.Fatalf("ListCommands() error = %v", err)
	}
	if len(commands) != 1 || commands[0].Status != devicev1.CommandSucceeded || string(commands[0].Result) != string(result) {
		t.Fatalf("commands = %+v", commands)
	}

	inventoryCommand := devices.Command{
		ID: "command-inventory", EndpointID: endpoint.ID, ProfileID: profile.ID, Name: devicev1.CapabilityInventoryRefresh,
		SchemaVersion: 1, IdempotencyKey: "idem-inventory", Status: devicev1.CommandDispatched, Payload: json.RawMessage(`{}`),
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	if err := store.CreateCommand(context.Background(), inventoryCommand); err != nil {
		t.Fatalf("CreateCommand(inventory) error = %v", err)
	}
	if err := store.UpdateCommandStatus(context.Background(), endpoint.ID, inventoryCommand.ID, devicev1.CommandAccepted, nil, nil, now.Add(time.Second)); err != nil {
		t.Fatalf("accept inventory command: %v", err)
	}
	if err := store.UpdateCommandStatus(context.Background(), endpoint.ID, inventoryCommand.ID, devicev1.CommandRunning, nil, nil, now.Add(2*time.Second)); err != nil {
		t.Fatalf("run inventory command: %v", err)
	}
	inventory := devicev1.DeviceInventory{
		SchemaVersion: devicev1.InventorySchemaVersion,
		CapturedAt:    now.Add(2 * time.Second),
		Storage:       []devicev1.StorageInventory{{ID: "c", Root: `C:\`, TotalBytes: 100, FreeBytes: 25}},
		Runtimes:      []devicev1.RuntimeInventory{{ID: "steam", Name: "Steam", Path: `C:\Steam\steam.exe`}},
	}
	inventoryPayload, err := json.Marshal(inventory)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	if err := store.CompleteCommand(context.Background(), endpoint.ID, devicev1.CommandResult{
		CommandID: inventoryCommand.ID,
		Status:    devicev1.CommandSucceeded,
		Payload:   inventoryPayload,
	}, now.Add(3*time.Second)); err != nil {
		t.Fatalf("CompleteCommand(inventory) error = %v", err)
	}
	storedInventory, err := store.GetInventory(context.Background(), endpoint.ID)
	if err != nil {
		t.Fatalf("GetInventory() error = %v", err)
	}
	if storedInventory == nil || len(storedInventory.Storage) != 1 || storedInventory.Storage[0].FreeBytes != 25 || len(storedInventory.Runtimes) != 1 {
		t.Fatalf("stored inventory = %#v", storedInventory)
	}

	if _, err := database.GetDB().ExecContext(context.Background(), `INSERT INTO canonical_games(id, created_at) VALUES ('game-1', ?)`, now.Unix()); err != nil {
		t.Fatalf("insert canonical game: %v", err)
	}
	if _, err := database.GetDB().ExecContext(context.Background(), `INSERT INTO source_games
		(id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, root_path, status, created_at)
		VALUES ('source-1', ?, 'integration-1', 'game-source-google-drive', 'archive-1', 'Game', 'windows_pc', 'base_game', 'packed', 'Games/Installers', 'found', ?)`, profile.ID, now.Unix()); err != nil {
		t.Fatalf("insert source game: %v", err)
	}
	installRequest := devicev1.ArchiveInstallRequest{
		GameID: "game-1", SourceGameID: "source-1", Title: "Game", ArchiveName: "game.zip", ArchiveFormat: "zip",
		ArchiveSize: 42, DownloadURL: "http://mga.test/api/device-transfers/token", DestinationRoot: `%USERPROFILE%\Games`, DestinationName: "Game",
		DownloadToken: "secret-token",
	}
	installPayload, _ := json.Marshal(installRequest)
	installCommand := devices.Command{
		ID: "command-install", EndpointID: endpoint.ID, ProfileID: profile.ID, Name: devicev1.CapabilityGameInstallArchive,
		SchemaVersion: 1, IdempotencyKey: "idem-install", Status: devicev1.CommandDispatched, Payload: installPayload,
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	if err := store.CreateCommand(context.Background(), installCommand); err != nil {
		t.Fatalf("CreateCommand(install) error = %v", err)
	}
	if err := store.UpdateCommandStatus(context.Background(), endpoint.ID, installCommand.ID, devicev1.CommandAccepted, nil, nil, now.Add(time.Second)); err != nil {
		t.Fatalf("accept install command: %v", err)
	}
	percent := uint8(55)
	stagePercent := uint8(25)
	if err := store.RecordCommandProgress(context.Background(), endpoint.ID, devicev1.CommandProgress{
		CommandID: installCommand.ID, Sequence: 1, Phase: "extracting", Percent: &percent,
		Stage: "install", StagePercent: &stagePercent, Message: "Extracting files",
	}, now.Add(2*time.Second)); err != nil {
		t.Fatalf("RecordCommandProgress() error = %v", err)
	}
	installResult := devicev1.ArchiveInstallResult{
		GameID: "game-1", SourceGameID: "source-1", InstallRoot: `C:\Games`, InstallPath: `C:\Games\Game`,
		ArchiveSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArchiveBytes: 42, InstalledAt: now.Add(3 * time.Second),
		LaunchTarget: "Game/game.exe", LaunchCandidates: []string{"Game/game.exe", "Game/alternate.exe"},
	}
	installResultPayload, _ := json.Marshal(installResult)
	if err := store.CompleteCommand(context.Background(), endpoint.ID, devicev1.CommandResult{
		CommandID: installCommand.ID, Status: devicev1.CommandSucceeded, Payload: installResultPayload,
	}, now.Add(3*time.Second)); err != nil {
		t.Fatalf("CompleteCommand(install) error = %v", err)
	}
	installations, err := store.ListInstallations(context.Background(), endpoint.ID, profile.ID)
	if err != nil || len(installations) != 1 || installations[0].InstallPath != `C:\Games\Game` ||
		installations[0].LaunchTarget != "Game/game.exe" || len(installations[0].LaunchCandidates) != 2 {
		t.Fatalf("ListInstallations() = %+v, error = %v", installations, err)
	}
	if err := store.UpdateInstallationLaunchTarget(context.Background(), endpoint.ID, "game-1", "source-1", profile.ID, "Game/alternate.exe", now.Add(4*time.Second)); err != nil {
		t.Fatalf("UpdateInstallationLaunchTarget() error = %v", err)
	}
	installations, err = store.ListInstallations(context.Background(), endpoint.ID, profile.ID)
	if err != nil || len(installations) != 1 || installations[0].LaunchTarget != "Game/alternate.exe" {
		t.Fatalf("updated installations = %+v, error = %v", installations, err)
	}
	commands, err = store.ListCommands(context.Background(), endpoint.ID, profile.ID, 20)
	if err != nil {
		t.Fatalf("ListCommands(after install) error = %v", err)
	}
	var storedInstallCommand *devices.Command
	for index := range commands {
		if commands[index].ID == installCommand.ID {
			storedInstallCommand = &commands[index]
			break
		}
	}
	if storedInstallCommand == nil || storedInstallCommand.ProgressPercent == nil || *storedInstallCommand.ProgressPercent != 55 ||
		storedInstallCommand.ProgressPhase != "extracting" || storedInstallCommand.ProgressStage != "install" ||
		storedInstallCommand.ProgressStagePercent == nil || *storedInstallCommand.ProgressStagePercent != 25 {
		t.Fatalf("install command progress = %+v", storedInstallCommand)
	}
}
