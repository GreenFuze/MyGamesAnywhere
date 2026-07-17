package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestInstallPreferenceRepositoryPersistsAndClearsBothScopes(t *testing.T) {
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
		`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES ('profile-prefs','Player','admin_player',?,?)`,
		`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, execution_mode, client_version, protocol_version, capabilities_json, status, created_at, updated_at) VALUES ('endpoint-prefs','instance-prefs','key','PC','pc','user','windows','amd64','standard','dev',1,'[]','offline',?,?)`,
	} {
		if _, err := database.GetDB().Exec(statement, now.Unix(), now.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewInstallPreferenceRepository(database)
	ctx := context.Background()
	if err := repository.SetProfileRoot(ctx, "profile-prefs", `%USERPROFILE%\Profile Games`, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetEndpointRoot(ctx, "endpoint-prefs", `C:\Games`, "profile-prefs", now); err != nil {
		t.Fatal(err)
	}
	profileRoot, err := repository.GetProfileRoot(ctx, "profile-prefs")
	if err != nil || profileRoot != `%USERPROFILE%\Profile Games` {
		t.Fatalf("profile root = %q, error = %v", profileRoot, err)
	}
	endpointRoot, err := repository.GetEndpointRoot(ctx, "endpoint-prefs")
	if err != nil || endpointRoot != `C:\Games` {
		t.Fatalf("endpoint root = %q, error = %v", endpointRoot, err)
	}
	if err := repository.SetProfileRoot(ctx, "profile-prefs", "", now); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetEndpointRoot(ctx, "endpoint-prefs", "", "profile-prefs", now); err != nil {
		t.Fatal(err)
	}
	profileRoot, _ = repository.GetProfileRoot(ctx, "profile-prefs")
	endpointRoot, _ = repository.GetEndpointRoot(ctx, "endpoint-prefs")
	if profileRoot != "" || endpointRoot != "" {
		t.Fatalf("cleared roots = %q, %q", profileRoot, endpointRoot)
	}
}
