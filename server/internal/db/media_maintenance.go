package db

import (
	"context"
	"fmt"
)

// ReclaimAbandonedMedia tidies artwork the downloader gave up on.
//
// "Gave up" was a dead end. Nothing revisited a permanent failure — the retry
// path excludes them by design — so a count of abandoned images only ever grew,
// and the console reported it with no way to act on it. Measured on one
// library: three abandoned assets, two Steam videos that 404 and one IGDB image
// that returned 502.
//
// All three belonged to no game at all. That is the important part: an asset
// nothing references cannot be replaced, because there is no game to ask a
// provider about, and it cannot be shown, because nothing points at it. It is
// simply a row that exists to be counted. Those are removed.
//
// An abandoned asset that a game does reference is a different thing: the game
// is real and wants a picture. Its failure state is cleared so the downloader
// tries once more — a 502 is a bad afternoon, not a verdict — with a cap so a
// URL that is genuinely gone cannot be retried forever.
type MediaReclaimResult struct {
	// OrphansRemoved counts abandoned assets no game referenced.
	OrphansRemoved int
	// Retried counts abandoned assets a game still wants, handed back to the
	// downloader.
	Retried int
	// LeftAlone counts abandoned assets that have been retried enough times
	// that trying again is not useful.
	LeftAlone int
}

// maxAbandonedMediaAttempts is how many times an abandoned image is handed back
// to the downloader before it is left alone. Small, because the failures worth
// retrying are transient ones, and a URL that has 404'd three times is not
// going to start existing.
const maxAbandonedMediaAttempts = 3

func (s *gameStore) ReclaimAbandonedMedia(ctx context.Context) (*MediaReclaimResult, error) {
	db := s.db.GetDB()
	result := &MediaReclaimResult{}

	orphans, err := db.ExecContext(ctx, `
		DELETE FROM media_assets
		WHERE COALESCE(download_permanent_failure, 0) = 1
		  AND NOT EXISTS (SELECT 1 FROM source_game_media m WHERE m.media_asset_id = media_assets.id)`)
	if err != nil {
		return nil, fmt.Errorf("remove abandoned media with no game: %w", err)
	}
	if removed, err := orphans.RowsAffected(); err == nil {
		result.OrphansRemoved = int(removed)
	}

	retried, err := db.ExecContext(ctx, `
		UPDATE media_assets
		SET download_permanent_failure = 0,
		    download_failed_at = NULL,
		    download_last_error = NULL
		WHERE COALESCE(download_permanent_failure, 0) = 1
		  AND COALESCE(download_attempts, 0) < ?`, maxAbandonedMediaAttempts)
	if err != nil {
		return nil, fmt.Errorf("retry abandoned media: %w", err)
	}
	if count, err := retried.RowsAffected(); err == nil {
		result.Retried = int(count)
	}

	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM media_assets
		WHERE COALESCE(download_permanent_failure, 0) = 1`).Scan(&result.LeftAlone); err != nil {
		return nil, fmt.Errorf("count abandoned media: %w", err)
	}
	return result, nil
}
