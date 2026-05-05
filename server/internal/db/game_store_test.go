package db

import (
	"context"
	"database/sql"
	"errors"
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
	if _, err := db.GetDB().ExecContext(ctx, `INSERT INTO metadata_resolver_matches
		(source_game_id, plugin_id, external_id, title, platform, outvoted, manual_selection, rating, created_at)
		VALUES (?, ?, ?, ?, ?, 0, 0, 0, ?)`,
		"scan:legacy-source", "metadata-igdb", "legacy-match", "Legacy Game", "windows_pc", 1700000000,
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

func TestPersistScanResultsDuplicateFilePathKeepsLatestValues(t *testing.T) {
	ctx := context.Background()
	logger := &warningCaptureLogger{}
	db, store := newTestGameStoreWithLogger(t, logger)

	err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:dupe-files",
			IntegrationID: "integration-1",
			PluginID:      "game-source-steam",
			ExternalID:    "dupe-files",
			RawTitle:      "Duplicate Files",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
			Files: []core.GameFile{
				{
					Path:     `C:\Games\Duplicate Files\game.exe`,
					FileName: "game-old.exe",
					Role:     core.GameFileRoleOptional,
					FileKind: "bin",
					Size:     100,
				},
				{
					Path:     `C:/Games/Duplicate Files/game.exe`,
					FileName: "game.exe",
					Role:     core.GameFileRoleRoot,
					FileKind: "exe",
					Size:     200,
				},
				{
					Path:     `C:/Games/Duplicate Files/game.exe`,
					FileName: "game.exe",
					Role:     core.GameFileRoleRoot,
					FileKind: "exe",
					Size:     200,
				},
			},
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:cover-source": {{
				PluginID:   "metadata-igdb",
				Title:      "Cover Game",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "cover-game",
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	})
	if err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM game_files WHERE source_game_id = ?`, "scan:dupe-files").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("file_count = %d, want 1 after duplicate path normalization", count)
	}

	var path, fileName, role, fileKind string
	var size int64
	var isDir int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT path, file_name, role, file_kind, size, is_dir
		FROM game_files WHERE source_game_id = ?`, "scan:dupe-files").Scan(&path, &fileName, &role, &fileKind, &size, &isDir); err != nil {
		t.Fatal(err)
	}
	if path != "C:/Games/Duplicate Files/game.exe" {
		t.Fatalf("path = %q, want normalized latest path", path)
	}
	if fileName != "game.exe" || role != string(core.GameFileRoleRoot) || fileKind != "exe" || size != 200 || isDir != 0 {
		t.Fatalf("persisted file = {%q %q %q %d %d}, want latest values", fileName, role, fileKind, size, isDir)
	}

	if len(logger.warnings) != 1 {
		t.Fatalf("warning count = %d, want 1 for differing duplicate file path", len(logger.warnings))
	}
	if !strings.Contains(logger.warnings[0], "duplicate game file in scan batch") {
		t.Fatalf("warning = %q, want duplicate file warning", logger.warnings[0])
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

func TestCanonicalCoverOverrideUsesLinkedMediaAndCanBeCleared(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-cover",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:cover-source",
			IntegrationID: "integration-cover",
			PluginID:      "game-source-steam",
			ExternalID:    "cover-source",
			RawTitle:      "Cover Game",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:cover-source": {{
				PluginID:   "metadata-igdb",
				Title:      "Cover Game",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "cover-game",
			}},
		},
		MediaItems: map[string][]core.MediaRef{
			"scan:cover-source": {
				{Type: core.MediaTypeCover, URL: "https://example.com/default-cover.png"},
				{Type: core.MediaTypeArtwork, URL: "https://example.com/artwork.png"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var canonicalID string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id=?`, "scan:cover-source").Scan(&canonicalID); err != nil {
		var sourceCount, matchCount, linkCount int
		_ = db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM source_games WHERE id=?`, "scan:cover-source").Scan(&sourceCount)
		_ = db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM metadata_resolver_matches WHERE source_game_id=?`, "scan:cover-source").Scan(&matchCount)
		_ = db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM canonical_source_games_link`).Scan(&linkCount)
		t.Fatalf("load canonical link: %v source_count=%d match_count=%d link_count=%d", err, sourceCount, matchCount, linkCount)
	}
	game, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil || len(game.Media) < 2 {
		t.Fatalf("game media = %+v, want two media refs", game)
	}
	artworkAssetID := game.Media[1].AssetID
	if err := store.SetCanonicalCoverOverride(ctx, canonicalID, artworkAssetID); err != nil {
		t.Fatal(err)
	}
	game, err = store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game.CoverOverride == nil || game.CoverOverride.AssetID != artworkAssetID {
		t.Fatalf("cover override = %+v, want asset %d", game.CoverOverride, artworkAssetID)
	}
	if err := store.ClearCanonicalCoverOverride(ctx, canonicalID); err != nil {
		t.Fatal(err)
	}
	game, err = store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game.CoverOverride != nil {
		t.Fatalf("cover override = %+v, want nil after clear", game.CoverOverride)
	}
}

