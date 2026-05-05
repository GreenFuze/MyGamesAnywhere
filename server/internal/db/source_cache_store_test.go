package db

import (
	"context"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestSourceCacheStoreScopesEntriesAndJobsByProfileContext(t *testing.T) {
	ctx := context.Background()
	database, _ := newTestGameStore(t)
	store := NewSourceCacheStore(database)

	now := time.Now().Unix()
	if _, err := database.GetDB().ExecContext(ctx, `INSERT INTO profiles
		(id, display_name, avatar_key, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?)`,
		"profile-1", "Profile One", "", string(core.ProfileRoleAdminPlayer), now, now,
		"profile-2", "Profile Two", "", string(core.ProfileRolePlayer), now, now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetDB().ExecContext(ctx, `INSERT INTO source_games
		(id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"source-1", "profile-1", "integration-1", "game-source-drive", "external-1", "Game One", "gba", "base_game", "self_contained", "found", now,
		"source-2", "profile-2", "integration-2", "game-source-drive", "external-2", "Game Two", "gba", "base_game", "self_contained", "found", now,
	); err != nil {
		t.Fatal(err)
	}

	if err := store.UpsertEntry(ctx, &core.SourceCacheEntry{
		ID:            "entry-1",
		CacheKey:      "cache-1",
		SourceGameID:  "source-1",
		SourceTitle:   "Game One",
		IntegrationID: "integration-1",
		PluginID:      "game-source-drive",
		Profile:       core.BrowserProfileEmulatorJS,
		Mode:          string(core.SourceDeliveryModeMaterialized),
		Status:        "ready",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntry(ctx, &core.SourceCacheEntry{
		ID:            "entry-2",
		CacheKey:      "cache-2",
		SourceGameID:  "source-2",
		SourceTitle:   "Game Two",
		IntegrationID: "integration-2",
		PluginID:      "game-source-drive",
		Profile:       core.BrowserProfileEmulatorJS,
		Mode:          string(core.SourceDeliveryModeMaterialized),
		Status:        "ready",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateJob(ctx, &core.SourceCacheJobStatus{
		JobID:         "job-1",
		CacheKey:      "cache-1",
		SourceGameID:  "source-1",
		SourceTitle:   "Game One",
		IntegrationID: "integration-1",
		PluginID:      "game-source-drive",
		Profile:       core.BrowserProfileEmulatorJS,
		Status:        "running",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateJob(ctx, &core.SourceCacheJobStatus{
		JobID:         "job-2",
		CacheKey:      "cache-2",
		SourceGameID:  "source-2",
		SourceTitle:   "Game Two",
		IntegrationID: "integration-2",
		PluginID:      "game-source-drive",
		Profile:       core.BrowserProfileEmulatorJS,
		Status:        "running",
	}); err != nil {
		t.Fatal(err)
	}

	profileOneCtx := core.WithProfile(ctx, &core.Profile{ID: "profile-1", Role: core.ProfileRoleAdminPlayer})
	entries, err := store.ListEntries(profileOneCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "entry-1" {
		t.Fatalf("profile one entries = %+v, want only entry-1", entries)
	}
	jobs, err := store.ListJobs(profileOneCtx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].JobID != "job-1" {
		t.Fatalf("profile one jobs = %+v, want only job-1", jobs)
	}
	otherJob, err := store.GetJob(profileOneCtx, "job-2")
	if err != nil {
		t.Fatal(err)
	}
	if otherJob != nil {
		t.Fatalf("profile one got profile two job: %+v", otherJob)
	}
	activeOtherJob, err := store.FindActiveJobByCacheKey(profileOneCtx, "cache-2")
	if err != nil {
		t.Fatal(err)
	}
	if activeOtherJob != nil {
		t.Fatalf("profile one found profile two active job: %+v", activeOtherJob)
	}

	if err := store.DeleteEntry(profileOneCtx, "entry-2"); err != nil {
		t.Fatal(err)
	}
	globalEntries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(globalEntries) != 2 {
		t.Fatalf("global entries after cross-profile delete = %d, want 2", len(globalEntries))
	}
	if err := store.ClearEntries(profileOneCtx); err != nil {
		t.Fatal(err)
	}
	globalEntries, err = store.ListEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(globalEntries) != 1 || globalEntries[0].ID != "entry-2" {
		t.Fatalf("global entries after profile clear = %+v, want only entry-2", globalEntries)
	}
}
