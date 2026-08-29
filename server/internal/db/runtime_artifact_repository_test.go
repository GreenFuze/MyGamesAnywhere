package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/runtimeartifact"
)

func TestRuntimeArtifactRepositoryRoundTrip(t *testing.T) {
	database := NewSQLiteDatabaseWithMigrationOptions(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "artifacts.sqlite")}, core.MigrationOptions{BackupBeforeMigrate: false})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	repository := NewRuntimeArtifactRepository(database)
	artifact := runtimeartifact.Artifact{ID: "retroarch-1", PackageID: "retroarch", DisplayName: "RetroArch", Category: runtimeartifact.CategoryEmulator, Version: "1.0", Channel: "stable", OS: "windows", Architecture: "amd64", Compatibility: []byte(`{"platforms":["snes"]}`), LicenseSPDX: "GPL-3.0-only", UpstreamURL: "https://example.test/retroarch.zip", AcquisitionMode: runtimeartifact.AcquisitionCached, Redistributable: true, ComplianceState: runtimeartifact.ComplianceApproved, SHA256: strings.Repeat("a", 64), SizeBytes: 42}
	saved, err := repository.Upsert(context.Background(), artifact)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != artifact.ID || saved.SHA256 != artifact.SHA256 || !saved.Redistributable {
		t.Fatalf("saved artifact = %+v", saved)
	}
	items, err := repository.List(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("list = %+v, %v", items, err)
	}
}