func TestCanonicalCoverOverrideRejectsUnlinkedMedia(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	for _, item := range []struct {
		sourceID string
		url      string
	}{
		{sourceID: "scan:cover-a", url: "https://example.com/a.png"},
		{sourceID: "scan:cover-b", url: "https://example.com/b.png"},
	} {
		if err := store.PersistScanResults(ctx, &core.ScanBatch{
			IntegrationID: item.sourceID,
			SourceGames: []*core.SourceGame{{
				ID:            item.sourceID,
				IntegrationID: item.sourceID,
				PluginID:      "game-source-steam",
				ExternalID:    item.sourceID,
				RawTitle:      item.sourceID,
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			}},
			ResolverMatches: map[string][]core.ResolverMatch{
				item.sourceID: {{
					PluginID:   "metadata-igdb",
					Title:      item.sourceID,
					Platform:   string(core.PlatformWindowsPC),
					ExternalID: item.sourceID,
				}},
			},
			MediaItems: map[string][]core.MediaRef{
				item.sourceID: {{Type: core.MediaTypeCover, URL: item.url}},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	var assetB int
	var canonicalAString string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id=?`, "scan:cover-a").Scan(&canonicalAString); err != nil {
		t.Fatal(err)
	}
	if canonicalAString == "" {
		t.Fatal("expected canonical id")
	}
	if err := db.GetDB().QueryRowContext(ctx, `SELECT ma.id FROM media_assets ma JOIN source_game_media sgm ON sgm.media_asset_id=ma.id WHERE sgm.source_game_id=?`, "scan:cover-b").Scan(&assetB); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCanonicalCoverOverride(ctx, canonicalAString, assetB); !errors.Is(err, core.ErrCoverOverrideMediaNotFound) {
		t.Fatalf("error = %v, want %v", err, core.ErrCoverOverrideMediaNotFound)
	}
}

func TestCanonicalHoverOverrideUsesLinkedMedia(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-hover",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:hover-source",
			IntegrationID: "integration-hover",
			PluginID:      "game-source-steam",
			ExternalID:    "hover-source",
			RawTitle:      "Hover Game",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:hover-source": {{
				PluginID:   "metadata-igdb",
				Title:      "Hover Game",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "hover-game",
			}},
		},
		MediaItems: map[string][]core.MediaRef{
			"scan:hover-source": {
				{Type: core.MediaTypeCover, URL: "https://example.com/default-cover.png"},
				{Type: core.MediaTypeScreenshot, URL: "https://example.com/hover-shot.png"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var canonicalID string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id=?`, "scan:hover-source").Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}
	game, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil || len(game.Media) < 2 {
		t.Fatalf("game media = %+v, want two media refs", game)
	}
	hoverAssetID := game.Media[1].AssetID
	if err := store.SetCanonicalHoverOverride(ctx, canonicalID, hoverAssetID); err != nil {
		t.Fatal(err)
	}
	game, err = store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game.HoverOverride == nil || game.HoverOverride.AssetID != hoverAssetID {
		t.Fatalf("hover override = %+v, want asset %d", game.HoverOverride, hoverAssetID)
	}
}

func TestCanonicalHoverOverrideRejectsUnlinkedMedia(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	for _, item := range []struct {
		sourceID string
		url      string
	}{
		{sourceID: "scan:hover-a", url: "https://example.com/a.png"},
		{sourceID: "scan:hover-b", url: "https://example.com/b.png"},
	} {
		if err := store.PersistScanResults(ctx, &core.ScanBatch{
			IntegrationID: item.sourceID,
			SourceGames: []*core.SourceGame{{
				ID:            item.sourceID,
				IntegrationID: item.sourceID,
				PluginID:      "game-source-steam",
				ExternalID:    item.sourceID,
				RawTitle:      item.sourceID,
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			}},
			ResolverMatches: map[string][]core.ResolverMatch{
				item.sourceID: {{
					PluginID:   "metadata-igdb",
					Title:      item.sourceID,
					Platform:   string(core.PlatformWindowsPC),
					ExternalID: item.sourceID,
				}},
			},
			MediaItems: map[string][]core.MediaRef{
				item.sourceID: {{Type: core.MediaTypeCover, URL: item.url}},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	var assetB int
	var canonicalAString string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id=?`, "scan:hover-a").Scan(&canonicalAString); err != nil {
		t.Fatal(err)
	}
	if err := db.GetDB().QueryRowContext(ctx, `SELECT ma.id FROM media_assets ma JOIN source_game_media sgm ON sgm.media_asset_id=ma.id WHERE sgm.source_game_id=?`, "scan:hover-b").Scan(&assetB); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCanonicalHoverOverride(ctx, canonicalAString, assetB); !errors.Is(err, core.ErrHoverOverrideMediaNotFound) {
		t.Fatalf("error = %v, want %v", err, core.ErrHoverOverrideMediaNotFound)
	}
}

func TestCanonicalBackgroundOverrideUsesLinkedMedia(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-background",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:background-source",
			IntegrationID: "integration-background",
			PluginID:      "game-source-steam",
			ExternalID:    "background-source",
			RawTitle:      "Background Game",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:background-source": {{
				PluginID:   "metadata-igdb",
				Title:      "Background Game",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "background-game",
			}},
		},
		MediaItems: map[string][]core.MediaRef{
			"scan:background-source": {
				{Type: core.MediaTypeCover, URL: "https://example.com/default-cover.png"},
				{Type: core.MediaTypeBackground, URL: "https://example.com/background.png"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var canonicalID string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id=?`, "scan:background-source").Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}
	game, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil || len(game.Media) < 2 {
		t.Fatalf("game media = %+v, want two media refs", game)
	}
	backgroundAssetID := game.Media[1].AssetID
	if err := store.SetCanonicalBackgroundOverride(ctx, canonicalID, backgroundAssetID); err != nil {
		t.Fatal(err)
	}
	game, err = store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game.BackgroundOverride == nil || game.BackgroundOverride.AssetID != backgroundAssetID {
		t.Fatalf("background override = %+v, want asset %d", game.BackgroundOverride, backgroundAssetID)
	}
}

func TestCanonicalBackgroundOverrideRejectsUnlinkedMedia(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	for _, item := range []struct {
		sourceID string
		url      string
	}{
		{sourceID: "scan:background-a", url: "https://example.com/a.png"},
		{sourceID: "scan:background-b", url: "https://example.com/b.png"},
	} {
		if err := store.PersistScanResults(ctx, &core.ScanBatch{
			IntegrationID: item.sourceID,
			SourceGames: []*core.SourceGame{{
				ID:            item.sourceID,
				IntegrationID: item.sourceID,
				PluginID:      "game-source-steam",
				ExternalID:    item.sourceID,
				RawTitle:      item.sourceID,
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			}},
			ResolverMatches: map[string][]core.ResolverMatch{
				item.sourceID: {{
					PluginID:   "metadata-igdb",
					Title:      item.sourceID,
					Platform:   string(core.PlatformWindowsPC),
					ExternalID: item.sourceID,
				}},
			},
			MediaItems: map[string][]core.MediaRef{
				item.sourceID: {{Type: core.MediaTypeCover, URL: item.url}},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	var assetB int
	var canonicalAString string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id=?`, "scan:background-a").Scan(&canonicalAString); err != nil {
		t.Fatal(err)
	}
	if err := db.GetDB().QueryRowContext(ctx, `SELECT ma.id FROM media_assets ma JOIN source_game_media sgm ON sgm.media_asset_id=ma.id WHERE sgm.source_game_id=?`, "scan:background-b").Scan(&assetB); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCanonicalBackgroundOverride(ctx, canonicalAString, assetB); !errors.Is(err, core.ErrBackgroundOverrideMediaNotFound) {
		t.Fatalf("error = %v, want %v", err, core.ErrBackgroundOverrideMediaNotFound)
	}
}

func TestCanonicalMediaOverrideBackfillPersistsLegacySelections(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-legacy-overrides",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:legacy-overrides",
			IntegrationID: "integration-legacy-overrides",
			PluginID:      "game-source-steam",
			ExternalID:    "legacy-overrides",
			RawTitle:      "Legacy Overrides",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:legacy-overrides": {{
				PluginID:   "metadata-igdb",
				Title:      "Legacy Overrides",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "legacy-overrides",
			}},
		},
		MediaItems: map[string][]core.MediaRef{
			"scan:legacy-overrides": {
				{Type: core.MediaTypeCover, URL: "https://example.com/legacy-cover.png"},
				{Type: core.MediaTypeScreenshot, URL: "https://example.com/legacy-shot.png"},
				{Type: core.MediaTypeBackground, URL: "https://example.com/legacy-background.png"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var canonicalID string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id=?`, "scan:legacy-overrides").Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}

	game, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil {
		t.Fatal("expected canonical game")
	}
	if game.CoverOverride == nil || !strings.Contains(game.CoverOverride.URL, "legacy-cover.png") {
		t.Fatalf("cover override = %+v, want legacy cover backfill", game.CoverOverride)
	}
	if game.HoverOverride == nil || !strings.Contains(game.HoverOverride.URL, "legacy-shot.png") {
		t.Fatalf("hover override = %+v, want legacy screenshot backfill", game.HoverOverride)
	}
	if game.BackgroundOverride == nil || !strings.Contains(game.BackgroundOverride.URL, "legacy-shot.png") {
		t.Fatalf("background override = %+v, want current effective screenshot backdrop backfill", game.BackgroundOverride)
	}

	var coverAssetID, hoverAssetID, backgroundAssetID int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT media_asset_id FROM canonical_game_cover_overrides WHERE canonical_id=?`, canonicalID).Scan(&coverAssetID); err != nil {
		t.Fatalf("cover override row missing: %v", err)
	}
	if err := db.GetDB().QueryRowContext(ctx, `SELECT media_asset_id FROM canonical_game_hover_overrides WHERE canonical_id=?`, canonicalID).Scan(&hoverAssetID); err != nil {
		t.Fatalf("hover override row missing: %v", err)
	}
	if err := db.GetDB().QueryRowContext(ctx, `SELECT media_asset_id FROM canonical_game_background_overrides WHERE canonical_id=?`, canonicalID).Scan(&backgroundAssetID); err != nil {
		t.Fatalf("background override row missing: %v", err)
	}
	if game.CoverOverride.AssetID != coverAssetID {
		t.Fatalf("cover asset = %d, want persisted %d", game.CoverOverride.AssetID, coverAssetID)
	}
	if game.HoverOverride.AssetID != hoverAssetID {
		t.Fatalf("hover asset = %d, want persisted %d", game.HoverOverride.AssetID, hoverAssetID)
	}
	if game.BackgroundOverride.AssetID != backgroundAssetID {
		t.Fatalf("background asset = %d, want persisted %d", game.BackgroundOverride.AssetID, backgroundAssetID)
	}

	reloaded, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CoverOverride == nil || reloaded.HoverOverride == nil || reloaded.BackgroundOverride == nil {
		t.Fatalf("reloaded overrides = %+v %+v %+v, want persisted overrides", reloaded.CoverOverride, reloaded.HoverOverride, reloaded.BackgroundOverride)
	}
}

func TestCanonicalMediaOverrideBackfillPreservesExplicitSelections(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-explicit-cover",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:explicit-cover",
			IntegrationID: "integration-explicit-cover",
			PluginID:      "game-source-steam",
			ExternalID:    "explicit-cover",
			RawTitle:      "Explicit Cover",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:explicit-cover": {{
				PluginID:   "metadata-igdb",
				Title:      "Explicit Cover",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "explicit-cover",
			}},
		},
		MediaItems: map[string][]core.MediaRef{
			"scan:explicit-cover": {
				{Type: core.MediaTypeCover, URL: "https://example.com/default-cover.png"},
				{Type: core.MediaTypeArtwork, URL: "https://example.com/explicit-artwork.png"},
				{Type: core.MediaTypeScreenshot, URL: "https://example.com/explicit-shot.png"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var canonicalID string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id=?`, "scan:explicit-cover").Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}
	game, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil || len(game.Media) < 3 {
		t.Fatalf("game media = %+v, want three media refs", game)
	}

	artworkAssetID := game.Media[1].AssetID
	if err := store.SetCanonicalCoverOverride(ctx, canonicalID, artworkAssetID); err != nil {
		t.Fatal(err)
	}

	reloaded, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CoverOverride == nil || reloaded.CoverOverride.AssetID != artworkAssetID {
		t.Fatalf("cover override = %+v, want preserved explicit artwork %d", reloaded.CoverOverride, artworkAssetID)
	}
	if reloaded.HoverOverride == nil || !strings.Contains(reloaded.HoverOverride.URL, "explicit-shot.png") {
		t.Fatalf("hover override = %+v, want screenshot backfill", reloaded.HoverOverride)
	}
}

func TestResolveCanonicalMediaRefByIdentityFallsBackToURL(t *testing.T) {
	media := []core.MediaRef{
		{AssetID: 12, Type: core.MediaTypeCover, URL: "https://example.com/assets/cover.png#fragment"},
	}

	resolved := resolveCanonicalMediaRefByIdentity(media, &core.MediaRef{URL: "https://example.com/assets/cover.png"})
	if resolved == nil {
		t.Fatal("expected URL identity fallback to resolve media ref")
	}
	if resolved.AssetID != 12 {
		t.Fatalf("resolved asset = %d, want 12", resolved.AssetID)
	}
}

func TestUpdateMediaAssetMetadataBackfillsDimensions(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-media-meta",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:media-meta",
			IntegrationID: "integration-media-meta",
			PluginID:      "game-source-steam",
			ExternalID:    "media-meta",
			RawTitle:      "Media Meta",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:media-meta": {{
				PluginID:   "metadata-igdb",
				Title:      "Media Meta",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "media-meta",
			}},
		},
		MediaItems: map[string][]core.MediaRef{
			"scan:media-meta": {
				{Type: core.MediaTypeScreenshot, URL: "https://example.com/probe-me.png"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var assetID int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT ma.id FROM media_assets ma JOIN source_game_media sgm ON sgm.media_asset_id=ma.id WHERE sgm.source_game_id=?`, "scan:media-meta").Scan(&assetID); err != nil {
		t.Fatal(err)
	}

	if err := store.UpdateMediaAssetMetadata(ctx, assetID, 1920, 1080, "image/png"); err != nil {
		t.Fatal(err)
	}

	var width, height int
	var mimeType sql.NullString
	if err := db.GetDB().QueryRowContext(ctx, `SELECT width, height, mime_type FROM media_assets WHERE id=?`, assetID).Scan(&width, &height, &mimeType); err != nil {
		t.Fatal(err)
	}
	if width != 1920 || height != 1080 {
		t.Fatalf("dimensions = %dx%d, want 1920x1080", width, height)
	}
	if mimeType.String != "image/png" {
		t.Fatalf("mime_type = %q, want image/png", mimeType.String)
	}
}

func seedSourceGameForDBTest(t *testing.T, ctx context.Context, store core.GameStore, sourceID, title string) {
	t.Helper()
	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: sourceID,
		SourceGames: []*core.SourceGame{{
			ID:            sourceID,
			IntegrationID: sourceID,
			PluginID:      "game-source-steam",
			ExternalID:    sourceID,
			RawTitle:      title,
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			sourceID: {{
				PluginID:   "metadata-igdb",
				Title:      title,
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: sourceID,
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGetCachedAchievementsDashboardUsesCachedRowsOnly(t *testing.T) {
	ctx := context.Background()
	_, store := newTestGameStore(t)

	for _, item := range []struct {
		id    string
		title string
	}{
		{id: "scan:ach-one", title: "Achievement One"},
		{id: "scan:ach-two", title: "Achievement Two"},
		{id: "scan:ach-empty", title: "No Cache"},
	} {
		seedSourceGameForDBTest(t, ctx, store, item.id, item.title)
	}
	if err := store.CacheAchievements(ctx, "scan:ach-one", &core.AchievementSet{
		Source:         "retroachievements",
		ExternalGameID: "ra-1",
		TotalCount:     10,
		UnlockedCount:  4,
		TotalPoints:    100,
		EarnedPoints:   40,
		FetchedAt:      time.Unix(1710000000, 0).UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CacheAchievements(ctx, "scan:ach-two", &core.AchievementSet{
		Source:         "retroachievements",
		ExternalGameID: "ra-2",
		TotalCount:     5,
		UnlockedCount:  5,
		TotalPoints:    50,
		EarnedPoints:   50,
		FetchedAt:      time.Unix(1710000100, 0).UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	dashboard, err := store.GetCachedAchievementsDashboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.Totals.TotalCount != 15 || dashboard.Totals.UnlockedCount != 9 {
		t.Fatalf("totals = %+v, want 15 total and 9 unlocked", dashboard.Totals)
	}
	if len(dashboard.Systems) != 1 || dashboard.Systems[0].GameCount != 2 {
		t.Fatalf("systems = %+v, want one system with two games", dashboard.Systems)
	}
	if len(dashboard.Games) != 2 {
		t.Fatalf("games = %d, want only cached achievement games", len(dashboard.Games))
	}
}

func TestGetCachedAchievementsExplorerUsesCachedRowsOnly(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	seedSourceGameForDBTest(t, ctx, store, "scan:ach-exp-one", "Explorer One")
	seedSourceGameForDBTest(t, ctx, store, "scan:ach-exp-two", "Explorer Two")
	seedSourceGameForDBTest(t, ctx, store, "scan:ach-exp-one-dup", "Explorer One Duplicate")

	if err := store.CacheAchievements(ctx, "scan:ach-exp-one", &core.AchievementSet{
		Source:         "retroachievements",
		ExternalGameID: "ra-exp-1",
		TotalCount:     2,
		UnlockedCount:  1,
		FetchedAt:      time.Unix(1710000000, 0).UTC(),
		Achievements: []core.Achievement{
			{ExternalID: "ach-1", Title: "First", Unlocked: true, UnlockedAt: time.Unix(1710000000, 0).UTC()},
			{ExternalID: "ach-2", Title: "Second", Unlocked: false},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CacheAchievements(ctx, "scan:ach-exp-two", &core.AchievementSet{
		Source:         "retroachievements",
		ExternalGameID: "ra-exp-2",
		TotalCount:     1,
		UnlockedCount:  1,
		FetchedAt:      time.Unix(1710000100, 0).UTC(),
		Achievements: []core.Achievement{
			{ExternalID: "ach-3", Title: "Third", Unlocked: true, UnlockedAt: time.Unix(1710000100, 0).UTC()},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var canonicalID string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id = ?`, "scan:ach-exp-one").Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDB().ExecContext(ctx, `UPDATE canonical_source_games_link SET canonical_id = ? WHERE source_game_id = ?`, canonicalID, "scan:ach-exp-one-dup"); err != nil {
		t.Fatal(err)
	}
	if err := store.CacheAchievements(ctx, "scan:ach-exp-one-dup", &core.AchievementSet{
		Source:         "retroachievements",
		ExternalGameID: "ra-exp-1",
		TotalCount:     99,
		UnlockedCount:  0,
		FetchedAt:      time.Unix(1700000000, 0).UTC(),
		Achievements: []core.Achievement{
			{ExternalID: "ach-old", Title: "Old Duplicate", Unlocked: false},
		},
	}); err != nil {
		t.Fatal(err)
	}

	explorer, err := store.GetCachedAchievementsExplorer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(explorer.Games) != 2 {
		t.Fatalf("games = %d, want 2 cached explorer games", len(explorer.Games))
	}

	var found bool
	for _, item := range explorer.Games {
		if item.Game == nil || item.Game.ID != canonicalID {
			continue
		}
		found = true
		if len(item.Systems) != 1 {
			t.Fatalf("systems = %d, want deduped single system", len(item.Systems))
		}
		if item.Systems[0].TotalCount != 2 || len(item.Systems[0].Achievements) != 2 {
			t.Fatalf("system = %+v, want latest cached set with 2 achievements", item.Systems[0])
		}
		if item.Systems[0].UnlockedCount != 1 {
			t.Fatalf("unlocked_count = %d, want 1", item.Systems[0].UnlockedCount)
		}
		if !item.Systems[0].Achievements[0].Unlocked || item.Systems[0].Achievements[0].UnlockedAt.IsZero() {
			t.Fatalf("first achievement = %+v, want unlocked with timestamp", item.Systems[0].Achievements[0])
		}
		if item.Systems[0].Achievements[1].Unlocked || !item.Systems[0].Achievements[1].UnlockedAt.IsZero() {
			t.Fatalf("second achievement = %+v, want locked without timestamp", item.Systems[0].Achievements[1])
		}
	}
	if !found {
		t.Fatalf("expected explorer data for canonical game %q", canonicalID)
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

func TestUndetectedCandidatesAreHiddenFromLibraryVisibilityAndBecomeVisibleWhenMatched(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

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

	if n, err := store.CountVisibleCanonicalGames(ctx); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatalf("count visible canonical = %d, want 1", n)
	}

	visibleIDs, err := store.GetVisibleCanonicalIDs(ctx, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(visibleIDs) != 1 {
		t.Fatalf("len(visible_ids) = %d, want 1", len(visibleIDs))
	}
	cleanCanonicalID := canonicalIDForSource(t, ctx, db, "scan:clean")
	if visibleIDs[0] != cleanCanonicalID {
		t.Fatalf("visible canonical id = %q, want %q", visibleIDs[0], cleanCanonicalID)
	}

	games, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("len(canonical games) = %d, want 1", len(games))
	}
	if len(games[0].SourceGames) != 1 || games[0].SourceGames[0].ID != "scan:clean" {
		t.Fatalf("canonical source games = %+v, want only clean source", games[0].SourceGames)
	}

	foundGames, err := store.GetFoundSourceGames(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(foundGames) != 1 || foundGames[0].ID != "scan:clean" {
		t.Fatalf("found games = %+v, want only clean source", foundGames)
	}

	stats, err := store.GetLibraryStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CanonicalGameCount != 1 {
		t.Fatalf("canonical_game_count = %d, want 1", stats.CanonicalGameCount)
	}
	if stats.SourceGameFoundCount != 1 {
		t.Fatalf("source_game_found_count = %d, want 1", stats.SourceGameFoundCount)
	}
	if stats.ByIntegrationID["integration-1"] != 1 {
		t.Fatalf("by_integration = %+v, want integration-1 => 1", stats.ByIntegrationID)
	}
	if stats.ByPluginID["game-source-steam"] != 1 {
		t.Fatalf("by_plugin = %+v, want game-source-steam => 1", stats.ByPluginID)
	}

	integrationGames, err := store.GetGamesByIntegrationID(ctx, "integration-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(integrationGames) != 1 || integrationGames[0].Title != "Resolved Game" {
		t.Fatalf("integration games = %+v, want only resolved game", integrationGames)
	}

	counts, err := store.GetSourceGameCountsByIntegration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts["integration-1"] != 1 {
		t.Fatalf("source game counts = %+v, want integration-1 => 1", counts)
	}

	if err := store.SaveManualReviewResult(ctx, &core.SourceGame{
		ID:            "scan:review-no-match",
		IntegrationID: "integration-1",
		PluginID:      "game-source-steam",
		ExternalID:    "review-no-match",
		RawTitle:      "Unidentified Game",
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		Status:        "found",
		ReviewState:   core.ManualReviewStateMatched,
		ManualReview: &core.ManualReviewDecision{
			State: core.ManualReviewStateMatched,
			Selected: &core.ManualReviewSelection{
				ProviderPluginID: "metadata-igdb",
				ExternalID:       "review-match",
				Title:            "Identified Game",
				Platform:         string(core.PlatformWindowsPC),
				Kind:             string(core.GameKindBaseGame),
			},
		},
	}, []core.ResolverMatch{{
		PluginID:        "metadata-igdb",
		ExternalID:      "review-match",
		Title:           "Identified Game",
		Platform:        string(core.PlatformWindowsPC),
		ManualSelection: true,
	}}, nil); err != nil {
		t.Fatal(err)
	}

	if n, err := store.CountVisibleCanonicalGames(ctx); err != nil {
		t.Fatal(err)
	} else if n != 2 {
		t.Fatalf("count visible canonical after match = %d, want 2", n)
	}

	foundGames, err = store.GetFoundSourceGames(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(foundGames) != 2 {
		t.Fatalf("len(found games after match) = %d, want 2", len(foundGames))
	}

	reviewCanonicalID := canonicalIDForSource(t, ctx, db, "scan:review-no-match")
	game, err := store.GetCanonicalGameByID(ctx, reviewCanonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil {
		t.Fatal("expected canonical game for matched review candidate")
	}
	if game.Title != "Identified Game" {
		t.Fatalf("canonical title = %q, want %q", game.Title, "Identified Game")
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

func TestManualReviewCandidateAccessIsProfileScoped(t *testing.T) {
	ctx := context.Background()
	profileOneCtx := core.WithProfile(ctx, &core.Profile{ID: "profile-1", Role: core.ProfileRoleAdminPlayer})
	profileTwoCtx := core.WithProfile(ctx, &core.Profile{ID: "profile-2", Role: core.ProfileRolePlayer})
	_, store := newTestGameStore(t)

	persistBatch(t, profileOneCtx, store, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:profile-1-review",
			IntegrationID: "integration-1",
			PluginID:      "game-source-smb",
			ExternalID:    "profile-1-review",
			RawTitle:      "Profile One Review",
			Platform:      core.PlatformUnknown,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindUnknown,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
	})
	persistBatch(t, profileTwoCtx, store, &core.ScanBatch{
		IntegrationID: "integration-2",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:profile-2-review",
			IntegrationID: "integration-2",
			PluginID:      "game-source-smb",
			ExternalID:    "profile-2-review",
			RawTitle:      "Profile Two Review",
			Platform:      core.PlatformUnknown,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindUnknown,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
	})

	candidates, err := store.ListManualReviewCandidates(profileOneCtx, core.ManualReviewScopeActive, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ID != "scan:profile-1-review" {
		t.Fatalf("profile one candidates = %+v, want only profile one candidate", candidates)
	}
	otherCandidate, err := store.GetManualReviewCandidate(profileOneCtx, "scan:profile-2-review")
	if err != nil {
		t.Fatal(err)
	}
	if otherCandidate != nil {
		t.Fatalf("profile one loaded profile two candidate: %+v", otherCandidate)
	}
	if err := store.SetManualReviewState(profileOneCtx, "scan:profile-2-review", core.ManualReviewStateNotAGame); !errors.Is(err, core.ErrManualReviewCandidateNotFound) {
		t.Fatalf("SetManualReviewState cross-profile error = %v, want ErrManualReviewCandidateNotFound", err)
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

func TestCanonicalGroupingMergesProviderBackedCleanTitleVersions(t *testing.T) {
	ctx := context.Background()
	_, store := newTestGameStore(t)

	persistBatch(t, ctx, store, &core.ScanBatch{
		IntegrationID: "roms",
		SourceGames: []*core.SourceGame{
			{
				ID:            "source-arcade",
				IntegrationID: "roms",
				PluginID:      "game-source-mame",
				ExternalID:    "mame-altbeast",
				RawTitle:      "Altered Beast (set 8) (8751 317-0078)",
				Platform:      core.PlatformArcade,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			},
			{
				ID:            "source-genesis",
				IntegrationID: "roms",
				PluginID:      "game-source-smb",
				ExternalID:    "genesis-altbeast",
				RawTitle:      "Altered Beast [!]",
				Platform:      core.PlatformGenesis,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{
			"source-arcade": {{
				PluginID:   "retroachievements",
				Title:      "Altered Beast (set 8) (8751 317-0078)",
				ExternalID: "11975",
			}},
			"source-genesis": {{
				PluginID:   "retroachievements",
				Title:      "Altered Beast",
				ExternalID: "24",
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	})

	games, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatalf("GetCanonicalGames: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("canonical games = %d, want 1", len(games))
	}
	if len(games[0].SourceGames) != 2 {
		t.Fatalf("source games = %d, want 2", len(games[0].SourceGames))
	}
	if games[0].Title != "Altered Beast" {
		t.Fatalf("canonical title = %q, want Altered Beast", games[0].Title)
	}
	if games[0].SourceGames[0].RawTitle == games[0].Title && games[0].SourceGames[0].ID == "source-arcade" {
		t.Fatalf("source raw title was unexpectedly cleaned: %+v", games[0].SourceGames[0])
	}
}

func TestCanonicalGroupingDoesNotMergeRawTitleOnlyRecords(t *testing.T) {
	ctx := context.Background()
	_, store := newTestGameStore(t)

	persistBatch(t, ctx, store, &core.ScanBatch{
		IntegrationID: "manual",
		SourceGames: []*core.SourceGame{
			{
				ID:            "source-one",
				IntegrationID: "manual",
				PluginID:      "game-source-smb",
				ExternalID:    "one",
				RawTitle:      "Altered Beast",
				Platform:      core.PlatformArcade,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
				ReviewState:   core.ManualReviewStateMatched,
			},
			{
				ID:            "source-two",
				IntegrationID: "manual",
				PluginID:      "game-source-smb",
				ExternalID:    "two",
				RawTitle:      "Altered Beast [!]",
				Platform:      core.PlatformGenesis,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
				ReviewState:   core.ManualReviewStateMatched,
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
	})

	games, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatalf("GetCanonicalGames: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("canonical games = %d, want 2", len(games))
	}
}

func TestCanonicalGroupingDoesNotMergeChildContentByTitle(t *testing.T) {
	ctx := context.Background()
	_, store := newTestGameStore(t)

	persistBatch(t, ctx, store, &core.ScanBatch{
		IntegrationID: "source",
		SourceGames: []*core.SourceGame{
			{
				ID:            "source-base",
				IntegrationID: "source",
				PluginID:      "game-source-smb",
				ExternalID:    "base",
				RawTitle:      "Doom",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			},
			{
				ID:            "source-dlc",
				IntegrationID: "source",
				PluginID:      "game-source-smb",
				ExternalID:    "dlc",
				RawTitle:      "Doom",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindDLC,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{
			"source-base": {{
				PluginID:   "metadata-test",
				Title:      "Doom",
				ExternalID: "doom-base",
			}},
			"source-dlc": {{
				PluginID:     "metadata-test",
				Title:        "Doom",
				Kind:         string(core.GameKindDLC),
				ParentGameID: "doom-base",
				ExternalID:   "doom-dlc",
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	})

	games, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatalf("GetCanonicalGames: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("canonical games = %d, want 2", len(games))
	}
}

func TestPersistScanResultsExplicitEmptyEntriesClearStaleMetadataAndMedia(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	sourceGame := &core.SourceGame{
		ID:            "scan:final-fantasy",
		IntegrationID: "integration-1",
		PluginID:      "game-source-xbox",
		ExternalID:    "xbox-final-fantasy",
		RawTitle:      "Final Fantasy",
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		Status:        "found",
	}

	persistBatch(t, ctx, store, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames:   []*core.SourceGame{sourceGame},
		ResolverMatches: map[string][]core.ResolverMatch{
			sourceGame.ID: {{
				PluginID:   "metadata-igdb",
				Title:      "Final Fantasy 2.0",
				ExternalID: "igdb-wrong",
			}},
		},
		MediaItems: map[string][]core.MediaRef{
			sourceGame.ID: {{
				Type:   core.MediaTypeScreenshot,
				URL:    "https://example.com/final-fantasy-2.png",
				Source: "metadata-igdb",
			}},
		},
	})

	persistBatch(t, ctx, store, &core.ScanBatch{
		IntegrationID:        "integration-1",
		SourceGames:          []*core.SourceGame{sourceGame},
		ResolverMatches:      map[string][]core.ResolverMatch{sourceGame.ID: {}},
		MediaItems:           map[string][]core.MediaRef{sourceGame.ID: {}},
		SkipMissingReconcile: true,
	})

	var matchCount int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM metadata_resolver_matches WHERE source_game_id = ?`, sourceGame.ID).Scan(&matchCount); err != nil {
		t.Fatal(err)
	}
	if matchCount != 0 {
		t.Fatalf("metadata_resolver_matches count = %d, want 0", matchCount)
	}

	var mediaCount int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM source_game_media WHERE source_game_id = ?`, sourceGame.ID).Scan(&mediaCount); err != nil {
		t.Fatal(err)
	}
	if mediaCount != 0 {
		t.Fatalf("source_game_media count = %d, want 0", mediaCount)
	}
}

func TestGetSourceGamesForCanonicalLoadsMedia(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	sourceGame := &core.SourceGame{
		ID:            "scan:xbox-media",
		IntegrationID: "integration-1",
		PluginID:      "game-source-xbox",
		ExternalID:    "xbox-final-fantasy",
		RawTitle:      "Final Fantasy",
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		Status:        "found",
	}

	persistBatch(t, ctx, store, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames:   []*core.SourceGame{sourceGame},
		ResolverMatches: map[string][]core.ResolverMatch{
			sourceGame.ID: {{
				PluginID:   "game-source-xbox",
				Title:      "Final Fantasy",
				ExternalID: "xbox-final-fantasy",
			}},
		},
		MediaItems: map[string][]core.MediaRef{
			sourceGame.ID: {{
				Type:   core.MediaTypeCover,
				URL:    "https://example.com/xbox-cover.jpg",
				Source: "game-source-xbox",
				Width:  600,
				Height: 800,
			}},
		},
	})

	canonicalID := canonicalIDForSource(t, ctx, db, sourceGame.ID)
	sourceGames, err := store.GetSourceGamesForCanonical(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceGames) != 1 {
		t.Fatalf("source game count = %d, want 1", len(sourceGames))
	}
	if len(sourceGames[0].Media) != 1 {
		t.Fatalf("media count = %d, want 1", len(sourceGames[0].Media))
	}
	ref := sourceGames[0].Media[0]
	if ref.URL != "https://example.com/xbox-cover.jpg" || ref.Source != "game-source-xbox" || ref.Width != 600 || ref.Height != 800 {
		t.Fatalf("loaded media = %+v, want xbox cover with dimensions", ref)
	}
}

func TestPersistMetadataRefreshPreservesXboxSourceRowsAndClearsStaleMetadataRows(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	sourceGame := &core.SourceGame{
		ID:            "scan:xbox-refresh",
		IntegrationID: "integration-1",
		PluginID:      "game-source-xbox",
		ExternalID:    "xbox-final-fantasy",
		RawTitle:      "Final Fantasy",
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		Status:        "found",
	}
	xboxMatch := core.ResolverMatch{
		PluginID:        "game-source-xbox",
		Title:           "Final Fantasy",
		ExternalID:      "xbox-final-fantasy",
		IsGamePass:      true,
		XcloudAvailable: true,
		StoreProductID:  "9NFINALFANTASY",
		XcloudURL:       "https://www.xbox.com/play/games/final-fantasy/9NFINALFANTASY",
	}
	xboxMedia := core.MediaRef{
		Type:   core.MediaTypeCover,
		URL:    "https://example.com/xbox-cover.jpg",
		Source: "game-source-xbox",
	}

	persistBatch(t, ctx, store, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames:   []*core.SourceGame{sourceGame},
		ResolverMatches: map[string][]core.ResolverMatch{
			sourceGame.ID: {
				xboxMatch,
				{PluginID: "metadata-igdb", Title: "Final Fantasy 2.0", ExternalID: "igdb-stale"},
			},
		},
		MediaItems: map[string][]core.MediaRef{
			sourceGame.ID: {
				xboxMedia,
				{Type: core.MediaTypeScreenshot, URL: "https://example.com/igdb-stale.png", Source: "metadata-igdb"},
			},
		},
	})

	persistBatch(t, ctx, store, &core.ScanBatch{
		IntegrationID:        "integration-1",
		SourceGames:          []*core.SourceGame{sourceGame},
		ResolverMatches:      map[string][]core.ResolverMatch{sourceGame.ID: {xboxMatch}},
		MediaItems:           map[string][]core.MediaRef{sourceGame.ID: {xboxMedia}},
		SkipMissingReconcile: true,
	})

	var igdbMatchCount int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM metadata_resolver_matches WHERE source_game_id = ? AND plugin_id = ?`, sourceGame.ID, "metadata-igdb").Scan(&igdbMatchCount); err != nil {
		t.Fatal(err)
	}
	if igdbMatchCount != 0 {
		t.Fatalf("igdb match count = %d, want 0", igdbMatchCount)
	}

	canonicalID := canonicalIDForSource(t, ctx, db, sourceGame.ID)
	game, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil {
		t.Fatal("expected canonical game")
	}
	if !game.IsGamePass || !game.XcloudAvailable || game.StoreProductID != "9NFINALFANTASY" || game.XcloudURL != "https://www.xbox.com/play/games/final-fantasy/9NFINALFANTASY" {
		t.Fatalf("xbox fields were not preserved: is_game_pass=%v xcloud=%v product=%q url=%q", game.IsGamePass, game.XcloudAvailable, game.StoreProductID, game.XcloudURL)
	}

	foundXboxMedia := false
	for _, ref := range game.Media {
		if ref.Source == "metadata-igdb" {
			t.Fatalf("stale metadata media was preserved: %+v", ref)
		}
		if ref.Source == "game-source-xbox" && ref.URL == xboxMedia.URL {
			foundXboxMedia = true
		}
	}
	if !foundXboxMedia {
		t.Fatalf("xbox media not found in canonical media: %+v", game.Media)
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

func TestCanonicalFavoritesPersistAndCascade(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	persistBatch(t, ctx, store, makeTestBatch("integration-1", "scan:favorite-a", "favorite-a", "Favorite A", "match-favorite-a"))
	canonicalID := canonicalIDForSource(t, ctx, db, "scan:favorite-a")

	game, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil {
		t.Fatal("expected canonical game")
	}
	if game.Favorite {
		t.Fatal("favorite should default to false")
	}

	if err := store.SetCanonicalFavorite(ctx, canonicalID); err != nil {
		t.Fatalf("SetCanonicalFavorite: %v", err)
	}

	game, err = store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil || !game.Favorite {
		t.Fatalf("favorite = %+v, want true", game)
	}

	games, err := store.GetCanonicalGamesByIDs(ctx, []string{canonicalID})
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 || games[0] == nil || !games[0].Favorite {
		t.Fatalf("games favorite = %+v, want one favorite game", games)
	}

	if err := store.ClearCanonicalFavorite(ctx, canonicalID); err != nil {
		t.Fatalf("ClearCanonicalFavorite: %v", err)
	}

	game, err = store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil || game.Favorite {
		t.Fatalf("favorite after clear = %+v, want false", game)
	}

	if err := store.SetCanonicalFavorite(ctx, canonicalID); err != nil {
		t.Fatalf("SetCanonicalFavorite second time: %v", err)
	}
	if _, err := db.GetDB().ExecContext(ctx, `DELETE FROM canonical_games WHERE id = ?`, canonicalID); err != nil {
		t.Fatalf("delete canonical game: %v", err)
	}
	var favoriteCount int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM canonical_game_favorites WHERE canonical_id = ?`, canonicalID).Scan(&favoriteCount); err != nil {
		t.Fatal(err)
	}
	if favoriteCount != 0 {
		t.Fatalf("favorite row count after canonical delete = %d, want 0", favoriteCount)
	}
}

func TestCanonicalFavoriteAccessIsProfileScoped(t *testing.T) {
	ctx := context.Background()
	profileOneCtx := core.WithProfile(ctx, &core.Profile{ID: "profile-1", Role: core.ProfileRoleAdminPlayer})
	profileTwoCtx := core.WithProfile(ctx, &core.Profile{ID: "profile-2", Role: core.ProfileRolePlayer})
	db, store := newTestGameStore(t)

	persistBatch(t, profileOneCtx, store, makeTestBatch("integration-1", "scan:profile-1-game", "external-1", "Profile One Game", "match-1"))
	persistBatch(t, profileTwoCtx, store, makeTestBatch("integration-2", "scan:profile-2-game", "external-2", "Profile Two Game", "match-2"))

	profileTwoIDs, err := store.GetVisibleCanonicalIDs(profileTwoCtx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(profileTwoIDs) != 1 {
		t.Fatalf("profile two canonical IDs = %+v, want 1", profileTwoIDs)
	}
	profileTwoCanonicalID := profileTwoIDs[0]

	if err := store.SetCanonicalFavorite(profileOneCtx, profileTwoCanonicalID); !errors.Is(err, core.ErrCanonicalGameNotFound) {
		t.Fatalf("SetCanonicalFavorite cross-profile error = %v, want ErrCanonicalGameNotFound", err)
	}
	if err := store.SetCanonicalFavorite(profileTwoCtx, profileTwoCanonicalID); err != nil {
		t.Fatalf("SetCanonicalFavorite profile two: %v", err)
	}
	if err := store.ClearCanonicalFavorite(profileOneCtx, profileTwoCanonicalID); !errors.Is(err, core.ErrCanonicalGameNotFound) {
		t.Fatalf("ClearCanonicalFavorite cross-profile error = %v, want ErrCanonicalGameNotFound", err)
	}
	var favorites int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM canonical_game_favorites WHERE canonical_id = ?`, profileTwoCanonicalID).Scan(&favorites); err != nil {
		t.Fatal(err)
	}
	if favorites != 1 {
		t.Fatalf("profile two favorite count after cross-profile clear = %d, want 1", favorites)
	}
}

func TestDeleteSourceGameByIDKeepsSiblingSourceRecordsAndRecomputesCanonical(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	persistBatch(t, ctx, store, makeTestBatch("integration-1", "scan:source-a", "source-a", "Alpha", "match-alpha"))
	persistBatch(t, ctx, store, makeTestBatch("integration-2", "scan:source-b", "source-b", "Alpha Alt", "match-alpha"))

	if err := store.CacheAchievements(ctx, "scan:source-a", &core.AchievementSet{
		GameID:         "scan:source-a",
		Source:         "metadata-steam",
		ExternalGameID: "match-alpha",
		TotalCount:     1,
		UnlockedCount:  1,
		Achievements: []core.Achievement{{
			ExternalID: "achievement-1",
			Title:      "Done",
			Unlocked:   true,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	canonicalID := canonicalIDForSource(t, ctx, db, "scan:source-a")
	if canonicalIDForSource(t, ctx, db, "scan:source-b") != canonicalID {
		t.Fatal("expected both source games to share one canonical id before delete")
	}

	if err := store.DeleteSourceGameByID(ctx, "scan:source-a"); err != nil {
		t.Fatalf("DeleteSourceGameByID: %v", err)
	}

	if _, err := store.GetCachedAchievements(ctx, "scan:source-a", "metadata-steam"); err != nil {
		t.Fatalf("GetCachedAchievements deleted source: %v", err)
	}

	game, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatalf("GetCanonicalGameByID: %v", err)
	}
	if game == nil {
		t.Fatal("expected canonical game to remain after deleting one sibling source")
	}
	if len(game.SourceGames) != 1 || game.SourceGames[0].ID != "scan:source-b" {
		t.Fatalf("remaining source games = %+v, want only scan:source-b", game.SourceGames)
	}

	var deletedCount int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM source_games WHERE id = ?`, "scan:source-a").Scan(&deletedCount); err != nil {
		t.Fatal(err)
	}
	if deletedCount != 0 {
		t.Fatalf("deleted source row count = %d, want 0", deletedCount)
	}
}

func TestDeleteSourceGameByIDIsProfileScoped(t *testing.T) {
	ctx := context.Background()
	profileOneCtx := core.WithProfile(ctx, &core.Profile{ID: "profile-1", Role: core.ProfileRoleAdminPlayer})
	profileTwoCtx := core.WithProfile(ctx, &core.Profile{ID: "profile-2", Role: core.ProfileRolePlayer})
	_, store := newTestGameStore(t)

	persistBatch(t, profileOneCtx, store, makeTestBatch("integration-1", "scan:profile-1-delete", "external-1", "Profile One Delete", "match-1"))
	persistBatch(t, profileTwoCtx, store, makeTestBatch("integration-2", "scan:profile-2-delete", "external-2", "Profile Two Delete", "match-2"))

	if err := store.DeleteSourceGameByID(profileOneCtx, "scan:profile-2-delete"); !errors.Is(err, core.ErrSourceGameDeleteNotFound) {
		t.Fatalf("DeleteSourceGameByID cross-profile error = %v, want ErrSourceGameDeleteNotFound", err)
	}
	source, err := store.GetManualReviewCandidate(profileTwoCtx, "scan:profile-2-delete")
	if err != nil {
		t.Fatal(err)
	}
	if source == nil {
		t.Fatal("profile two source was deleted by profile one")
	}
}

func TestDeleteSourceGameByIDRemovesSoloCanonicalMembership(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	persistBatch(t, ctx, store, makeTestBatch("integration-1", "scan:solo", "source-solo", "Solo", "match-solo"))
	canonicalID := canonicalIDForSource(t, ctx, db, "scan:solo")

	if err := store.DeleteSourceGameByID(ctx, "scan:solo"); err != nil {
		t.Fatalf("DeleteSourceGameByID: %v", err)
	}

	game, err := store.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		t.Fatalf("GetCanonicalGameByID: %v", err)
	}
	if game != nil {
		t.Fatalf("expected canonical game to disappear after deleting its only source, got %+v", game)
	}

	var linkCount int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM canonical_source_games_link WHERE canonical_id = ?`, canonicalID).Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if linkCount != 0 {
		t.Fatalf("canonical link count = %d, want 0", linkCount)
	}
}

func TestPersistScanResultsHardDeletesOutOfScopeRowsAndSoftDeletesInScopeRows(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	initialBatch := &core.ScanBatch{
		IntegrationID: "drive-1",
		SourceGames: []*core.SourceGame{
			{
				ID:            "sg-games-current",
				IntegrationID: "drive-1",
				PluginID:      "game-source-google-drive",
				ExternalID:    "ext-games-current",
				RawTitle:      "Current Game",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				RootPath:      "Games/Current Game",
				Status:        "found",
			},
			{
				ID:            "sg-games-missing",
				IntegrationID: "drive-1",
				PluginID:      "game-source-google-drive",
				ExternalID:    "ext-games-missing",
				RawTitle:      "Missing In Scope",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				RootPath:      "Games/Missing In Scope",
				Status:        "found",
			},
			{
				ID:            "sg-other",
				IntegrationID: "drive-1",
				PluginID:      "game-source-google-drive",
				ExternalID:    "ext-other",
				RawTitle:      "Outside Scope",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				RootPath:      "Other/Outside Scope",
				Status:        "found",
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
	}
	persistBatch(t, ctx, store, initialBatch)

	refreshBatch := &core.ScanBatch{
		IntegrationID: "drive-1",
		SourceGames: []*core.SourceGame{
			{
				ID:            "sg-games-current",
				IntegrationID: "drive-1",
				PluginID:      "game-source-google-drive",
				ExternalID:    "ext-games-current",
				RawTitle:      "Current Game",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				RootPath:      "Games/Current Game",
				Status:        "found",
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
		FilesystemScope: &core.FilesystemScanScope{
			PluginID: "game-source-google-drive",
			IncludePaths: []core.FilesystemIncludePath{{
				Path:      "Games",
				Recursive: true,
			}},
		},
	}
	persistBatch(t, ctx, store, refreshBatch)

	var status string
	if err := db.GetDB().QueryRowContext(ctx, `SELECT status FROM source_games WHERE id = ?`, "sg-games-missing").Scan(&status); err != nil {
		t.Fatalf("load in-scope row: %v", err)
	}
	if status != "not_found" {
		t.Fatalf("status = %q, want not_found", status)
	}

	var outsideCount int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM source_games WHERE id = ?`, "sg-other").Scan(&outsideCount); err != nil {
		t.Fatalf("count out-of-scope row: %v", err)
	}
	if outsideCount != 0 {
		t.Fatalf("out-of-scope row count = %d, want 0", outsideCount)
	}
}
