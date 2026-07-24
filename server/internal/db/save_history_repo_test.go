package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/savehistory"
)

func TestSaveHistoryRepositoryRetainsByServerAcceptanceAndScopesProfiles(t *testing.T) {
	database := NewSQLiteDatabaseWithMigrationOptions(
		testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "mga.sqlite")},
		core.MigrationOptions{BackupBeforeMigrate: false},
	)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, id := range []string{"profile-history-a", "profile-history-b"} {
		if _, err := database.GetDB().Exec(`INSERT INTO profiles(id, display_name, role, created_at, updated_at)
			VALUES (?, 'Player', 'player', ?, ?)`, id, now.Unix(), now.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewSaveHistoryRepository(database)
	repository.now = func() time.Time { return now }
	ctx := context.Background()
	policy := savehistory.Policy{
		ProfileID: "profile-history-a", DomainID: "save:domain-a",
		RetainVersions: 2, RetainDays: 30,
	}
	if _, err := repository.UpsertPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	reportedFuture := now.Add(365 * 24 * time.Hour)
	reportedPast := now.Add(-365 * 24 * time.Hour)
	for index, reported := range []*time.Time{&reportedFuture, &reportedPast, &reportedFuture} {
		accepted := now.Add(time.Duration(index) * time.Minute)
		version := savehistory.Version{
			ID: "version-" + string(rune('a'+index)), ProfileID: policy.ProfileID, DomainID: policy.DomainID,
			CanonicalGameID: "game", SourceGameID: "source", Runtime: "scummvm", SlotID: "autosave",
			IntegrationID: "save-store", ManifestHash: repeatHex(byte('a' + index)),
			OriginLabel: "Living room PC", RouteLabel: "ScummVM", AcceptedAt: accepted,
			ReportedAt: reported, PayloadKey: "payload-" + string(rune('a'+index)),
		}
		pruned, err := repository.RecordVersion(ctx, version, policy)
		if err != nil {
			t.Fatal(err)
		}
		if index == 2 && (len(pruned) != 1 || pruned[0].ID != "version-a") {
			t.Fatalf("pruned versions = %+v", pruned)
		}
	}
	versions, err := repository.ListVersions(ctx, policy.ProfileID, policy.DomainID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].ID != "version-c" || versions[1].ID != "version-b" {
		t.Fatalf("server acceptance order = %+v", versions)
	}
	foreign, err := repository.ListVersions(ctx, "profile-history-b", policy.DomainID)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("foreign profile history = %+v, %v", foreign, err)
	}
	if version, err := repository.GetVersion(ctx, "profile-history-b", "version-c"); err != nil || version != nil {
		t.Fatalf("foreign version lookup = %+v, %v", version, err)
	}
	policy.RetainVersions = 1
	pruned, err := repository.UpsertPolicy(ctx, policy)
	if err != nil || len(pruned) != 1 || pruned[0].ID != "version-b" {
		t.Fatalf("policy reduction pruned = %+v, %v", pruned, err)
	}
	versions, err = repository.ListVersions(ctx, policy.ProfileID, policy.DomainID)
	if err != nil || len(versions) != 1 || versions[0].ID != "version-c" {
		t.Fatalf("history after policy reduction = %+v, %v", versions, err)
	}
}

func repeatHex(value byte) string {
	result := make([]byte, 64)
	for index := range result {
		result[index] = value
	}
	return string(result)
}
