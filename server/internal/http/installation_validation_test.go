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

type validationDBConfig struct{ path string }

func (c validationDBConfig) Get(key string) string {
	if key == "DB_PATH" {
		return c.path
	}
	return ""
}
func (validationDBConfig) GetInt(string) int   { return 0 }
func (validationDBConfig) GetBool(string) bool { return false }
func (validationDBConfig) Validate() error     { return nil }

type validationTransport struct{ writes [][]byte }

func (t *validationTransport) Write(_ context.Context, data []byte) error {
	t.writes = append(t.writes, append([]byte(nil), data...))
	return nil
}
func (*validationTransport) Close() error { return nil }

func TestInstallationValidationSchedulerAndManualUseSameTypedCommand(t *testing.T) {
	database := dbpkg.NewSQLiteDatabase(noopLogger{}, validationDBConfig{path: filepath.Join(t.TempDir(), "validation.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	profile := &core.Profile{ID: "profile-1", DisplayName: "Player", Role: core.ProfileRoleAdminPlayer, CreatedAt: now, UpdatedAt: now}
	profiles := dbpkg.NewProfileRepository(database)
	if err := profiles.Create(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO canonical_games (id, created_at) VALUES ('game-1',?)`, []any{now.Unix()}},
		{`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at) VALUES ('source-1','profile-1','i','p','e','Game','windows_pc','base_game','packed','found','matched',?)`, []any{now.Unix()}},
		{`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, execution_mode, client_version, protocol_version, capabilities_json, status, created_at, updated_at) VALUES ('endpoint-1','instance-1',?,'PC','pc','user','windows','amd64','standard','dev',1,?,'ready',?,?)`, []any{base64.RawURLEncoding.EncodeToString(make([]byte, 32)), `["game.validate_installations"]`, now.Unix(), now.Unix()}},
		{`INSERT INTO device_grants (endpoint_id, profile_id, access_level, created_at, updated_at) VALUES ('endpoint-1','profile-1','owner',?,?)`, []any{now.Unix(), now.Unix()}},
		{`INSERT INTO device_game_installations (endpoint_id, game_id, source_game_id, profile_id, install_root, install_path, archive_sha256, archive_bytes, installed_at, updated_at, launch_target, launch_candidates_json, install_kind, install_state) VALUES ('endpoint-1','game-1','source-1','profile-1','C:\Games','C:\Games\Game','hash',1,?,?,'Game.exe','["Game.exe"]','managed_archive','installed')`, []any{now.Unix(), now.Unix()}},
	} {
		if _, err := database.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	store := dbpkg.NewDeviceStore(database)
	hub := devices.NewHub()
	transport := &validationTransport{}
	if err := hub.Register("endpoint-1", transport); err != nil {
		t.Fatal(err)
	}
	deviceService, err := devices.NewService(store, hub)
	if err != nil {
		t.Fatal(err)
	}
	settings := &backgroundTestSettingRepository{settings: make(map[string]*core.Setting)}
	scheduler, err := NewInstallationValidationService(deviceService, &backgroundTestProfileRepository{profiles: []*core.Profile{profile}}, settings, noopLogger{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	scheduler.now = func() time.Time { return now }
	if err := scheduler.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(transport.writes) != 0 {
		t.Fatal("validation ran before initial delay")
	}
	now = now.Add(installationValidationInitialDelay)
	if err := scheduler.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(transport.writes) != 1 {
		t.Fatalf("background command writes = %d", len(transport.writes))
	}
	var envelope devicev1.Envelope
	if err := json.Unmarshal(transport.writes[0], &envelope); err != nil {
		t.Fatal(err)
	}
	request, err := devicev1.DecodePayload[devicev1.CommandRequest](envelope)
	if err != nil {
		t.Fatal(err)
	}
	var validation devicev1.InstallationValidationRequest
	if err := json.Unmarshal(request.Payload, &validation); err != nil {
		t.Fatal(err)
	}
	if validation.Trigger != "background" || len(validation.Items) != 1 || validation.Items[0].GameID != "game-1" {
		t.Fatalf("background request = %#v", validation)
	}
	profileCtx := core.WithProfile(context.Background(), profile)
	status, err := scheduler.Status(profileCtx)
	if err != nil || len(status.Devices) != 1 || status.Devices[0].State != "running" {
		t.Fatalf("running status = %#v, error = %v", status, err)
	}

	if _, err := scheduler.RunNow(profileCtx, "endpoint-1", profile.ID); err != nil {
		t.Fatalf("manual joins active command: %v", err)
	}
	if len(transport.writes) != 1 {
		t.Fatal("manual check duplicated active validation")
	}

	if _, err := scheduler.UpdateConfig(profileCtx, InstallationValidationScheduleConfig{Enabled: false, IntervalMinutes: 30}); err != nil {
		t.Fatal(err)
	}
	if settings.settings[profile.ID+":"+installationValidationScheduleSetting] == nil {
		t.Fatal("profile-scoped schedule was not persisted")
	}
	if _, err := scheduler.UpdateConfig(profileCtx, InstallationValidationScheduleConfig{Enabled: true, IntervalMinutes: 1}); err == nil {
		t.Fatal("invalid interval accepted")
	}
}

func TestEligibleInstallationCountProtectsFailedCleanupStates(t *testing.T) {
	installations := []devices.GameInstallation{
		{InstallState: devicev1.InstallStateInstalled}, {InstallState: devicev1.InstallStateMissing}, {InstallState: devicev1.InstallStateNeedsRepair},
		{InstallState: devicev1.InstallStateAttentionRequired}, {InstallState: devicev1.InstallStateCleanupRequired}, {InstallState: devicev1.InstallStateIgnoredFailure},
	}
	if got := eligibleInstallationCount(installations); got != 3 {
		t.Fatalf("eligible count = %d, want 3", got)
	}
}
