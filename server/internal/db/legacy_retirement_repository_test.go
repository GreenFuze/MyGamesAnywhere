package db

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestLegacyRetirementReportPreservesInstallEvidenceWithoutEndpointKeys(t *testing.T) {
	database := NewSQLiteDatabaseWithMigrationOptions(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "retirement.sqlite")}, core.MigrationOptions{BackupBeforeMigrate: false})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).Unix()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO profiles(id,display_name,role,created_at,updated_at) VALUES(?,?,?,?,?)`, []any{"profile-1", "Admin", string(core.ProfileRoleAdminPlayer), now, now}},
		{`INSERT INTO integrations(id,profile_id,plugin_id,label,integration_type,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, []any{"integration-1", "profile-1", "test", "Test", "source", now, now}},
		{`INSERT INTO canonical_games(id,created_at) VALUES(?,?)`, []any{"game-1", now}},
		{`INSERT INTO source_games(id,profile_id,integration_id,plugin_id,external_id,raw_title,platform,kind,group_kind,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, []any{"source-1", "profile-1", "integration-1", "test", "external-1", "Game", "windows", "base_game", "unknown", "found", now}},
		{`INSERT INTO device_endpoints(id,client_instance_id,public_key,display_name,host_name,os_user,platform,arch,client_version,protocol_version,capabilities_json,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, []any{"endpoint-1", "instance-1", "public-secret-key", "Old PC", "host", "user", "windows", "amd64", "0.1", 1, "{}", "offline", now, now}},
		{`INSERT INTO device_grants(endpoint_id,profile_id,access_level,created_at,updated_at) VALUES(?,?,?,?,?)`, []any{"endpoint-1", "profile-1", "owner", now, now}},
		{`INSERT INTO device_game_installations(endpoint_id,game_id,source_game_id,profile_id,install_root,install_path,archive_sha256,archive_bytes,installed_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, []any{"endpoint-1", "game-1", "source-1", "profile-1", `D:\Games`, `D:\Games\Game`, "digest", 42, now, now}},
		{`INSERT INTO device_install_preferences(endpoint_id,install_root_template,updated_at) VALUES(?,?,?)`, []any{"endpoint-1", `D:\Games\{title}`, now}},
		{`INSERT INTO device_storefront_products(endpoint_id,profile_id,game_id,source_game_id,provider,product_id,title,install_path,installed,observed_at,use_granted) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, []any{"endpoint-1", "profile-1", "game-1", "source-1", "steam", "10", "Game", `D:\Steam\Game`, 1, now, 0}},
	}
	for _, statement := range statements {
		if _, err := database.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := NewLegacyRetirementRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	report, err := repository.BuildReport(context.Background(), "profile-1", time.Unix(now, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Installations) != 1 || report.Installations[0].InstallPath != `D:\Games\Game` || len(report.Storefront) != 1 {
		t.Fatalf("report = %+v", report)
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "public-secret-key") || strings.Contains(string(encoded), "os_user") {
		t.Fatalf("report leaked endpoint authority: %s", encoded)
	}
}
