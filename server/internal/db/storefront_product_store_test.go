package db

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
)

func TestStorefrontObservationIsRequestBoundedProfileScopedAndNotGlobalInventory(t *testing.T) {
	database := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "storefront.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	profiles := NewProfileRepository(database)
	for _, id := range []string{"profile-a", "profile-b"} {
		if err := profiles.Create(ctx, &core.Profile{ID: id, DisplayName: id, Role: core.ProfileRolePlayer, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	ctxA := core.WithProfile(ctx, &core.Profile{ID: "profile-a"})
	integrations := NewIntegrationRepository(database)
	if err := integrations.Create(ctxA, &core.Integration{ID: "steam-a", PluginID: "steam-source", Label: "Steam", IntegrationType: "source", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetDB().ExecContext(ctx, `INSERT INTO canonical_games(id, created_at) VALUES ('game-a',?)`, now.Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetDB().ExecContext(ctx, `INSERT INTO source_games
		(id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, created_at)
		VALUES ('source-a','profile-a','steam-a','steam-source','12345','Game','windows_pc','base_game','unknown','found',?)`, now.Unix()); err != nil {
		t.Fatal(err)
	}
	store := NewDeviceStore(database)
	challenge := devices.PairingChallenge{ID: "challenge-a", CodeHash: "hash-a", ProfileID: "profile-a", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := store.CreatePairingChallenge(ctx, challenge); err != nil {
		t.Fatal(err)
	}
	endpoint := devices.Endpoint{ID: "endpoint-a", ClientInstanceID: "instance-a", PublicKey: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), DisplayName: "PC / Alice", HostName: "pc", OSUser: "alice", Platform: "windows", Arch: "amd64", ClientVersion: "dev", ProtocolVersion: devicev1.Version, Capabilities: []string{devicev1.CapabilityInventoryRefresh}, Status: devicev1.EndpointReady, CreatedAt: now, UpdatedAt: now}
	if _, err := store.PairEndpoint(ctx, challenge.CodeHash, now, endpoint); err != nil {
		t.Fatal(err)
	}
	candidate := devicev1.StorefrontProductCandidate{GameID: "game-a", SourceGameID: "source-a", Provider: devicev1.StorefrontProviderSteam, ProductID: "12345", Title: "Game"}
	requestPayload, _ := json.Marshal(devicev1.InventoryRefreshRequest{StorefrontCandidates: []devicev1.StorefrontProductCandidate{candidate}})
	command := devices.Command{ID: "command-a", EndpointID: endpoint.ID, ProfileID: "profile-a", Name: devicev1.CapabilityInventoryRefresh, SchemaVersion: 1, IdempotencyKey: "inventory-a", Status: devicev1.CommandDispatched, Payload: requestPayload, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := store.CreateCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateCommandStatus(ctx, endpoint.ID, command.ID, devicev1.CommandAccepted, nil, nil, now); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateCommandStatus(ctx, endpoint.ID, command.ID, devicev1.CommandRunning, nil, nil, now); err != nil {
		t.Fatal(err)
	}
	inventory := devicev1.DeviceInventory{SchemaVersion: devicev1.InventorySchemaVersion, CapturedAt: now, StorefrontProducts: []devicev1.StorefrontProductObservation{{StorefrontProductCandidate: candidate, InstallPath: `C:\Program Files (x86)\Steam\steamapps\common\Game`, ObservedAt: now}}}
	resultPayload, _ := json.Marshal(inventory)
	if err := store.CompleteCommand(ctx, endpoint.ID, devicev1.CommandResult{CommandID: command.ID, Status: devicev1.CommandSucceeded, Payload: resultPayload}, now); err != nil {
		t.Fatal(err)
	}
	owned, err := store.ListStorefrontProducts(ctx, endpoint.ID, "profile-a")
	if err != nil || len(owned) != 1 || owned[0].ProductID != "12345" || owned[0].UseGranted {
		t.Fatalf("owned storefront products = %+v, error = %v", owned, err)
	}
	foreign, err := store.ListStorefrontProducts(ctx, endpoint.ID, "profile-b")
	if err != nil || len(foreign) != 0 {
		t.Fatalf("foreign profile storefront products = %+v, error = %v", foreign, err)
	}
	globalInventory, err := store.GetInventory(ctx, endpoint.ID)
	if err != nil || globalInventory == nil || len(globalInventory.StorefrontProducts) != 0 {
		t.Fatalf("endpoint-global inventory leaked storefront observations: %+v, error = %v", globalInventory, err)
	}
}
