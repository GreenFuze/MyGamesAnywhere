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
}
