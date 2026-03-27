package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/google/uuid"
)

func TestEnsureSchemaBackfillsCanonicalGames(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestGameStore(t)

	if _, err := db.GetDB().ExecContext(ctx, `DELETE FROM canonical_games`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDB().ExecContext(ctx, `DELETE FROM canonical_source_games_link`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDB().ExecContext(ctx, `DELETE FROM source_games`); err != nil {
		t.Fatal(err)
	}

	if _, err := db.GetDB().ExecContext(ctx, `INSERT INTO source_games
		(id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"scan:legacy-source", "integration-1", "game-source-steam", "source-1",
		"Legacy Game", "windows_pc", "base_game", "self_contained", "found", 1700000000,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDB().ExecContext(ctx, `INSERT INTO canonical_source_games_link (canonical_id, source_game_id) VALUES (?, ?)`,
		"legacy-canonical", "scan:legacy-source",
	); err != nil {
		t.Fatal(err)
	}

	if err := db.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	var createdAt int64
	if err := db.GetDB().QueryRowContext(ctx, `SELECT created_at FROM canonical_games WHERE id = ?`, "legacy-canonical").Scan(&createdAt); err != nil {
		t.Fatalf("expected backfilled canonical row: %v", err)
	}
	if createdAt != 1700000000 {
		t.Fatalf("created_at = %d, want %d", createdAt, 1700000000)
	}

	var sourceGameID string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT source_game_id FROM canonical_source_games_link WHERE canonical_id = ?`, "legacy-canonical").Scan(&sourceGameID); err != nil {
		t.Fatalf("expected canonical link to remain: %v", err)
	}
	if sourceGameID != "scan:legacy-source" {
		t.Fatalf("source_game_id = %q, want %q", sourceGameID, "scan:legacy-source")
	}
}

func TestEnsureSchemaMigratesLegacyScanCanonicalIDsToUUIDs(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	if _, err := db.GetDB().ExecContext(ctx, `DELETE FROM canonical_games`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDB().ExecContext(ctx, `DELETE FROM canonical_source_games_link`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDB().ExecContext(ctx, `DELETE FROM source_games`); err != nil {
		t.Fatal(err)
	}

	if _, err := db.GetDB().ExecContext(ctx, `INSERT INTO source_games
		(id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"scan:legacy-source", "integration-1", "game-source-steam", "source-1",
		"Legacy Game", "windows_pc", "base_game", "self_contained", "found", 1700000000,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDB().ExecContext(ctx, `INSERT INTO canonical_source_games_link (canonical_id, source_game_id) VALUES (?, ?)`,
		"scan:legacy-source", "scan:legacy-source",
	); err != nil {
		t.Fatal(err)
	}

	if err := db.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	var migratedID string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id = ?`, "scan:legacy-source").Scan(&migratedID); err != nil {
		t.Fatalf("expected migrated canonical link: %v", err)
	}
	if strings.HasPrefix(migratedID, "scan:") {
		t.Fatalf("migrated canonical id = %q, want UUID", migratedID)
	}
	if _, err := uuid.Parse(migratedID); err != nil {
		t.Fatalf("migrated canonical id = %q, want valid UUID: %v", migratedID, err)
	}

	var legacyCount int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM canonical_games WHERE id = ?`, "scan:legacy-source").Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 {
		t.Fatalf("legacy canonical id row still present, count = %d", legacyCount)
	}

	game, err := store.GetCanonicalGameByID(ctx, migratedID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil {
		t.Fatal("expected migrated canonical id to resolve")
	}
	if game.ID != migratedID {
		t.Fatalf("game.ID = %q, want migrated canonical id %q", game.ID, migratedID)
	}
}

func TestStableCanonicalIDSurvivesMerge(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	persistBatch(t, ctx, store, makeTestBatch("integration-1", "scan:source-a", "source-a", "Alpha", "match-alpha"))
	persistBatch(t, ctx, store, makeTestBatch("integration-2", "scan:source-b", "source-b", "Bravo", "match-bravo"))

	canonicalA := canonicalIDForSource(t, ctx, db, "scan:source-a")
	canonicalB := canonicalIDForSource(t, ctx, db, "scan:source-b")
	if canonicalA == canonicalB {
		t.Fatalf("expected separate canonical ids before merge, both were %q", canonicalA)
	}
	if _, err := db.GetDB().ExecContext(ctx, `UPDATE canonical_games SET created_at = ? WHERE id = ?`, 100, canonicalA); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDB().ExecContext(ctx, `UPDATE canonical_games SET created_at = ? WHERE id = ?`, 200, canonicalB); err != nil {
		t.Fatal(err)
	}

	persistBatch(t, ctx, store, makeTestBatch("integration-2", "scan:source-b", "source-b", "Bravo", "match-alpha"))

	mergedA := canonicalIDForSource(t, ctx, db, "scan:source-a")
	mergedB := canonicalIDForSource(t, ctx, db, "scan:source-b")
	if mergedA != mergedB {
		t.Fatalf("expected merged canonical ids to match, got %q and %q", mergedA, mergedB)
	}
	if mergedA != canonicalA {
		t.Fatalf("merged canonical id = %q, want original surviving id %q", mergedA, canonicalA)
	}
}

func TestStableCanonicalIDSurvivesSplit(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	persistBatch(t, ctx, store, makeTestBatch("integration-1", "scan:source-a", "source-a", "Alpha", "match-alpha"))
	persistBatch(t, ctx, store, makeTestBatch("integration-2", "scan:source-b", "source-b", "Bravo", "match-alpha"))

	mergedCanonical := canonicalIDForSource(t, ctx, db, "scan:source-a")
	if mergedCanonical != canonicalIDForSource(t, ctx, db, "scan:source-b") {
		t.Fatalf("expected initial merged canonical id")
	}

	persistBatch(t, ctx, store, makeTestBatch("integration-2", "scan:source-b", "source-b", "Bravo", "match-bravo"))

	afterSplitA := canonicalIDForSource(t, ctx, db, "scan:source-a")
	afterSplitB := canonicalIDForSource(t, ctx, db, "scan:source-b")
	if afterSplitA != mergedCanonical {
		t.Fatalf("split canonical for source-a = %q, want original %q", afterSplitA, mergedCanonical)
	}
	if afterSplitB == mergedCanonical {
		t.Fatalf("expected source-b to receive a new canonical id after split")
	}
}

func TestGetCanonicalGameByIDDoesNotResolveLegacySourceID(t *testing.T) {
	ctx := context.Background()
	_, store := newTestGameStore(t)

	persistBatch(t, ctx, store, makeTestBatch("integration-1", "scan:source-a", "source-a", "Alpha", "match-alpha"))

	game, err := store.GetCanonicalGameByID(ctx, "scan:source-a")
	if err != nil {
		t.Fatal(err)
	}
	if game != nil {
		t.Fatalf("expected legacy source id lookup to return nil, got %q", game.ID)
	}
}

func TestCanonicalGameIncludesCachedAchievementSummary(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	persistBatch(t, ctx, store, makeTestBatch("integration-1", "scan:source-a", "source-a", "Alpha", "match-alpha"))

	canonicalID := canonicalIDForSource(t, ctx, db, "scan:source-a")
	fetchedAt := time.Unix(1710000000, 0).UTC()
	if err := store.CacheAchievements(ctx, "scan:source-a", &core.AchievementSet{
		Source:         "game-source-steam",
		ExternalGameID: "220",
		TotalCount:     3,
		UnlockedCount:  1,
		TotalPoints:    30,
		EarnedPoints:   10,
		Achievements: []core.Achievement{
			{ExternalID: "a1", Title: "One", Points: 10, Unlocked: true, UnlockedAt: fetchedAt},
			{ExternalID: "a2", Title: "Two", Points: 10},
			{ExternalID: "a3", Title: "Three", Points: 10},
		},
		FetchedAt: fetchedAt,
	}); err != nil {
		t.Fatal(err)
	}

	game, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil || game.AchievementSummary == nil {
		t.Fatal("expected canonical game to include cached achievement summary")
	}
	if game.AchievementSummary.SourceCount != 1 {
		t.Fatalf("source_count = %d, want 1", game.AchievementSummary.SourceCount)
	}
	if game.AchievementSummary.TotalCount != 3 {
		t.Fatalf("total_count = %d, want 3", game.AchievementSummary.TotalCount)
	}
	if game.AchievementSummary.UnlockedCount != 1 {
		t.Fatalf("unlocked_count = %d, want 1", game.AchievementSummary.UnlockedCount)
	}
	if game.AchievementSummary.TotalPoints != 30 {
		t.Fatalf("total_points = %d, want 30", game.AchievementSummary.TotalPoints)
	}
	if game.AchievementSummary.EarnedPoints != 10 {
		t.Fatalf("earned_points = %d, want 10", game.AchievementSummary.EarnedPoints)
	}
}

func newTestGameStore(t *testing.T) (*sqliteDatabase, *gameStore) {
	t.Helper()

	rawDB := NewSQLiteDatabase(testLogger{}, testDBConfig{})
	db, ok := rawDB.(*sqliteDatabase)
	if !ok {
		t.Fatal("expected sqlite database implementation")
	}
	if err := db.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := db.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	store := NewGameStore(db, testLogger{}).(*gameStore)
	return db, store
}

func makeTestBatch(
	integrationID string,
	sourceGameID string,
	sourceExternalID string,
	title string,
	matchExternalID string,
) *core.ScanBatch {
	sourceGame := &core.SourceGame{
		ID:            sourceGameID,
		IntegrationID: integrationID,
		PluginID:      "game-source-steam",
		ExternalID:    sourceExternalID,
		RawTitle:      title,
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		Status:        "found",
	}

	return &core.ScanBatch{
		IntegrationID: integrationID,
		SourceGames:   []*core.SourceGame{sourceGame},
		ResolverMatches: map[string][]core.ResolverMatch{
			sourceGameID: {{
				PluginID:   "metadata-steam",
				Title:      title,
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: matchExternalID,
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}
}

func persistBatch(t *testing.T, ctx context.Context, store *gameStore, batch *core.ScanBatch) {
	t.Helper()
	if err := store.PersistScanResults(ctx, batch); err != nil {
		t.Fatal(err)
	}
}

func canonicalIDForSource(t *testing.T, ctx context.Context, db *sqliteDatabase, sourceGameID string) string {
	t.Helper()

	var canonicalID string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id = ?`, sourceGameID).Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}
	return canonicalID
}
