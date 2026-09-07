package db

import (
	"context"
	"testing"
)

// Artwork the downloader gave up on was a dead end: nothing revisited a
// permanent failure, so the count only grew and the console reported it with
// nothing anyone could do about it.
//
// Measured on one library, all three abandoned assets belonged to no game —
// two Steam videos returning 404 and an IGDB image returning 502. An asset
// nothing references cannot be replaced and cannot be shown; it exists to be
// counted.

func TestAbandonedArtworkNoGameWantsIsRemoved(t *testing.T) {
	ctx := context.Background()
	database, store := newTestGameStore(t)

	seedAsset(t, ctx, database, 1, "https://example.invalid/gone.png", 1, 1)

	result, err := store.ReclaimAbandonedMedia(ctx)
	if err != nil {
		t.Fatalf("ReclaimAbandonedMedia() error = %v", err)
	}
	if result.OrphansRemoved != 1 {
		t.Fatalf("orphans removed = %d, want 1", result.OrphansRemoved)
	}

	var remaining int
	if err := database.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM media_assets WHERE id=1`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatal("an abandoned asset no game referenced was kept")
	}
}

func TestAbandonedArtworkAGameStillWantsIsRetried(t *testing.T) {
	// The game is real and wants a picture. A 502 is a bad afternoon, not a
	// verdict, and nothing else was ever going to try again.
	ctx := context.Background()
	database, store := newTestGameStore(t)

	seedSourceGameForMedia(t, ctx, database, "scan:wants-art")
	seedAsset(t, ctx, database, 2, "https://example.invalid/flaky.png", 1, 1)
	linkAsset(t, ctx, database, "scan:wants-art", 2, "cover")

	result, err := store.ReclaimAbandonedMedia(ctx)
	if err != nil {
		t.Fatalf("ReclaimAbandonedMedia() error = %v", err)
	}
	if result.OrphansRemoved != 0 {
		t.Fatalf("orphans removed = %d, want 0 — a referenced asset was deleted", result.OrphansRemoved)
	}
	if result.Retried != 1 {
		t.Fatalf("retried = %d, want 1", result.Retried)
	}

	var permanent int
	if err := database.GetDB().QueryRowContext(ctx,
		`SELECT COALESCE(download_permanent_failure,0) FROM media_assets WHERE id=2`).Scan(&permanent); err != nil {
		t.Fatal(err)
	}
	if permanent != 0 {
		t.Fatal("a referenced asset was left abandoned")
	}
}

func TestArtworkRetriedEnoughTimesIsLeftAlone(t *testing.T) {
	// A URL that has failed this often is not going to start existing, and
	// retrying it every scan would be work with a known outcome.
	ctx := context.Background()
	database, store := newTestGameStore(t)

	seedSourceGameForMedia(t, ctx, database, "scan:hopeless")
	seedAsset(t, ctx, database, 3, "https://example.invalid/never.png", maxAbandonedMediaAttempts, 1)
	linkAsset(t, ctx, database, "scan:hopeless", 3, "cover")

	result, err := store.ReclaimAbandonedMedia(ctx)
	if err != nil {
		t.Fatalf("ReclaimAbandonedMedia() error = %v", err)
	}
	if result.Retried != 0 {
		t.Fatalf("retried = %d, want 0 for an asset already tried %d times", result.Retried, maxAbandonedMediaAttempts)
	}
	if result.LeftAlone != 1 {
		t.Fatalf("left alone = %d, want 1", result.LeftAlone)
	}
}

func TestReclaimLeavesHealthyArtworkAlone(t *testing.T) {
	ctx := context.Background()
	database, store := newTestGameStore(t)

	seedSourceGameForMedia(t, ctx, database, "scan:fine")
	seedAsset(t, ctx, database, 4, "https://example.invalid/good.png", 0, 0)
	linkAsset(t, ctx, database, "scan:fine", 4, "cover")

	result, err := store.ReclaimAbandonedMedia(ctx)
	if err != nil {
		t.Fatalf("ReclaimAbandonedMedia() error = %v", err)
	}
	if result.OrphansRemoved != 0 || result.Retried != 0 {
		t.Fatalf("result = %+v, want nothing touched for a healthy asset", result)
	}

	var present int
	if err := database.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM media_assets WHERE id=4`).Scan(&present); err != nil {
		t.Fatal(err)
	}
	if present != 1 {
		t.Fatal("a healthy asset was removed")
	}
}

func seedAsset(t *testing.T, ctx context.Context, database *sqliteDatabase, id int, url string, attempts, permanent int) {
	t.Helper()
	if _, err := database.GetDB().ExecContext(ctx,
		`INSERT INTO media_assets (id, url, download_attempts, download_permanent_failure, download_last_error)
		 VALUES (?, ?, ?, ?, 'unexpected status 404')`, id, url, attempts, permanent); err != nil {
		t.Fatal(err)
	}
}

func seedSourceGameForMedia(t *testing.T, ctx context.Context, database *sqliteDatabase, id string) {
	t.Helper()
	if _, err := database.GetDB().ExecContext(ctx,
		`INSERT INTO source_games (id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, created_at)
		 VALUES (?, 'integration-1', 'game-source-local', ?, 'Art Wanted', 'windows_pc', 'base_game', 'self_contained', 'found', 1700000000)`,
		id, id); err != nil {
		t.Fatal(err)
	}
}

func linkAsset(t *testing.T, ctx context.Context, database *sqliteDatabase, sourceGameID string, assetID int, mediaType string) {
	t.Helper()
	if _, err := database.GetDB().ExecContext(ctx,
		`INSERT INTO source_game_media (source_game_id, media_asset_id, type) VALUES (?, ?, ?)`,
		sourceGameID, assetID, mediaType); err != nil {
		t.Fatal(err)
	}
}
