package db

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/google/uuid"
)

type warningCaptureLogger struct {
	warnings []string
}

func (l *warningCaptureLogger) Info(string, ...any) {}

func (l *warningCaptureLogger) Error(string, error, ...any) {}

func (l *warningCaptureLogger) Debug(string, ...any) {}

func (l *warningCaptureLogger) Warn(msg string, args ...any) {
	l.warnings = append(l.warnings, fmt.Sprintf("%s %v", msg, args))
}

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

func TestPersistScanResultsReusesExistingRowForSameNaturalKey(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	if _, err := db.GetDB().ExecContext(ctx, `INSERT INTO source_games
		(id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"scan:legacy-source", "integration-1", "game-source-steam", "source-a",
		"Legacy Alpha", "windows_pc", "base_game", "self_contained", "found", 1700000000,
	); err != nil {
		t.Fatal(err)
	}

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:new-source-id",
			IntegrationID: "integration-1",
			PluginID:      "game-source-steam",
			ExternalID:    "source-a",
			RawTitle:      "Alpha",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
			Files: []core.GameFile{{
				Path:     "C:/Games/Alpha/game.exe",
				FileName: "game.exe",
				Role:     core.GameFileRoleRoot,
				FileKind: "exe",
				Size:     4096,
			}},
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:new-source-id": {{
				PluginID:   "metadata-steam",
				Title:      "Alpha",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "match-alpha",
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	var sourceGameID, rawTitle, status string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT id, raw_title, status FROM source_games
		WHERE integration_id=? AND plugin_id=? AND external_id=?`,
		"integration-1", "game-source-steam", "source-a",
	).Scan(&sourceGameID, &rawTitle, &status); err != nil {
		t.Fatal(err)
	}
	if sourceGameID != "scan:legacy-source" {
		t.Fatalf("source game id = %q, want legacy persisted id", sourceGameID)
	}
	if rawTitle != "Alpha" {
		t.Fatalf("raw_title = %q, want updated title", rawTitle)
	}
	if status != "found" {
		t.Fatalf("status = %q, want found", status)
	}

	var count int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM source_games
		WHERE integration_id=? AND plugin_id=? AND external_id=?`,
		"integration-1", "game-source-steam", "source-a",
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("source game row count = %d, want 1", count)
	}

	var fileCount int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM game_files WHERE source_game_id=?`, "scan:legacy-source").Scan(&fileCount); err != nil {
		t.Fatal(err)
	}
	if fileCount != 1 {
		t.Fatalf("file_count = %d, want 1 for reused persisted source id", fileCount)
	}
}

