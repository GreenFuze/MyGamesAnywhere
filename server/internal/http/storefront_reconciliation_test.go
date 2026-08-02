package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	dbpkg "github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
)

func TestStorefrontReconciliationUsesProfileOwnedExactCandidatesWithoutDuplicateDispatch(t *testing.T) {
	database := dbpkg.NewSQLiteDatabase(noopLogger{}, validationDBConfig{path: filepath.Join(t.TempDir(), "storefront-reconciliation.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	profile := &core.Profile{ID: "profile-1", DisplayName: "Player", Role: core.ProfileRolePlayer, CreatedAt: now, UpdatedAt: now}
	profiles := dbpkg.NewProfileRepository(database)
	if err := profiles.Create(ctx, profile); err != nil {
		t.Fatal(err)
	}
	profileCtx := core.WithProfile(ctx, profile)
	integrations := dbpkg.NewIntegrationRepository(database)
	if err := integrations.Create(profileCtx, &core.Integration{ID: "steam-1", PluginID: steamGameSourcePluginID, Label: "Steam", IntegrationType: "source", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO canonical_games (id, created_at) VALUES ('game-1',?)`, []any{now.Unix()}},
		{`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at) VALUES ('source-1','profile-1','steam-1','game-source-steam','12345','Game','windows_pc','base_game','unknown','found','matched',?)`, []any{now.Unix()}},
		{`INSERT INTO canonical_source_games_link (canonical_id, source_game_id) VALUES ('game-1','source-1')`, nil},
		{`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, execution_mode, client_version, protocol_version, capabilities_json, status, created_at, updated_at) VALUES ('endpoint-1','instance-1',?,'PC','pc','user','windows','amd64','standard','dev',1,?,'ready',?,?)`, []any{base64.RawURLEncoding.EncodeToString(make([]byte, 32)), `["inventory.refresh"]`, now.Unix(), now.Unix()}},
		{`INSERT INTO device_grants (endpoint_id, profile_id, access_level, created_at, updated_at) VALUES ('endpoint-1','profile-1','owner',?,?)`, []any{now.Unix(), now.Unix()}},
	} {
		if _, err := database.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	deviceStore := dbpkg.NewDeviceStore(database)
	hub := devices.NewHub()
	transport := &validationTransport{}
	if err := hub.Register("endpoint-1", transport); err != nil {
		t.Fatal(err)
	}
	deviceService, err := devices.NewService(deviceStore, hub)
	if err != nil {
		t.Fatal(err)
	}
	gameStore := dbpkg.NewGameStore(database, noopLogger{})
	service, err := NewStorefrontReconciliationService(deviceService, &backgroundTestProfileRepository{profiles: []*core.Profile{profile}}, gameStore, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	if err := service.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(transport.writes) != 1 {
		t.Fatalf("background storefront command writes = %d", len(transport.writes))
	}
	var envelope devicev1.Envelope
	if err := json.Unmarshal(transport.writes[0], &envelope); err != nil {
		t.Fatal(err)
	}
	command, err := devicev1.DecodePayload[devicev1.CommandRequest](envelope)
	if err != nil {
		t.Fatal(err)
	}
	if command.Name != devicev1.CapabilityInventoryRefresh {
		t.Fatalf("command name = %q", command.Name)
	}
	var request devicev1.InventoryRefreshRequest
	if err := json.Unmarshal(command.Payload, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.StorefrontCandidates) != 1 || request.StorefrontCandidates[0].ProductID != "12345" || request.StorefrontCandidates[0].SourceGameID != "source-1" {
		t.Fatalf("storefront request = %+v", request)
	}
	if err := service.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(transport.writes) != 1 {
		t.Fatal("active background reconciliation was dispatched twice")
	}
}
