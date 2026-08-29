package db

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/legacyretirement"
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

// The accepted retirement plan requires runtime, save-domain, and prepared-copy
// recovery evidence alongside endpoint/install/storefront evidence, and forbids
// reusable authentication material in the human-readable export.
func TestLegacyRetirementReportCoversRuntimeSaveAndPreparedCopyEvidence(t *testing.T) {
	database := NewSQLiteDatabaseWithMigrationOptions(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "retirement-evidence.sqlite")}, core.MigrationOptions{BackupBeforeMigrate: false})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).Unix()
	// Build the persisted inventory JSON from values so the Windows paths under
	// test are escaped exactly the way the retired client wrote them.
	runtimesJSON, err := json.Marshal([]map[string]any{{
		"id": "retroarch", "name": "RetroArch", "version": "1.16",
		"path": `D:\Emulators\RetroArch`, "core_probe_state": "ready",
	}})
	if err != nil {
		t.Fatal(err)
	}
	preparedJSON, err := json.Marshal([]map[string]any{{
		"local_prepared_copy_id": "prepared-1", "game_id": "game-1", "source_game_id": "source-1",
		"title": "Game", "prepared_path": `D:\Prepared\Game`, "file_count": 12,
		"total_bytes": 2048, "prepared_at": "2027-01-15T10:00:00Z",
	}})
	if err != nil {
		t.Fatal(err)
	}
	runtimes, prepared := string(runtimesJSON), string(preparedJSON)
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
		{`INSERT INTO device_inventories(endpoint_id,schema_version,captured_at,storage_json,runtimes_json,updated_at,prepared_copies_json) VALUES(?,?,?,?,?,?,?)`, []any{"endpoint-1", 1, now, "[]", runtimes, now, prepared}},
		{`INSERT INTO device_emulator_preferences(endpoint_id,platform,emulator_id,updated_at) VALUES(?,?,?,?)`, []any{"endpoint-1", "snes", "retroarch", now}},
		{`INSERT INTO device_emulator_core_preferences(endpoint_id,platform,emulator_id,core_id,updated_at) VALUES(?,?,?,?,?)`, []any{"endpoint-1", "snes", "retroarch", "snes9x", now}},
		{`INSERT INTO device_save_domain_links(endpoint_id,game_id,source_game_id,route_kind,emulator_id,local_save_domain_id,adapter_id,authority_state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, []any{"endpoint-1", "game-1", "source-1", "emulator", "retroarch", "domain-1", "adapter-1", "owned_here", now, now}},
	}
	for _, statement := range statements {
		if _, err := database.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("%s: %v", statement.query, err)
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

	// Every classified table that carries owner-recovery evidence must appear
	// as a detail section, not only as a row count.
	if len(report.EmulatorPreferences) != 1 || report.EmulatorPreferences[0].EmulatorID != "retroarch" {
		t.Fatalf("emulator preferences = %+v", report.EmulatorPreferences)
	}
	if len(report.EmulatorCorePreferences) != 1 || report.EmulatorCorePreferences[0].CoreID != "snes9x" {
		t.Fatalf("emulator core preferences = %+v", report.EmulatorCorePreferences)
	}
	if len(report.SaveDomainLinks) != 1 || report.SaveDomainLinks[0].LocalSaveDomainID != "domain-1" {
		t.Fatalf("save domain links = %+v", report.SaveDomainLinks)
	}
	if len(report.Runtimes) != 1 || report.Runtimes[0].Path != `D:\Emulators\RetroArch` {
		t.Fatalf("runtimes = %+v", report.Runtimes)
	}
	if len(report.PreparedCopies) != 1 || report.PreparedCopies[0].PreparedPath != `D:\Prepared\Game` || report.PreparedCopies[0].FileCount != 12 {
		t.Fatalf("prepared copies = %+v", report.PreparedCopies)
	}
	if len(report.ExcludedSensitiveMaterial) == 0 {
		t.Fatal("report must document the material it deliberately excludes")
	}

	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "public-secret-key") {
		t.Fatalf("report leaked endpoint key material: %s", encoded)
	}
}

// Corrupt legacy inventory JSON must fail the export loudly rather than
// silently dropping recovery evidence an owner may still need.
func TestLegacyRetirementReportFailsOnUnreadableInventoryEvidence(t *testing.T) {
	if _, err := legacyretirement.DecodeInventory("endpoint-1", time.Unix(1_800_000_000, 0).UTC(), "{not json", "[]"); err == nil {
		t.Fatal("expected malformed legacy runtime inventory to fail the export")
	}
}