func TestPersistScanResultsDuplicateNaturalKeyInBatchKeepsLastEntryAndWarns(t *testing.T) {
	ctx := context.Background()
	logger := &warningCaptureLogger{}
	_, store := newTestGameStoreWithLogger(t, logger)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{
			{
				ID:            "scan:first-source",
				IntegrationID: "integration-1",
				PluginID:      "game-source-steam",
				ExternalID:    "source-a",
				RawTitle:      "Alpha First",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
				Files: []core.GameFile{{
					Path:     "C:/Games/Alpha/first.exe",
					FileName: "first.exe",
					Role:     core.GameFileRoleRoot,
					FileKind: "exe",
					Size:     100,
				}},
			},
			{
				ID:            "scan:second-source",
				IntegrationID: "integration-1",
				PluginID:      "game-source-steam",
				ExternalID:    "source-a",
				RawTitle:      "Alpha Second",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
				Files: []core.GameFile{{
					Path:     "C:/Games/Alpha/second.exe",
					FileName: "second.exe",
					Role:     core.GameFileRoleRoot,
					FileKind: "exe",
					Size:     200,
				}},
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:first-source": {{
				PluginID:   "metadata-steam",
				Title:      "Alpha First",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "match-first",
			}},
			"scan:second-source": {{
				PluginID:   "metadata-steam",
				Title:      "Alpha Second",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "match-second",
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	var keptTitle string
	if err := store.db.GetDB().QueryRowContext(ctx, `SELECT raw_title FROM source_games WHERE id = ?`, "scan:second-source").Scan(&keptTitle); err != nil {
		t.Fatal(err)
	}
	if keptTitle != "Alpha Second" {
		t.Fatalf("raw_title = %q, want last duplicate title", keptTitle)
	}

	var olderCount int
	if err := store.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM source_games WHERE id = ?`, "scan:first-source").Scan(&olderCount); err != nil {
		t.Fatal(err)
	}
	if olderCount != 0 {
		t.Fatalf("expected overwritten duplicate source id to be absent, count = %d", olderCount)
	}

	if len(logger.warnings) != 1 {
		t.Fatalf("warning count = %d, want 1", len(logger.warnings))
	}
	warning := logger.warnings[0]
	for _, want := range []string{
		"game-source-steam",
		"source-a",
		"scan:first-source",
		"Alpha First",
		"scan:second-source",
		"Alpha Second",
	} {
		if !strings.Contains(warning, want) {
			t.Fatalf("warning %q missing %q", warning, want)
		}
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

func TestGetLibraryStatsIncludesDashboardFields(t *testing.T) {
	ctx := context.Background()
	_, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{
			{
				ID:            "scan:source-one",
				IntegrationID: "integration-1",
				PluginID:      "game-source-steam",
				ExternalID:    "source-one",
				RawTitle:      "Game One",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:source-one": {{
				PluginID:    "metadata-igdb",
				Title:       "Game One",
				Platform:    string(core.PlatformWindowsPC),
				ExternalID:  "match-one",
				Description: "Game One description",
				ReleaseDate: "1998-11-19",
				Genres:      []string{"RPG", "Adventure"},
			}},
		},
		MediaItems: map[string][]core.MediaRef{
			"scan:source-one": {{
				Type: core.MediaTypeCover,
				URL:  "https://example.com/game-one-cover.png",
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-2",
		SourceGames: []*core.SourceGame{
			{
				ID:            "scan:source-two",
				IntegrationID: "integration-2",
				PluginID:      "game-source-gog",
				ExternalID:    "source-two",
				RawTitle:      "Game Two",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:source-two": {{
				PluginID:    "metadata-igdb",
				Title:       "Game Two",
				Platform:    string(core.PlatformWindowsPC),
				ExternalID:  "match-two",
				Description: "   ",
				ReleaseDate: "2004",
				Genres:      []string{"RPG", "Action"},
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.CacheAchievements(ctx, "scan:source-one", &core.AchievementSet{
		Source:         "game-source-steam",
		ExternalGameID: "220",
		TotalCount:     1,
		UnlockedCount:  0,
		FetchedAt:      time.Unix(1710000000, 0).UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	stats, err := store.GetLibraryStats(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if stats.GamesWithMedia != 1 {
		t.Fatalf("games_with_media = %d, want 1", stats.GamesWithMedia)
	}
	if stats.GamesWithDescription != 1 {
		t.Fatalf("games_with_description = %d, want 1", stats.GamesWithDescription)
	}
	if stats.GamesWithAchievements != 1 {
		t.Fatalf("games_with_achievements = %d, want 1", stats.GamesWithAchievements)
	}
	if stats.ByDecade["1990s"] != 1 || stats.ByDecade["2000s"] != 1 {
		t.Fatalf("unexpected by_decade: %+v", stats.ByDecade)
	}
	if stats.TopGenres["RPG"] != 2 || stats.TopGenres["Adventure"] != 1 || stats.TopGenres["Action"] != 1 {
		t.Fatalf("unexpected top_genres: %+v", stats.TopGenres)
	}
	if stats.PercentWithMedia != 50 {
		t.Fatalf("percent_with_media = %v, want 50", stats.PercentWithMedia)
	}
	if stats.PercentWithDescription != 50 {
		t.Fatalf("percent_with_description = %v, want 50", stats.PercentWithDescription)
	}
	if stats.PercentWithAchievements != 50 {
		t.Fatalf("percent_with_achievements = %v, want 50", stats.PercentWithAchievements)
	}
}

func TestListManualReviewCandidatesFiltersToNeedsReview(t *testing.T) {
	ctx := context.Background()
	_, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{
			{
				ID:            "scan:review-no-match",
				IntegrationID: "integration-1",
				PluginID:      "game-source-steam",
				ExternalID:    "review-no-match",
				RawTitle:      "Unidentified Game",
				Platform:      core.PlatformUnknown,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindUnknown,
				Status:        "found",
				Files: []core.GameFile{{
					Path:     "C:/Games/Unidentified/game.exe",
					FileName: "game.exe",
					Role:     core.GameFileRoleRoot,
					FileKind: "exe",
					Size:     1024,
				}},
			},
			{
				ID:            "scan:review-no-title",
				IntegrationID: "integration-1",
				PluginID:      "game-source-steam",
				ExternalID:    "review-no-title",
				RawTitle:      "Match Without Title",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			},
			{
				ID:            "scan:clean",
				IntegrationID: "integration-1",
				PluginID:      "game-source-steam",
				ExternalID:    "clean",
				RawTitle:      "Resolved Game",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:review-no-title": {{
				PluginID:   "metadata-igdb",
				Title:      "",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "match-without-title",
			}},
			"scan:clean": {{
				PluginID:   "metadata-igdb",
				Title:      "Resolved Game",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "clean-match",
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	candidates, err := store.ListManualReviewCandidates(ctx, core.ManualReviewScopeActive, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2", len(candidates))
	}

	reasonsByID := make(map[string][]string, len(candidates))
	for _, candidate := range candidates {
		reasonsByID[candidate.ID] = candidate.ReviewReasons
	}

	if _, ok := reasonsByID["scan:clean"]; ok {
		t.Fatal("expected resolved source game to be excluded from manual review candidates")
	}
	if !containsString(reasonsByID["scan:review-no-match"], "no_metadata_matches") {
		t.Fatalf("review-no-match reasons = %+v, want no_metadata_matches", reasonsByID["scan:review-no-match"])
	}
	if !containsString(reasonsByID["scan:review-no-match"], "unknown_platform") {
		t.Fatalf("review-no-match reasons = %+v, want unknown_platform", reasonsByID["scan:review-no-match"])
	}
	if !containsString(reasonsByID["scan:review-no-match"], "unknown_grouping") {
		t.Fatalf("review-no-match reasons = %+v, want unknown_grouping", reasonsByID["scan:review-no-match"])
	}
	if !containsString(reasonsByID["scan:review-no-title"], "no_resolved_title") {
		t.Fatalf("review-no-title reasons = %+v, want no_resolved_title", reasonsByID["scan:review-no-title"])
	}
}

func TestGetManualReviewCandidateReturnsDirectSourceDetailEvenWhenNotQueued(t *testing.T) {
	ctx := context.Background()
	_, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:clean",
			IntegrationID: "integration-1",
			PluginID:      "game-source-steam",
			ExternalID:    "clean",
			RawTitle:      "Resolved Game",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			RootPath:      "C:/Games/Resolved",
			Status:        "found",
			Files: []core.GameFile{{
				Path:     "C:/Games/Resolved/game.exe",
				FileName: "game.exe",
				Role:     core.GameFileRoleRoot,
				FileKind: "exe",
				Size:     2048,
			}},
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:clean": {{
				PluginID:   "metadata-igdb",
				Title:      "Resolved Game",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "clean-match",
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	candidates, err := store.ListManualReviewCandidates(ctx, core.ManualReviewScopeActive, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("len(candidates) = %d, want 0 for resolved source game", len(candidates))
	}

	candidate, err := store.GetManualReviewCandidate(ctx, "scan:clean")
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil {
		t.Fatal("expected direct manual review candidate detail")
	}
	if candidate.CurrentTitle != "Resolved Game" {
		t.Fatalf("current_title = %q, want %q", candidate.CurrentTitle, "Resolved Game")
	}
	if len(candidate.Files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(candidate.Files))
	}
	if len(candidate.ResolverMatches) != 1 {
		t.Fatalf("len(resolver_matches) = %d, want 1", len(candidate.ResolverMatches))
	}
	if len(candidate.ReviewReasons) != 0 {
		t.Fatalf("review_reasons = %+v, want empty for clean direct candidate", candidate.ReviewReasons)
	}
}

func TestManualReviewCandidatesSupportArchiveScopeAndUnarchive(t *testing.T) {
	ctx := context.Background()
	_, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:archive-me",
			IntegrationID: "integration-1",
			PluginID:      "game-source-steam",
			ExternalID:    "archive-me",
			RawTitle:      "Archive Me",
			Platform:      core.PlatformUnknown,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindUnknown,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	active, err := store.ListManualReviewCandidates(ctx, core.ManualReviewScopeActive, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != "scan:archive-me" {
		t.Fatalf("active candidates = %+v, want archive-me in active scope", active)
	}

	if err := store.SetManualReviewState(ctx, "scan:archive-me", core.ManualReviewStateNotAGame); err != nil {
		t.Fatal(err)
	}

	active, err = store.ListManualReviewCandidates(ctx, core.ManualReviewScopeActive, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("len(active) = %d, want 0 after archive", len(active))
	}

	archive, err := store.ListManualReviewCandidates(ctx, core.ManualReviewScopeArchive, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive) != 1 || archive[0].ID != "scan:archive-me" {
		t.Fatalf("archive candidates = %+v, want archive-me in archive scope", archive)
	}
	if archive[0].ReviewState != core.ManualReviewStateNotAGame {
		t.Fatalf("archive review_state = %q, want %q", archive[0].ReviewState, core.ManualReviewStateNotAGame)
	}

	foundGames, err := store.GetFoundSourceGames(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(foundGames) != 0 {
		t.Fatalf("len(foundGames) = %d, want 0 while archived", len(foundGames))
	}

	if err := store.SetManualReviewState(ctx, "scan:archive-me", core.ManualReviewStatePending); err != nil {
		t.Fatal(err)
	}

	active, err = store.ListManualReviewCandidates(ctx, core.ManualReviewScopeActive, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != "scan:archive-me" {
		t.Fatalf("active candidates after unarchive = %+v, want archive-me", active)
	}
}

func TestSaveManualReviewResultPersistsStickyDecisionAndCanonicalView(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:review-apply",
			IntegrationID: "integration-1",
			PluginID:      "game-source-steam",
			ExternalID:    "review-apply",
			RawTitle:      "Mystery Setup",
			Platform:      core.PlatformUnknown,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindUnknown,
			RootPath:      "C:/Games/Mystery Setup",
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	selection := &core.ManualReviewSelection{
		ProviderIntegrationID: "metadata-1",
		ProviderPluginID:      "metadata-igdb",
		Title:                 "Mystery Game",
		Platform:              string(core.PlatformWindowsPC),
		Kind:                  string(core.GameKindBaseGame),
		ExternalID:            "igdb-1",
		URL:                   "https://example.com/igdb-1",
		ImageURL:              "https://example.com/igdb-1-cover.png",
	}
	if err := store.SaveManualReviewResult(ctx, &core.SourceGame{
		ID:            "scan:review-apply",
		IntegrationID: "integration-1",
		PluginID:      "game-source-steam",
		ExternalID:    "review-apply",
		RawTitle:      "Mystery Setup",
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		RootPath:      "C:/Games/Mystery Setup",
		Status:        "found",
		ReviewState:   core.ManualReviewStateMatched,
		ManualReview: &core.ManualReviewDecision{
			State:    core.ManualReviewStateMatched,
			Selected: selection,
		},
	}, []core.ResolverMatch{{
		PluginID:        "metadata-igdb",
		ExternalID:      "igdb-1",
		Title:           "Mystery Game",
		Platform:        string(core.PlatformWindowsPC),
		Kind:            string(core.GameKindBaseGame),
		URL:             "https://example.com/igdb-1",
		ManualSelection: true,
	}}, []core.MediaRef{{
		Type:   core.MediaTypeCover,
		URL:    "https://example.com/igdb-1-cover.png",
		Source: "metadata-igdb",
	}}); err != nil {
		t.Fatal(err)
	}

	active, err := store.ListManualReviewCandidates(ctx, core.ManualReviewScopeActive, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("len(active) = %d, want 0 after apply", len(active))
	}

	candidate, err := store.GetManualReviewCandidate(ctx, "scan:review-apply")
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil {
		t.Fatal("expected applied manual review candidate detail")
	}
	if candidate.ReviewState != core.ManualReviewStateMatched {
		t.Fatalf("review_state = %q, want %q", candidate.ReviewState, core.ManualReviewStateMatched)
	}
	if candidate.CanonicalGameID == "" {
		t.Fatal("expected canonical_game_id after apply")
	}
	if len(candidate.ResolverMatches) != 1 {
		t.Fatalf("len(resolver_matches) = %d, want 1", len(candidate.ResolverMatches))
	}
	if !candidate.ResolverMatches[0].ManualSelection {
		t.Fatal("expected resolver match to be marked manual_selection")
	}

	var reviewState, manualReviewJSON string
	if err := db.GetDB().QueryRowContext(ctx,
		`SELECT review_state, COALESCE(manual_review_json, '') FROM source_games WHERE id = ?`,
		"scan:review-apply",
	).Scan(&reviewState, &manualReviewJSON); err != nil {
		t.Fatal(err)
	}
	if reviewState != string(core.ManualReviewStateMatched) {
		t.Fatalf("review_state column = %q, want %q", reviewState, core.ManualReviewStateMatched)
	}
	if !strings.Contains(manualReviewJSON, `"external_id":"igdb-1"`) {
		t.Fatalf("manual_review_json = %q, want saved selection payload", manualReviewJSON)
	}

	game, err := store.GetCanonicalGameByID(ctx, candidate.CanonicalGameID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil {
		t.Fatal("expected canonical game after apply")
	}
	if game.Title != "Mystery Game" {
		t.Fatalf("canonical title = %q, want %q", game.Title, "Mystery Game")
	}
}

func TestPersistScanResultsPreservesStickyManualSelectionAcrossRefresh(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:sticky",
			IntegrationID: "integration-1",
			PluginID:      "game-source-steam",
			ExternalID:    "sticky",
			RawTitle:      "Sticky Setup",
			Platform:      core.PlatformUnknown,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindUnknown,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	selection := &core.ManualReviewSelection{
		ProviderIntegrationID: "metadata-1",
		ProviderPluginID:      "metadata-igdb",
		Title:                 "Chosen Game",
		Platform:              string(core.PlatformWindowsPC),
		ExternalID:            "igdb-1",
	}
	if err := store.SaveManualReviewResult(ctx, &core.SourceGame{
		ID:            "scan:sticky",
		IntegrationID: "integration-1",
		PluginID:      "game-source-steam",
		ExternalID:    "sticky",
		RawTitle:      "Sticky Setup",
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		Status:        "found",
		ReviewState:   core.ManualReviewStateMatched,
		ManualReview: &core.ManualReviewDecision{
			State:    core.ManualReviewStateMatched,
			Selected: selection,
		},
	}, []core.ResolverMatch{{
		PluginID:        "metadata-igdb",
		ExternalID:      "igdb-1",
		Title:           "Chosen Game",
		Platform:        string(core.PlatformWindowsPC),
		ManualSelection: true,
	}}, nil); err != nil {
		t.Fatal(err)
	}

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:sticky",
			IntegrationID: "integration-1",
			PluginID:      "game-source-steam",
			ExternalID:    "sticky",
			RawTitle:      "Sticky Setup",
			Platform:      core.PlatformUnknown,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindUnknown,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:sticky": {{
				PluginID:   "metadata-other",
				ExternalID: "other-1",
				Title:      "Wrong Game",
				Platform:   string(core.PlatformWindowsPC),
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	candidate, err := store.GetManualReviewCandidate(ctx, "scan:sticky")
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil {
		t.Fatal("expected sticky candidate detail")
	}
	if candidate.ReviewState != core.ManualReviewStateMatched {
		t.Fatalf("review_state = %q, want %q", candidate.ReviewState, core.ManualReviewStateMatched)
	}
	if len(candidate.ResolverMatches) != 2 {
		t.Fatalf("len(resolver_matches) = %d, want 2", len(candidate.ResolverMatches))
	}

	var manualMatch, outvotedMatch *core.ResolverMatch
	for i := range candidate.ResolverMatches {
		match := &candidate.ResolverMatches[i]
		if match.ManualSelection {
			manualMatch = match
		}
		if match.PluginID == "metadata-other" {
			outvotedMatch = match
		}
	}
	if manualMatch == nil || manualMatch.Title != "Chosen Game" {
		t.Fatalf("manual match = %+v, want sticky selected title", manualMatch)
	}
	if outvotedMatch == nil || !outvotedMatch.Outvoted {
		t.Fatalf("outvoted match = %+v, want conflicting refresh result to be outvoted", outvotedMatch)
	}

	var reviewState string
	if err := db.GetDB().QueryRowContext(ctx,
		`SELECT review_state FROM source_games WHERE id = ?`,
		"scan:sticky",
	).Scan(&reviewState); err != nil {
		t.Fatal(err)
	}
	if reviewState != string(core.ManualReviewStateMatched) {
		t.Fatalf("review_state column = %q, want %q", reviewState, core.ManualReviewStateMatched)
	}
}

func newTestGameStore(t *testing.T) (*sqliteDatabase, *gameStore) {
	return newTestGameStoreWithLogger(t, testLogger{})
}

func newTestGameStoreWithLogger(t *testing.T, logger core.Logger) (*sqliteDatabase, *gameStore) {
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

	store := NewGameStore(db, logger).(*gameStore)
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
