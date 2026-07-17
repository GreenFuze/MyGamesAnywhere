package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestEmulatorPreferenceRepositoryPersistsPerPlatformAndClears(t *testing.T) {
	database := NewSQLiteDatabaseWithMigrationOptions(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "mga.sqlite")}, core.MigrationOptions{BackupBeforeMigrate: false})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, statement := range []string{
		`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES ('profile-emulators','Player','admin_player',?,?)`,
		`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, execution_mode, client_version, protocol_version, capabilities_json, status, created_at, updated_at) VALUES ('endpoint-emulators','instance-emulators','key','PC','pc','user','windows','amd64','standard','dev',1,'[]','offline',?,?)`,
	} {
		if _, err := database.GetDB().Exec(statement, now.Unix(), now.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewEmulatorPreferenceRepository(database)
	ctx := context.Background()
	if err := repository.SetDefault(ctx, "endpoint-emulators", core.PlatformPS1, "duckstation", "profile-emulators", now); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetDefault(ctx, "endpoint-emulators", core.PlatformPS2, "pcsx2", "profile-emulators", now); err != nil {
		t.Fatal(err)
	}
	defaults, err := repository.ListDefaults(ctx, "endpoint-emulators")
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults) != 2 || defaults[core.PlatformPS1] != "duckstation" || defaults[core.PlatformPS2] != "pcsx2" {
		t.Fatalf("defaults = %#v", defaults)
	}
	if err := repository.SetDefault(ctx, "endpoint-emulators", core.PlatformPS1, "", "profile-emulators", now); err != nil {
		t.Fatal(err)
	}
	defaults, _ = repository.ListDefaults(ctx, "endpoint-emulators")
	if _, exists := defaults[core.PlatformPS1]; exists || defaults[core.PlatformPS2] != "pcsx2" {
		t.Fatalf("cleared defaults = %#v", defaults)
	}
	if err := repository.SetCoreDefault(ctx, "endpoint-emulators", core.PlatformPS1, "retroarch", "swanstation", "profile-emulators", now); err != nil {
		t.Fatal(err)
	}
	cores, err := repository.ListCoreDefaults(ctx, "endpoint-emulators")
	if err != nil || cores[core.PlatformPS1]["retroarch"] != "swanstation" {
		t.Fatalf("core defaults=%#v err=%v", cores, err)
	}
	if err := repository.SetCoreDefault(ctx, "endpoint-emulators", core.PlatformPS1, "retroarch", "", "profile-emulators", now); err != nil {
		t.Fatal(err)
	}
	cores, _ = repository.ListCoreDefaults(ctx, "endpoint-emulators")
	if cores[core.PlatformPS1]["retroarch"] != "" {
		t.Fatalf("cleared core defaults = %#v", cores)
	}
}
