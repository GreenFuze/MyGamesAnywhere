package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
	"github.com/google/uuid"
)

type gameStore struct {
	db     core.Database
	logger core.Logger
}

func NewGameStore(db core.Database, logger core.Logger) core.GameStore {
	return &gameStore{db: db, logger: logger}
}

// ── Writes ──────────────────────────────────────────────────────────

func (s *gameStore) PersistScanResults(ctx context.Context, batch *core.ScanBatch) error {
	if err := batch.Validate(); err != nil {
		return fmt.Errorf("invalid scan batch: %w", err)
	}
	batch = s.normalizeDuplicateSourceGames(batch)

	db := s.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	// 1. Load existing source games for this integration to detect moves.
	existing, err := s.loadExistingSourceGames(ctx, tx, batch.IntegrationID)
	if err != nil {
		return fmt.Errorf("load existing: %w", err)
	}
	existingByID := make(map[string]existingSourceGame, len(existing))
	existingByNaturalKey := make(map[string]existingSourceGame, len(existing))
	for _, item := range existing {
		existingByID[item.ID] = item
		existingByNaturalKey[sourceGameNaturalKey(item.PluginID, item.ExternalID)] = item
	}

	seenIDs := make(map[string]bool, len(batch.SourceGames))
	sourceGamesByID := make(map[string]*core.SourceGame, len(batch.SourceGames))
	persistedSourceIDs := make(map[string]string, len(batch.SourceGames))

	// 2. Upsert source games.
	for _, sg := range batch.SourceGames {
		batchSourceID := sg.ID
		naturalKey := sourceGameNaturalKey(sg.PluginID, sg.ExternalID)

		persistedID := batchSourceID
		existingRecord := existingByID[batchSourceID]
		if item, ok := existingByNaturalKey[naturalKey]; ok {
			persistedID = item.ID
			existingRecord = item
		}

		persistedSG := *sg
		persistedSG.ID = persistedID

		seenIDs[persistedID] = true
		persistedSourceIDs[batchSourceID] = persistedID
		sourceGamesByID[persistedID] = &persistedSG

		// Detect move: same integration, a not_found game shares the same raw_title+platform.
		if sg.RootPath != "" {
			for _, ex := range existing {
				if ex.Status == "not_found" && ex.RawTitle == sg.RawTitle &&
					ex.Platform == string(sg.Platform) && ex.ID != persistedID {
					s.logger.Info("detected game move",
						"old_id", ex.ID, "new_id", persistedID,
						"old_path", ex.RootPath, "new_path", sg.RootPath)
					// Soft-delete the old record and continue with the new one.
					tx.ExecContext(ctx, `UPDATE source_games SET status='replaced' WHERE id=?`, ex.ID)
				}
			}
		}

		var lastSeen int64
		if sg.LastSeenAt != nil {
			lastSeen = sg.LastSeenAt.Unix()
		} else {
			lastSeen = now
		}
		finalReviewState, finalManualReviewJSON, err := resolveManualReviewPersistence(&persistedSG, existingRecord)
		if err != nil {
			return fmt.Errorf("resolve manual review persistence for %s: %w", persistedID, err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO source_games
			(id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, root_path, url, status, review_state, manual_review_json, last_seen_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'found', ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				raw_title = excluded.raw_title,
				platform = excluded.platform,
				kind = excluded.kind,
				group_kind = excluded.group_kind,
				root_path = excluded.root_path,
				url = excluded.url,
				status = 'found',
				review_state = excluded.review_state,
				manual_review_json = excluded.manual_review_json,
				last_seen_at = excluded.last_seen_at`,
			persistedID, sg.IntegrationID, sg.PluginID, sg.ExternalID,
			sg.RawTitle, string(sg.Platform), string(sg.Kind), string(sg.GroupKind),
			nullEmpty(sg.RootPath), nullEmpty(sg.URL), string(finalReviewState), nullEmpty(finalManualReviewJSON), lastSeen, now)
		if err != nil {
			return fmt.Errorf("upsert source game %s: %w", persistedID, err)
		}

		// 3. Replace files for this source game.
		if _, err := tx.ExecContext(ctx, `DELETE FROM game_files WHERE source_game_id=?`, persistedID); err != nil {
			return fmt.Errorf("delete files for %s: %w", persistedID, err)
		}
		for _, f := range sg.Files {
			isDir := 0
			if f.IsDir {
				isDir = 1
			}
			var modifiedAt any
			if f.ModifiedAt != nil {
				modifiedAt = f.ModifiedAt.Unix()
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO game_files
				(source_game_id, path, file_name, role, file_kind, size, is_dir, object_id, revision, modified_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(source_game_id, path) DO UPDATE SET
					file_name = excluded.file_name,
					role = excluded.role,
					file_kind = excluded.file_kind,
					size = excluded.size,
					is_dir = excluded.is_dir,
					object_id = excluded.object_id,
					revision = excluded.revision,
					modified_at = excluded.modified_at`,
				persistedID, f.Path, f.FileName, string(f.Role), nullEmpty(f.FileKind), f.Size, isDir, nullEmpty(f.ObjectID), nullEmpty(f.Revision), modifiedAt)
			if err != nil {
				return fmt.Errorf("insert file for %s: %w", persistedID, err)
			}
		}
	}

	// 4. Insert resolver matches.
	for batchSourceID, matches := range batch.ResolverMatches {
		persistedID := persistedSourceIDs[batchSourceID]
		if persistedID == "" || !seenIDs[persistedID] {
			continue
		}
		processedMatches, err := applyPersistedManualReview(sourceGamesByID[persistedID], matches, existingByID[persistedID])
		if err != nil {
			return fmt.Errorf("apply manual review for %s: %w", persistedID, err)
		}
		// Clear old matches for this source game.
		if _, err := tx.ExecContext(ctx, `DELETE FROM metadata_resolver_matches WHERE source_game_id=?`, persistedID); err != nil {
			return fmt.Errorf("delete matches for %s: %w", persistedID, err)
		}
		for _, m := range processedMatches {
			metaJSON, _ := buildMetadataJSON(m)
			_, err := tx.ExecContext(ctx, `INSERT INTO metadata_resolver_matches
				(source_game_id, plugin_id, external_id, title, platform, url, outvoted, manual_selection,
				 developer, publisher, release_date, rating, metadata_json, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				persistedID, m.PluginID, m.ExternalID,
				nullEmpty(m.Title), nullEmpty(m.Platform), nullEmpty(m.URL),
				boolToInt(m.Outvoted), boolToInt(m.ManualSelection),
				nullEmpty(m.Developer), nullEmpty(m.Publisher), nullEmpty(m.ReleaseDate),
				m.Rating, nullEmpty(metaJSON), now)
			if err != nil {
				return fmt.Errorf("insert match for %s/%s: %w", persistedID, m.PluginID, err)
			}
		}
	}

	// 5. Upsert media assets + link to source games.
	for batchSourceID, refs := range batch.MediaItems {
		persistedID := persistedSourceIDs[batchSourceID]
		if persistedID == "" || !seenIDs[persistedID] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM source_game_media WHERE source_game_id=?`, persistedID); err != nil {
			return fmt.Errorf("delete media links for %s: %w", persistedID, err)
		}
		for _, ref := range refs {
			if ref.URL == "" {
				continue
			}
			// Upsert into global media_assets by URL.
			_, err := tx.ExecContext(ctx, `INSERT INTO media_assets (url, width, height, mime_type)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(url) DO UPDATE SET
					width = COALESCE(NULLIF(excluded.width,0), media_assets.width),
					height = COALESCE(NULLIF(excluded.height,0), media_assets.height)`,
				ref.URL, ref.Width, ref.Height, nullEmpty(""))
			if err != nil {
				return fmt.Errorf("upsert media asset %s: %w", ref.URL, err)
			}

			// Get the asset ID.
			var assetID int
			err = tx.QueryRowContext(ctx, `SELECT id FROM media_assets WHERE url=?`, ref.URL).Scan(&assetID)
			if err != nil {
				return fmt.Errorf("get media asset id: %w", err)
			}

			_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO source_game_media
				(source_game_id, media_asset_id, type, source) VALUES (?, ?, ?, ?)`,
				persistedID, assetID, string(ref.Type), nullEmpty(ref.Source))
			if err != nil {
				return fmt.Errorf("link media for %s: %w", persistedID, err)
			}
		}
	}

	// 6. Reconcile source games from this integration not seen in complete scan batches.
	if !batch.SkipMissingReconcile {
		if err := s.reconcileMissingSourceGames(ctx, tx, batch.IntegrationID, seenIDs, batch.FilesystemScope); err != nil {
			return fmt.Errorf("reconcile missing source games: %w", err)
		}
	}

	// 7. Recompute canonical groupings.
	if err := s.recomputeCanonicalGroups(ctx, tx); err != nil {
		return fmt.Errorf("recompute canonical: %w", err)
	}

	return tx.Commit()
}

func (s *gameStore) CacheAchievements(ctx context.Context, sourceGameID string, set *core.AchievementSet) error {
	if sourceGameID == "" || set == nil {
		return fmt.Errorf("sourceGameID and set are required")
	}

	db := s.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete existing set + achievements for this (source_game, source).
	var oldSetID sql.NullInt64
	tx.QueryRowContext(ctx, `SELECT id FROM achievement_sets WHERE source_game_id=? AND source=?`,
		sourceGameID, set.Source).Scan(&oldSetID)
	if oldSetID.Valid {
		tx.ExecContext(ctx, `DELETE FROM achievements WHERE set_id=?`, oldSetID.Int64)
		tx.ExecContext(ctx, `DELETE FROM achievement_sets WHERE id=?`, oldSetID.Int64)
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO achievement_sets
		(source_game_id, source, external_game_id, total_count, unlocked_count, total_points, earned_points, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sourceGameID, set.Source, set.ExternalGameID,
		set.TotalCount, set.UnlockedCount, set.TotalPoints, set.EarnedPoints,
		set.FetchedAt.Unix())
	if err != nil {
		return fmt.Errorf("insert achievement set: %w", err)
	}
	setID, _ := res.LastInsertId()

	for _, a := range set.Achievements {
		var unlockedAt *int64
		if !a.UnlockedAt.IsZero() {
			u := a.UnlockedAt.Unix()
			unlockedAt = &u
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO achievements
			(set_id, external_id, title, description, locked_icon, unlocked_icon, points, rarity, unlocked, unlocked_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			setID, a.ExternalID, a.Title, a.Description,
			nullEmpty(a.LockedIcon), nullEmpty(a.UnlockedIcon),
			a.Points, a.Rarity, boolToInt(a.Unlocked), unlockedAt)
		if err != nil {
			return fmt.Errorf("insert achievement: %w", err)
		}
	}

	return tx.Commit()
}

func (s *gameStore) SetCanonicalCoverOverride(ctx context.Context, canonicalID string, mediaAssetID int) error {
	if strings.TrimSpace(canonicalID) == "" {
		return core.ErrCanonicalGameNotFound
	}
	if mediaAssetID <= 0 {
		return core.ErrCoverOverrideMediaNotFound
	}
	db := s.db.GetDB()
	var linked int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM canonical_source_games_link l
		JOIN source_game_media m ON m.source_game_id = l.source_game_id
		JOIN source_games sg ON sg.id = l.source_game_id
		WHERE l.canonical_id = ?
		  AND m.media_asset_id = ?
		  AND `+visibleSourceGameWhere("sg"), canonicalID, mediaAssetID).Scan(&linked); err != nil {
		return err
	}
	if linked == 0 {
		exists, err := s.canonicalGameExists(ctx, canonicalID)
		if err != nil {
			return err
		}
		if !exists {
			return core.ErrCanonicalGameNotFound
		}
		return core.ErrCoverOverrideMediaNotFound
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO canonical_game_cover_overrides (canonical_id, media_asset_id, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(canonical_id) DO UPDATE SET media_asset_id=excluded.media_asset_id, updated_at=excluded.updated_at`,
		canonicalID, mediaAssetID, time.Now().Unix())
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `DELETE FROM canonical_game_cover_override_clears WHERE canonical_id=?`, canonicalID)
	return err
}

func (s *gameStore) ClearCanonicalCoverOverride(ctx context.Context, canonicalID string) error {
	if strings.TrimSpace(canonicalID) == "" {
		return core.ErrCanonicalGameNotFound
	}
	db := s.db.GetDB()
	res, err := db.ExecContext(ctx, `DELETE FROM canonical_game_cover_overrides WHERE canonical_id=?`, canonicalID)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		exists, err := s.canonicalGameExists(ctx, canonicalID)
		if err != nil {
			return err
		}
		if !exists {
			return core.ErrCanonicalGameNotFound
		}
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO canonical_game_cover_override_clears (canonical_id, cleared_at)
		VALUES (?, ?)
		ON CONFLICT(canonical_id) DO UPDATE SET cleared_at=excluded.cleared_at`,
		canonicalID, time.Now().Unix())
	return err
}

func (s *gameStore) SetCanonicalHoverOverride(ctx context.Context, canonicalID string, mediaAssetID int) error {
	if strings.TrimSpace(canonicalID) == "" {
		return core.ErrCanonicalGameNotFound
	}
	if mediaAssetID <= 0 {
		return core.ErrHoverOverrideMediaNotFound
	}
	db := s.db.GetDB()
	var linked int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM canonical_source_games_link l
		JOIN source_game_media m ON m.source_game_id = l.source_game_id
		JOIN source_games sg ON sg.id = l.source_game_id
		WHERE l.canonical_id = ?
		  AND m.media_asset_id = ?
		  AND `+visibleSourceGameWhere("sg"), canonicalID, mediaAssetID).Scan(&linked); err != nil {
		return err
	}
	if linked == 0 {
		exists, err := s.canonicalGameExists(ctx, canonicalID)
		if err != nil {
			return err
		}
		if !exists {
			return core.ErrCanonicalGameNotFound
		}
		return core.ErrHoverOverrideMediaNotFound
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO canonical_game_hover_overrides (canonical_id, media_asset_id, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(canonical_id) DO UPDATE SET media_asset_id=excluded.media_asset_id, updated_at=excluded.updated_at`,
		canonicalID, mediaAssetID, time.Now().Unix())
	return err
}

func (s *gameStore) SetCanonicalBackgroundOverride(ctx context.Context, canonicalID string, mediaAssetID int) error {
	if strings.TrimSpace(canonicalID) == "" {
		return core.ErrCanonicalGameNotFound
	}
	if mediaAssetID <= 0 {
		return core.ErrBackgroundOverrideMediaNotFound
	}
	db := s.db.GetDB()
	var linked int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM canonical_source_games_link l
		JOIN source_game_media m ON m.source_game_id = l.source_game_id
		JOIN source_games sg ON sg.id = l.source_game_id
		WHERE l.canonical_id = ?
		  AND m.media_asset_id = ?
		  AND `+visibleSourceGameWhere("sg"), canonicalID, mediaAssetID).Scan(&linked); err != nil {
		return err
	}
	if linked == 0 {
		exists, err := s.canonicalGameExists(ctx, canonicalID)
		if err != nil {
			return err
		}
		if !exists {
			return core.ErrCanonicalGameNotFound
		}
		return core.ErrBackgroundOverrideMediaNotFound
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO canonical_game_background_overrides (canonical_id, media_asset_id, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(canonical_id) DO UPDATE SET media_asset_id=excluded.media_asset_id, updated_at=excluded.updated_at`,
		canonicalID, mediaAssetID, time.Now().Unix())
	return err
}

func (s *gameStore) UpdateMediaAsset(ctx context.Context, assetID int, localPath, hash string) error {
	if assetID <= 0 {
		return fmt.Errorf("assetID must be positive")
	}
	_, err := s.db.GetDB().ExecContext(ctx,
		`UPDATE media_assets SET local_path=?, hash=? WHERE id=?`,
		localPath, hash, assetID)
	return err
}

func (s *gameStore) UpdateMediaAssetMetadata(ctx context.Context, assetID, width, height int, mimeType string) error {
	if assetID <= 0 {
		return fmt.Errorf("assetID must be positive")
	}
	_, err := s.db.GetDB().ExecContext(ctx, `
		UPDATE media_assets
		SET width = COALESCE(NULLIF(?, 0), width),
		    height = COALESCE(NULLIF(?, 0), height),
		    mime_type = COALESCE(NULLIF(?, ''), mime_type)
		WHERE id = ?`,
		width, height, strings.TrimSpace(mimeType), assetID,
	)
	return err
}

func (s *gameStore) DeleteAllGames(ctx context.Context) error {
	db := s.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tables := []string{
		"achievements", "achievement_sets",
		"canonical_game_cover_overrides",
		"canonical_game_hover_overrides",
		"canonical_game_background_overrides",
		"source_game_media", "media_assets",
		"metadata_resolver_matches",
		"game_files",
		"canonical_source_games_link",
		"source_games",
	}
	for _, t := range tables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+t); err != nil {
			return fmt.Errorf("delete %s: %w", t, err)
		}
	}
	return tx.Commit()
}

// ── Reads ───────────────────────────────────────────────────────────

func (s *gameStore) GetCanonicalGames(ctx context.Context) ([]*core.CanonicalGame, error) {
	ids, err := s.GetVisibleCanonicalIDs(ctx, 0, -1)
	if err != nil {
		return nil, err
	}
	return s.GetCanonicalGamesByIDs(ctx, ids)
}

func (s *gameStore) GetCanonicalGamesByIDs(ctx context.Context, ids []string) ([]*core.CanonicalGame, error) {
	return s.canonicalGamesForIDs(ctx, ids)
}

func (s *gameStore) CountVisibleCanonicalGames(ctx context.Context) (int, error) {
	var n int
	err := s.db.GetDB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT canonical_id FROM canonical_source_games_link l
			WHERE EXISTS (
				SELECT 1 FROM source_games sg
				WHERE sg.id = l.source_game_id AND `+visibleSourceGameWhere("sg")+`
			)
			GROUP BY canonical_id
		)`).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *gameStore) canonicalGameExists(ctx context.Context, canonicalID string) (bool, error) {
	var n int
	err := s.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(1) FROM canonical_games WHERE id=?`, canonicalID).Scan(&n)
	return n > 0, err
}

// GetVisibleCanonicalIDs returns canonical IDs that have at least one found source game,
// ordered by canonical_id. limit <= 0 means no upper bound (SQLite: LIMIT -1).
func (s *gameStore) GetVisibleCanonicalIDs(ctx context.Context, offset, limit int) ([]string, error) {
	if offset < 0 {
		offset = 0
	}
	db := s.db.GetDB()
	q := `
		SELECT canonical_id FROM canonical_source_games_link l
		WHERE EXISTS (
			SELECT 1 FROM source_games sg
			WHERE sg.id = l.source_game_id AND ` + visibleSourceGameWhere("sg") + `
		)
		GROUP BY canonical_id
		ORDER BY canonical_id`
	var args []any
	switch {
	case limit > 0:
		q += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	case offset > 0:
		q += " LIMIT -1 OFFSET ?"
		args = append(args, offset)
	}

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		ids = append(ids, cid)
	}
	return ids, rows.Err()
}

func (s *gameStore) canonicalGamesForIDs(ctx context.Context, ids []string) ([]*core.CanonicalGame, error) {
	db := s.db.GetDB()
	var result []*core.CanonicalGame
	for _, cid := range ids {
		rows, err := db.QueryContext(ctx, `SELECT source_game_id FROM canonical_source_games_link WHERE canonical_id=?`, cid)
		if err != nil {
			return nil, err
		}
		var sgIDs []string
		for rows.Next() {
			var sgid string
			if err := rows.Scan(&sgid); err != nil {
				rows.Close()
				return nil, err
			}
			sgIDs = append(sgIDs, sgid)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		if len(sgIDs) == 0 {
			continue
		}
		cg, err := s.buildCanonicalGame(ctx, db, cid, sgIDs)
		if err != nil {
			s.logger.Error("build canonical game", err, "canonical_id", cid)
			continue
		}
		if cg != nil {
			result = append(result, cg)
		}
	}
	return result, nil
}

func (s *gameStore) GetMediaAssetByID(ctx context.Context, id int) (*core.MediaAsset, error) {
	if id <= 0 {
		return nil, nil
	}
	row := s.db.GetDB().QueryRowContext(ctx,
		`SELECT id, url, local_path, hash, width, height, mime_type FROM media_assets WHERE id=?`, id)
	var a core.MediaAsset
	var lp, h, mt sql.NullString
	if err := row.Scan(&a.ID, &a.URL, &lp, &h, &a.Width, &a.Height, &mt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	a.LocalPath = lp.String
	a.Hash = h.String
	a.MimeType = mt.String
	return &a, nil
}

func (s *gameStore) GetCanonicalGameByID(ctx context.Context, canonicalID string) (*core.CanonicalGame, error) {
	db := s.db.GetDB()
	rows, err := db.QueryContext(ctx, `SELECT source_game_id FROM canonical_source_games_link WHERE canonical_id=?`, canonicalID)
	if err != nil {
		return nil, err
	}
	var sgIDs []string
	for rows.Next() {
		var sgid string
		if err := rows.Scan(&sgid); err != nil {
			rows.Close()
			return nil, err
		}
		sgIDs = append(sgIDs, sgid)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(sgIDs) == 0 {
		return nil, nil
	}
	return s.buildCanonicalGame(ctx, db, canonicalID, sgIDs)
}

func (s *gameStore) GetSourceGamesForCanonical(ctx context.Context, canonicalID string) ([]*core.SourceGame, error) {
	db := s.db.GetDB()
	rows, err := db.QueryContext(ctx, `SELECT source_game_id FROM canonical_source_games_link WHERE canonical_id=?`, canonicalID)
	if err != nil {
		return nil, err
	}

	var sgIDs []string
	for rows.Next() {
		var sgid string
		if err := rows.Scan(&sgid); err != nil {
			rows.Close()
			return nil, err
		}
		sgIDs = append(sgIDs, sgid)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	var out []*core.SourceGame
	for _, sgid := range sgIDs {
		sg, err := s.loadSourceGame(ctx, db, sgid)
		if err != nil {
			return nil, err
		}
		if isVisibleSourceGame(sg) {
			out = append(out, sg)
		}
	}
	return out, nil
}

func (s *gameStore) GetPendingMediaDownloads(ctx context.Context, limit int) ([]*core.MediaAsset, error) {
	rows, err := s.db.GetDB().QueryContext(ctx,
		`SELECT id, url, local_path, hash, width, height, mime_type FROM media_assets WHERE local_path IS NULL OR local_path='' LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*core.MediaAsset
	for rows.Next() {
		var a core.MediaAsset
		var lp, h, mt sql.NullString
		if err := rows.Scan(&a.ID, &a.URL, &lp, &h, &a.Width, &a.Height, &mt); err != nil {
			return nil, err
		}
		a.LocalPath = lp.String
		a.Hash = h.String
		a.MimeType = mt.String
		out = append(out, &a)
	}
	return out, nil
}

func (s *gameStore) GetCachedAchievements(ctx context.Context, sourceGameID, source string) (*core.AchievementSet, error) {
	db := s.db.GetDB()
	var setID int
	var fetchedAt int64
	var set core.AchievementSet
	err := db.QueryRowContext(ctx,
		`SELECT id, source_game_id, source, external_game_id, total_count, unlocked_count, total_points, earned_points, fetched_at
		 FROM achievement_sets WHERE source_game_id=? AND source=?`, sourceGameID, source).Scan(
		&setID, &set.GameID, &set.Source, &set.ExternalGameID,
		&set.TotalCount, &set.UnlockedCount, &set.TotalPoints, &set.EarnedPoints, &fetchedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	set.FetchedAt = time.Unix(fetchedAt, 0)

	rows, err := db.QueryContext(ctx,
		`SELECT external_id, title, description, locked_icon, unlocked_icon, points, rarity, unlocked, unlocked_at
		 FROM achievements WHERE set_id=?`, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var a core.Achievement
		var li, ui sql.NullString
		var unlockedAt sql.NullInt64
		if err := rows.Scan(&a.ExternalID, &a.Title, &a.Description, &li, &ui,
			&a.Points, &a.Rarity, &a.Unlocked, &unlockedAt); err != nil {
			return nil, err
		}
		a.LockedIcon = li.String
		a.UnlockedIcon = ui.String
		if unlockedAt.Valid && unlockedAt.Int64 > 0 {
			a.UnlockedAt = time.Unix(unlockedAt.Int64, 0)
		}
		set.Achievements = append(set.Achievements, a)
	}
	return &set, nil
}

func (s *gameStore) GetExternalIDsForCanonical(ctx context.Context, canonicalID string) ([]core.ExternalID, error) {
	db := s.db.GetDB()
	// Gather from source_games themselves.
	rows, err := db.QueryContext(ctx, `SELECT sg.plugin_id, sg.external_id, sg.url
		FROM source_games sg
		JOIN canonical_source_games_link l ON l.source_game_id = sg.id
		WHERE l.canonical_id=? AND `+visibleSourceGameWhere("sg"), canonicalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	var out []core.ExternalID
	for rows.Next() {
		var eid core.ExternalID
		var u sql.NullString
		if err := rows.Scan(&eid.Source, &eid.ExternalID, &u); err != nil {
			return nil, err
		}
		eid.URL = u.String
		key := eid.Source + "|" + eid.ExternalID
		if !seen[key] {
			seen[key] = true
			out = append(out, eid)
		}
	}

	// Gather from resolver matches.
	rows2, err := db.QueryContext(ctx, `SELECT m.plugin_id, m.external_id, m.url
		FROM metadata_resolver_matches m
		JOIN canonical_source_games_link l ON l.source_game_id = m.source_game_id
		JOIN source_games sg ON sg.id = m.source_game_id AND `+visibleSourceGameWhere("sg")+`
		WHERE l.canonical_id=? AND m.outvoted=0`, canonicalID)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	for rows2.Next() {
		var eid core.ExternalID
		var u sql.NullString
		if err := rows2.Scan(&eid.Source, &eid.ExternalID, &u); err != nil {
			return nil, err
		}
		eid.URL = u.String
		key := eid.Source + "|" + eid.ExternalID
		if !seen[key] {
			seen[key] = true
			out = append(out, eid)
		}
	}
	return out, nil
}

func (s *gameStore) GetLibraryStats(ctx context.Context) (*core.LibraryStats, error) {
	db := s.db.GetDB()
	out := &core.LibraryStats{
		ByPlatform:         make(map[string]int),
		ByDecade:           make(map[string]int),
		ByKind:             make(map[string]int),
		TopGenres:          make(map[string]int),
		ByIntegrationID:    make(map[string]int),
		ByPluginID:         make(map[string]int),
		ByMetadataPluginID: make(map[string]int),
	}

	if err := db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT l.canonical_id) FROM canonical_source_games_link l
		WHERE EXISTS (SELECT 1 FROM source_games sg WHERE sg.id = l.source_game_id AND `+visibleSourceGameWhere("sg")+`)`).Scan(&out.CanonicalGameCount); err != nil {
		return nil, err
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM source_games WHERE `+visibleSourceGameWhere("source_games")).Scan(&out.SourceGameFoundCount); err != nil {
		return nil, err
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM source_games`).Scan(&out.SourceGameTotalCount); err != nil {
		return nil, err
	}

	if err := scanGroupCounts(ctx, db, `SELECT platform, COUNT(*) FROM source_games WHERE `+visibleSourceGameWhere("source_games")+` GROUP BY platform`, out.ByPlatform); err != nil {
		return nil, err
	}
	if err := scanGroupCounts(ctx, db, `SELECT kind, COUNT(*) FROM source_games WHERE `+visibleSourceGameWhere("source_games")+` GROUP BY kind`, out.ByKind); err != nil {
		return nil, err
	}
	if err := scanGroupCounts(ctx, db, `SELECT integration_id, COUNT(*) FROM source_games WHERE `+visibleSourceGameWhere("source_games")+` GROUP BY integration_id`, out.ByIntegrationID); err != nil {
		return nil, err
	}
	if err := scanGroupCounts(ctx, db, `SELECT plugin_id, COUNT(*) FROM source_games WHERE `+visibleSourceGameWhere("source_games")+` GROUP BY plugin_id`, out.ByPluginID); err != nil {
		return nil, err
	}

	// Per-metadata-plugin enrichment counts (non-outvoted resolver matches).
	if err := scanGroupCounts(ctx, db,
		`SELECT m.plugin_id, COUNT(DISTINCT m.source_game_id)
		 FROM metadata_resolver_matches m
		 JOIN source_games sg ON sg.id = m.source_game_id AND `+visibleSourceGameWhere("sg")+`
		 WHERE m.outvoted = 0
		 GROUP BY m.plugin_id`, out.ByMetadataPluginID); err != nil {
		return nil, err
	}

	q := `SELECT COUNT(DISTINCT l.canonical_id) FROM canonical_source_games_link l
		JOIN source_games sg ON sg.id = l.source_game_id AND ` + visibleSourceGameWhere("sg") + `
		JOIN metadata_resolver_matches m ON m.source_game_id = l.source_game_id
		WHERE m.outvoted=0 AND IFNULL(m.title,'')!=''`
	if err := db.QueryRowContext(ctx, q).Scan(&out.CanonicalWithResolverTitle); err != nil {
		return nil, err
	}
	if out.CanonicalGameCount > 0 {
		out.PercentWithResolverTitle = float64(out.CanonicalWithResolverTitle) / float64(out.CanonicalGameCount) * 100
	}

	games, err := s.GetCanonicalGames(ctx)
	if err != nil {
		return nil, err
	}
	for _, game := range games {
		if game == nil {
			continue
		}
		if strings.TrimSpace(game.Description) != "" {
			out.GamesWithDescription++
		}
		if len(game.Media) > 0 {
			out.GamesWithMedia++
		}
		if game.AchievementSummary != nil && game.AchievementSummary.SourceCount > 0 {
			out.GamesWithAchievements++
		}
		if decade := decadeLabel(game.ReleaseDate); decade != "" {
			out.ByDecade[decade]++
		}
		for _, genre := range game.Genres {
			genre = strings.TrimSpace(genre)
			if genre == "" {
				continue
			}
			out.TopGenres[genre]++
		}
	}
	if out.CanonicalGameCount > 0 {
		out.PercentWithDescription = float64(out.GamesWithDescription) / float64(out.CanonicalGameCount) * 100
		out.PercentWithMedia = float64(out.GamesWithMedia) / float64(out.CanonicalGameCount) * 100
		out.PercentWithAchievements = float64(out.GamesWithAchievements) / float64(out.CanonicalGameCount) * 100
	}
	return out, nil
}

type cachedAchievementDashboardRow struct {
	canonicalID   string
	source        string
	externalID    string
	totalCount    int
	unlockedCount int
	totalPoints   int
	earnedPoints  int
}

type cachedAchievementExplorerSet struct {
	setID         int64
	canonicalID   string
	sourceGameID  string
	source        string
	externalID    string
	totalCount    int
	unlockedCount int
	totalPoints   int
	earnedPoints  int
	fetchedAt     int64
}

func (s *gameStore) GetCachedAchievementsDashboard(ctx context.Context) (*core.CachedAchievementsDashboard, error) {
	games, err := s.GetCanonicalGames(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.GetDB().QueryContext(ctx, `
		SELECT l.canonical_id, a.source, a.external_game_id, a.total_count, a.unlocked_count,
		       COALESCE(a.total_points, 0), COALESCE(a.earned_points, 0)
		FROM achievement_sets a
		JOIN canonical_source_games_link l ON l.source_game_id = a.source_game_id
		JOIN source_games sg ON sg.id = l.source_game_id
		WHERE `+visibleSourceGameWhere("sg")+`
		ORDER BY l.canonical_id, a.source, a.fetched_at DESC, a.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byGame := make(map[string][]cachedAchievementDashboardRow)
	seenGameSet := make(map[string]bool)
	for rows.Next() {
		var item cachedAchievementDashboardRow
		if err := rows.Scan(&item.canonicalID, &item.source, &item.externalID, &item.totalCount, &item.unlockedCount, &item.totalPoints, &item.earnedPoints); err != nil {
			return nil, err
		}
		key := item.canonicalID + "|" + item.source + "|" + item.externalID
		if seenGameSet[key] {
			continue
		}
		seenGameSet[key] = true
		byGame[item.canonicalID] = append(byGame[item.canonicalID], item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := &core.CachedAchievementsDashboard{}
	systemTotals := make(map[string]*core.CachedAchievementSystemSummary)
	systemGames := make(map[string]map[string]bool)
	for _, game := range games {
		if game == nil || game.AchievementSummary == nil {
			continue
		}
		items := byGame[game.ID]
		if len(items) == 0 {
			continue
		}
		gameSystems := aggregateAchievementRows(items)
		out.Games = append(out.Games, core.CachedAchievementGameSummary{
			Game:    game,
			Systems: gameSystems,
		})
		out.Totals.TotalCount += game.AchievementSummary.TotalCount
		out.Totals.UnlockedCount += game.AchievementSummary.UnlockedCount
		out.Totals.TotalPoints += game.AchievementSummary.TotalPoints
		out.Totals.EarnedPoints += game.AchievementSummary.EarnedPoints

		for _, system := range gameSystems {
			total := systemTotals[system.Source]
			if total == nil {
				total = &core.CachedAchievementSystemSummary{Source: system.Source}
				systemTotals[system.Source] = total
				systemGames[system.Source] = make(map[string]bool)
			}
			if !systemGames[system.Source][game.ID] {
				systemGames[system.Source][game.ID] = true
				total.GameCount++
			}
			total.TotalCount += system.TotalCount
			total.UnlockedCount += system.UnlockedCount
			total.TotalPoints += system.TotalPoints
			total.EarnedPoints += system.EarnedPoints
		}
	}

	for _, system := range systemTotals {
		out.Systems = append(out.Systems, *system)
	}
	out.Totals.SourceCount = len(out.Systems)
	sort.Slice(out.Systems, func(i, j int) bool {
		if out.Systems[i].GameCount != out.Systems[j].GameCount {
			return out.Systems[i].GameCount > out.Systems[j].GameCount
		}
		return out.Systems[i].Source < out.Systems[j].Source
	})
	sort.Slice(out.Games, func(i, j int) bool {
		return strings.ToLower(out.Games[i].Game.Title) < strings.ToLower(out.Games[j].Game.Title)
	})
	return out, nil
}

func (s *gameStore) GetCachedAchievementsExplorer(ctx context.Context) (*core.CachedAchievementsExplorer, error) {
	games, err := s.GetCanonicalGames(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.GetDB().QueryContext(ctx, `
		SELECT a.id, l.canonical_id, a.source_game_id, a.source, a.external_game_id,
		       a.total_count, a.unlocked_count, COALESCE(a.total_points, 0), COALESCE(a.earned_points, 0), a.fetched_at
		FROM achievement_sets a
		JOIN canonical_source_games_link l ON l.source_game_id = a.source_game_id
		JOIN source_games sg ON sg.id = l.source_game_id
		WHERE `+visibleSourceGameWhere("sg")+`
		ORDER BY l.canonical_id, a.source, a.external_game_id, a.fetched_at DESC, a.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	setsByGame := make(map[string][]cachedAchievementExplorerSet)
	setIDs := make([]int64, 0)
	seenGameSet := make(map[string]bool)
	for rows.Next() {
		var item cachedAchievementExplorerSet
		if err := rows.Scan(
			&item.setID,
			&item.canonicalID,
			&item.sourceGameID,
			&item.source,
			&item.externalID,
			&item.totalCount,
			&item.unlockedCount,
			&item.totalPoints,
			&item.earnedPoints,
			&item.fetchedAt,
		); err != nil {
			return nil, err
		}
		key := item.canonicalID + "|" + item.source + "|" + item.externalID
		if seenGameSet[key] {
			continue
		}
		seenGameSet[key] = true
		setsByGame[item.canonicalID] = append(setsByGame[item.canonicalID], item)
		setIDs = append(setIDs, item.setID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	achievementsBySet := make(map[int64][]core.Achievement, len(setIDs))
	if len(setIDs) > 0 {
		placeholders := make([]string, len(setIDs))
		args := make([]any, len(setIDs))
		for i, id := range setIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		query := fmt.Sprintf(`SELECT set_id, external_id, title, description, locked_icon, unlocked_icon, points, rarity, unlocked, unlocked_at
			FROM achievements
			WHERE set_id IN (%s)
			ORDER BY unlocked DESC, COALESCE(unlocked_at, 0) DESC, title, external_id`, strings.Join(placeholders, ","))
		achievementRows, err := s.db.GetDB().QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer achievementRows.Close()

		for achievementRows.Next() {
			var (
				setID        int64
				item         core.Achievement
				lockedIcon   sql.NullString
				unlockedIcon sql.NullString
				unlockedAt   sql.NullInt64
			)
			if err := achievementRows.Scan(
				&setID,
				&item.ExternalID,
				&item.Title,
				&item.Description,
				&lockedIcon,
				&unlockedIcon,
				&item.Points,
				&item.Rarity,
				&item.Unlocked,
				&unlockedAt,
			); err != nil {
				return nil, err
			}
			if lockedIcon.Valid {
				item.LockedIcon = lockedIcon.String
			}
			if unlockedIcon.Valid {
				item.UnlockedIcon = unlockedIcon.String
			}
			if unlockedAt.Valid {
				item.UnlockedAt = time.Unix(unlockedAt.Int64, 0).UTC()
			}
			achievementsBySet[setID] = append(achievementsBySet[setID], item)
		}
		if err := achievementRows.Err(); err != nil {
			return nil, err
		}
	}

	out := &core.CachedAchievementsExplorer{}
	for _, game := range games {
		if game == nil {
			continue
		}
		items := setsByGame[game.ID]
		if len(items) == 0 {
			continue
		}
		gameItem := core.CachedAchievementGameExplorer{
			Game:    game,
			Systems: make([]core.AchievementSet, 0, len(items)),
		}
		for _, item := range items {
			gameItem.Systems = append(gameItem.Systems, core.AchievementSet{
				GameID:         item.sourceGameID,
				Source:         item.source,
				ExternalGameID: item.externalID,
				TotalCount:     item.totalCount,
				UnlockedCount:  item.unlockedCount,
				TotalPoints:    item.totalPoints,
				EarnedPoints:   item.earnedPoints,
				FetchedAt:      time.Unix(item.fetchedAt, 0).UTC(),
				Achievements:   append([]core.Achievement(nil), achievementsBySet[item.setID]...),
			})
		}
		sort.Slice(gameItem.Systems, func(i, j int) bool {
			if gameItem.Systems[i].UnlockedCount != gameItem.Systems[j].UnlockedCount {
				return gameItem.Systems[i].UnlockedCount > gameItem.Systems[j].UnlockedCount
			}
			return gameItem.Systems[i].Source < gameItem.Systems[j].Source
		})
		out.Games = append(out.Games, gameItem)
	}
	sort.Slice(out.Games, func(i, j int) bool {
		return strings.ToLower(out.Games[i].Game.Title) < strings.ToLower(out.Games[j].Game.Title)
	})
	return out, nil
}

func aggregateAchievementRows(items []cachedAchievementDashboardRow) []core.CachedAchievementSystemSummary {
	bySource := make(map[string]*core.CachedAchievementSystemSummary)
	for _, item := range items {
		summary := bySource[item.source]
		if summary == nil {
			summary = &core.CachedAchievementSystemSummary{Source: item.source}
			bySource[item.source] = summary
		}
		summary.GameCount = 1
		summary.TotalCount += item.totalCount
		summary.UnlockedCount += item.unlockedCount
		summary.TotalPoints += item.totalPoints
		summary.EarnedPoints += item.earnedPoints
	}
	out := make([]core.CachedAchievementSystemSummary, 0, len(bySource))
	for _, item := range bySource {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Source < out[j].Source
	})
	return out
}

var yearPattern = regexp.MustCompile(`\b(\d{4})\b`)

func decadeLabel(releaseDate string) string {
	match := yearPattern.FindStringSubmatch(releaseDate)
	if len(match) != 2 {
		return ""
	}
	year := 0
	for _, ch := range match[1] {
		year = (year * 10) + int(ch-'0')
	}
	if year < 1000 {
		return ""
	}
	decade := (year / 10) * 10
	return fmt.Sprintf("%ds", decade)
}

func (s *gameStore) GetGamesByIntegrationID(ctx context.Context, integrationID string, limit int) ([]core.GameListItem, error) {
	db := s.db.GetDB()

	q := fmt.Sprintf(`SELECT DISTINCT l.canonical_id,
	        COALESCE(NULLIF(m.title,''), sg.raw_title) AS title,
	        sg.platform
	      FROM source_games sg
	      JOIN canonical_source_games_link l ON l.source_game_id = sg.id
	      LEFT JOIN metadata_resolver_matches m ON m.source_game_id = sg.id AND m.outvoted = 0
	      WHERE sg.integration_id = ? AND %s
	      GROUP BY l.canonical_id
	      ORDER BY title
	      LIMIT ?`, visibleSourceGameWhere("sg"))

	if limit <= 0 {
		limit = 10000
	}

	rows, err := db.QueryContext(ctx, q, integrationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []core.GameListItem
	for rows.Next() {
		var g core.GameListItem
		if err := rows.Scan(&g.ID, &g.Title, &g.Platform); err != nil {
			return nil, err
		}
		items = append(items, g)
	}
	return items, rows.Err()
}

func (s *gameStore) GetEnrichedGamesByPluginID(ctx context.Context, pluginID string, limit int) ([]core.GameListItem, error) {
	db := s.db.GetDB()

	q := fmt.Sprintf(`SELECT DISTINCT l.canonical_id,
	        COALESCE(NULLIF(m.title,''), sg.raw_title) AS title,
	        sg.platform
	      FROM metadata_resolver_matches m
	      JOIN source_games sg ON sg.id = m.source_game_id AND %s
	      JOIN canonical_source_games_link l ON l.source_game_id = sg.id
	      WHERE m.plugin_id = ? AND m.outvoted = 0
	      GROUP BY l.canonical_id
	      ORDER BY title
	      LIMIT ?`, visibleSourceGameWhere("sg"))

	if limit <= 0 {
		limit = 10000
	}

	rows, err := db.QueryContext(ctx, q, pluginID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []core.GameListItem
	for rows.Next() {
		var g core.GameListItem
		if err := rows.Scan(&g.ID, &g.Title, &g.Platform); err != nil {
			return nil, err
		}
		items = append(items, g)
	}
	return items, rows.Err()
}

func reviewReasonsForCandidate(platform core.Platform, groupKind core.GroupKind, resolverMatchCount, resolverTitleCount int) []string {
	var reasons []string
	if resolverMatchCount == 0 {
		reasons = append(reasons, "no_metadata_matches")
	}
	if resolverTitleCount == 0 {
		reasons = append(reasons, "no_resolved_title")
	}
	if platform == core.PlatformUnknown {
		reasons = append(reasons, "unknown_platform")
	}
	if groupKind == core.GroupKindUnknown {
		reasons = append(reasons, "unknown_grouping")
	}
	return reasons
}

func (s *gameStore) ListManualReviewCandidates(ctx context.Context, scope core.ManualReviewScope, limit int) ([]*core.ManualReviewCandidate, error) {
	db := s.db.GetDB()
	if limit <= 0 {
		limit = 10000
	}
	whereClause := activeUndetectedSourceGameWhere("sg")
	if scope == core.ManualReviewScopeArchive {
		whereClause = "sg.status = 'found' AND IFNULL(sg.review_state, 'pending') = 'not_a_game'"
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT
		sg.id,
		COALESCE(l.canonical_id, ''),
		COALESCE((
			SELECT NULLIF(m.title, '')
			FROM metadata_resolver_matches m
			WHERE m.source_game_id = sg.id AND m.outvoted = 0
			ORDER BY m.id
			LIMIT 1
		), sg.raw_title),
		sg.raw_title,
		sg.platform,
		sg.kind,
		sg.group_kind,
		sg.integration_id,
		sg.plugin_id,
		sg.external_id,
		COALESCE(sg.root_path, ''),
		COALESCE(sg.url, ''),
		sg.status,
		COALESCE(sg.review_state, 'pending'),
		COALESCE((SELECT COUNT(*) FROM game_files gf WHERE gf.source_game_id = sg.id), 0),
		COALESCE((SELECT COUNT(*) FROM metadata_resolver_matches m WHERE m.source_game_id = sg.id), 0),
		COALESCE((SELECT COUNT(*) FROM metadata_resolver_matches m WHERE m.source_game_id = sg.id AND m.outvoted = 0 AND IFNULL(m.title, '') != ''), 0),
		sg.created_at,
		sg.last_seen_at
	FROM source_games sg
	LEFT JOIN canonical_source_games_link l ON l.source_game_id = sg.id
	WHERE %s
	ORDER BY sg.raw_title
	LIMIT ?`, whereClause), limit)
	if err != nil {
		return nil, fmt.Errorf("list manual review candidates: %w", err)
	}
	defer rows.Close()

	var candidates []*core.ManualReviewCandidate
	for rows.Next() {
		var candidate core.ManualReviewCandidate
		var lastSeen sql.NullInt64
		var resolverTitleCount int
		var createdAt int64
		if err := rows.Scan(
			&candidate.ID,
			&candidate.CanonicalGameID,
			&candidate.CurrentTitle,
			&candidate.RawTitle,
			(*string)(&candidate.Platform),
			(*string)(&candidate.Kind),
			(*string)(&candidate.GroupKind),
			&candidate.IntegrationID,
			&candidate.PluginID,
			&candidate.ExternalID,
			&candidate.RootPath,
			&candidate.URL,
			&candidate.Status,
			(*string)(&candidate.ReviewState),
			&candidate.FileCount,
			&candidate.ResolverMatchCount,
			&resolverTitleCount,
			&createdAt,
			&lastSeen,
		); err != nil {
			return nil, err
		}
		candidate.CreatedAt = time.Unix(createdAt, 0)
		if lastSeen.Valid {
			t := time.Unix(lastSeen.Int64, 0)
			candidate.LastSeenAt = &t
		}
		candidate.ReviewReasons = reviewReasonsForCandidate(candidate.Platform, candidate.GroupKind, candidate.ResolverMatchCount, resolverTitleCount)
		if scope == core.ManualReviewScopeActive && len(candidate.ReviewReasons) == 0 {
			continue
		}
		candidates = append(candidates, &candidate)
	}
	return candidates, rows.Err()
}

func (s *gameStore) GetManualReviewCandidate(ctx context.Context, candidateID string) (*core.ManualReviewCandidate, error) {
	db := s.db.GetDB()
	sg, err := s.loadSourceGame(ctx, db, candidateID)
	if err != nil {
		return nil, fmt.Errorf("load manual review candidate source game: %w", err)
	}
	if sg == nil || sg.Status != "found" {
		return nil, nil
	}

	candidate := &core.ManualReviewCandidate{
		ID:                 sg.ID,
		RawTitle:           sg.RawTitle,
		CurrentTitle:       sg.RawTitle,
		Platform:           sg.Platform,
		Kind:               sg.Kind,
		GroupKind:          sg.GroupKind,
		IntegrationID:      sg.IntegrationID,
		PluginID:           sg.PluginID,
		ExternalID:         sg.ExternalID,
		RootPath:           sg.RootPath,
		URL:                sg.URL,
		Status:             sg.Status,
		ReviewState:        sg.ReviewState,
		FileCount:          len(sg.Files),
		ResolverMatchCount: len(sg.ResolverMatches),
		Files:              append([]core.GameFile(nil), sg.Files...),
		ResolverMatches:    append([]core.ResolverMatch(nil), sg.ResolverMatches...),
		CreatedAt:          sg.CreatedAt,
		LastSeenAt:         sg.LastSeenAt,
	}

	resolverTitleCount := 0
	for _, match := range sg.ResolverMatches {
		if !match.Outvoted && strings.TrimSpace(match.Title) != "" {
			resolverTitleCount++
			if candidate.CurrentTitle == sg.RawTitle {
				candidate.CurrentTitle = match.Title
			}
		}
	}
	candidate.ReviewReasons = reviewReasonsForCandidate(candidate.Platform, candidate.GroupKind, candidate.ResolverMatchCount, resolverTitleCount)

	var canonicalID sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id = ? LIMIT 1`,
		sg.ID,
	).Scan(&canonicalID); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("load manual review candidate canonical id: %w", err)
	}
	candidate.CanonicalGameID = canonicalID.String

	return candidate, nil
}

func (s *gameStore) SaveManualReviewResult(
	ctx context.Context,
	sourceGame *core.SourceGame,
	resolverMatches []core.ResolverMatch,
	media []core.MediaRef,
) error {
	if sourceGame == nil {
		return fmt.Errorf("source game is required")
	}
	if sourceGame.ID == "" {
		return fmt.Errorf("source game id is required")
	}
	if sourceGame.ReviewState == "" {
		sourceGame.ReviewState = core.ManualReviewStatePending
	}
	manualReviewJSON, err := manualReviewDecisionJSON(sourceGame.ManualReview)
	if err != nil {
		return fmt.Errorf("marshal manual review decision: %w", err)
	}

	db := s.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var lastSeen any
	if sourceGame.LastSeenAt != nil {
		lastSeen = sourceGame.LastSeenAt.Unix()
	}

	result, err := tx.ExecContext(ctx, `UPDATE source_games
		SET raw_title = ?,
			platform = ?,
			kind = ?,
			group_kind = ?,
			root_path = ?,
			url = ?,
			status = 'found',
			review_state = ?,
			manual_review_json = ?,
			last_seen_at = COALESCE(?, last_seen_at)
		WHERE id = ?`,
		sourceGame.RawTitle,
		string(sourceGame.Platform),
		string(sourceGame.Kind),
		string(sourceGame.GroupKind),
		nullEmpty(sourceGame.RootPath),
		nullEmpty(sourceGame.URL),
		string(sourceGame.ReviewState),
		nullEmpty(manualReviewJSON),
		lastSeen,
		sourceGame.ID,
	)
	if err != nil {
		return fmt.Errorf("update source game review result %s: %w", sourceGame.ID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected for source game review result %s: %w", sourceGame.ID, err)
	}
	if rowsAffected == 0 {
		return core.ErrManualReviewCandidateNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM metadata_resolver_matches WHERE source_game_id = ?`, sourceGame.ID); err != nil {
		return fmt.Errorf("delete resolver matches for %s: %w", sourceGame.ID, err)
	}
	now := time.Now().Unix()
	for _, match := range resolverMatches {
		metaJSON, _ := buildMetadataJSON(match)
		if _, err := tx.ExecContext(ctx, `INSERT INTO metadata_resolver_matches
			(source_game_id, plugin_id, external_id, title, platform, url, outvoted, manual_selection,
			 developer, publisher, release_date, rating, metadata_json, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sourceGame.ID,
			match.PluginID,
			match.ExternalID,
			nullEmpty(match.Title),
			nullEmpty(match.Platform),
			nullEmpty(match.URL),
			boolToInt(match.Outvoted),
			boolToInt(match.ManualSelection),
			nullEmpty(match.Developer),
			nullEmpty(match.Publisher),
			nullEmpty(match.ReleaseDate),
			match.Rating,
			nullEmpty(metaJSON),
			now,
		); err != nil {
			return fmt.Errorf("insert manual review match for %s/%s: %w", sourceGame.ID, match.PluginID, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM source_game_media WHERE source_game_id = ?`, sourceGame.ID); err != nil {
		return fmt.Errorf("delete media links for %s: %w", sourceGame.ID, err)
	}
	for _, ref := range media {
		if ref.URL == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_assets (url, width, height, mime_type)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(url) DO UPDATE SET
				width = COALESCE(NULLIF(excluded.width,0), media_assets.width),
				height = COALESCE(NULLIF(excluded.height,0), media_assets.height)`,
			ref.URL, ref.Width, ref.Height, nullEmpty("")); err != nil {
			return fmt.Errorf("upsert media asset %s: %w", ref.URL, err)
		}
		var assetID int
		if err := tx.QueryRowContext(ctx, `SELECT id FROM media_assets WHERE url = ?`, ref.URL).Scan(&assetID); err != nil {
			return fmt.Errorf("get media asset id for %s: %w", ref.URL, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO source_game_media
			(source_game_id, media_asset_id, type, source) VALUES (?, ?, ?, ?)`,
			sourceGame.ID, assetID, string(ref.Type), nullEmpty(ref.Source)); err != nil {
			return fmt.Errorf("link manual review media for %s: %w", sourceGame.ID, err)
		}
	}

	if err := s.recomputeCanonicalGroups(ctx, tx); err != nil {
		return fmt.Errorf("recompute canonical after manual review save: %w", err)
	}
	return tx.Commit()
}

func (s *gameStore) SaveRefreshedMetadataProviderResults(ctx context.Context, sourceGames []*core.SourceGame) error {
	if len(sourceGames) == 0 {
		return nil
	}

	db := s.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	for _, sourceGame := range sourceGames {
		if sourceGame == nil || strings.TrimSpace(sourceGame.ID) == "" {
			continue
		}

		var existingID string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM source_games WHERE id = ?`, sourceGame.ID).Scan(&existingID); err != nil {
			if err == sql.ErrNoRows {
				return core.ErrSourceGameDeleteNotFound
			}
			return fmt.Errorf("verify refreshed source game %s: %w", sourceGame.ID, err)
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM metadata_resolver_matches WHERE source_game_id = ?`, sourceGame.ID); err != nil {
			return fmt.Errorf("delete refreshed resolver matches for %s: %w", sourceGame.ID, err)
		}
		for _, match := range sourceGame.ResolverMatches {
			metaJSON, _ := buildMetadataJSON(match)
			if _, err := tx.ExecContext(ctx, `INSERT INTO metadata_resolver_matches
				(source_game_id, plugin_id, external_id, title, platform, url, outvoted, manual_selection,
				 developer, publisher, release_date, rating, metadata_json, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				sourceGame.ID,
				match.PluginID,
				match.ExternalID,
				nullEmpty(match.Title),
				nullEmpty(match.Platform),
				nullEmpty(match.URL),
				boolToInt(match.Outvoted),
				boolToInt(match.ManualSelection),
				nullEmpty(match.Developer),
				nullEmpty(match.Publisher),
				nullEmpty(match.ReleaseDate),
				match.Rating,
				nullEmpty(metaJSON),
				now,
			); err != nil {
				return fmt.Errorf("insert refreshed match for %s/%s: %w", sourceGame.ID, match.PluginID, err)
			}
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM source_game_media WHERE source_game_id = ?`, sourceGame.ID); err != nil {
			return fmt.Errorf("delete refreshed media links for %s: %w", sourceGame.ID, err)
		}
		for _, ref := range sourceGame.Media {
			if strings.TrimSpace(ref.URL) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO media_assets (url, width, height, mime_type)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(url) DO UPDATE SET
					width = COALESCE(NULLIF(excluded.width,0), media_assets.width),
					height = COALESCE(NULLIF(excluded.height,0), media_assets.height)`,
				ref.URL, ref.Width, ref.Height, nullEmpty("")); err != nil {
				return fmt.Errorf("upsert refreshed media asset %s: %w", ref.URL, err)
			}
			var assetID int
			if err := tx.QueryRowContext(ctx, `SELECT id FROM media_assets WHERE url = ?`, ref.URL).Scan(&assetID); err != nil {
				return fmt.Errorf("get refreshed media asset id for %s: %w", ref.URL, err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO source_game_media
				(source_game_id, media_asset_id, type, source) VALUES (?, ?, ?, ?)`,
				sourceGame.ID, assetID, string(ref.Type), nullEmpty(ref.Source)); err != nil {
				return fmt.Errorf("link refreshed media for %s: %w", sourceGame.ID, err)
			}
		}
	}

	if err := s.recomputeCanonicalGroups(ctx, tx); err != nil {
		return fmt.Errorf("recompute canonical after provider refresh: %w", err)
	}
	return tx.Commit()
}

func (s *gameStore) SetManualReviewState(ctx context.Context, candidateID string, state core.ManualReviewState) error {
	if candidateID == "" {
		return fmt.Errorf("candidate id is required")
	}
	if state == "" {
		return fmt.Errorf("review state is required")
	}

	db := s.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `UPDATE source_games
		SET review_state = ?, manual_review_json = NULL
		WHERE id = ?`, string(state), candidateID)
	if err != nil {
		return fmt.Errorf("update manual review state %s: %w", candidateID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected for manual review state %s: %w", candidateID, err)
	}
	if rowsAffected == 0 {
		return core.ErrManualReviewCandidateNotFound
	}
	if err := s.recomputeCanonicalGroups(ctx, tx); err != nil {
		return fmt.Errorf("recompute canonical after manual review state update: %w", err)
	}
	return tx.Commit()
}

func (s *gameStore) GetFoundSourceGames(ctx context.Context, integrationIDs []string) ([]*core.FoundSourceGame, error) {
	db := s.db.GetDB()

	var rows *sql.Rows
	var err error

	if len(integrationIDs) == 0 {
		rows, err = db.QueryContext(ctx,
			`SELECT id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, COALESCE(root_path,'')
			 FROM source_games WHERE `+visibleSourceGameWhere("source_games"))
	} else {
		// Build placeholders for the IN clause.
		placeholders := make([]string, len(integrationIDs))
		args := make([]any, len(integrationIDs))
		for i, id := range integrationIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		q := fmt.Sprintf(
			`SELECT id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, COALESCE(root_path,'')
			 FROM source_games WHERE %s AND integration_id IN (%s)`,
			visibleSourceGameWhere("source_games"),
			strings.Join(placeholders, ","))
		rows, err = db.QueryContext(ctx, q, args...)
	}

	if err != nil {
		return nil, fmt.Errorf("get found source games: %w", err)
	}
	defer rows.Close()

	var games []*core.FoundSourceGame
	for rows.Next() {
		var g core.FoundSourceGame
		if err := rows.Scan(&g.ID, &g.IntegrationID, &g.PluginID, &g.ExternalID,
			&g.RawTitle, &g.Platform, &g.Kind, &g.GroupKind, &g.RootPath); err != nil {
			return nil, err
		}
		games = append(games, &g)
	}
	return games, rows.Err()
}

func (s *gameStore) GetFoundSourceGameRecords(ctx context.Context, integrationIDs []string) ([]*core.SourceGame, error) {
	db := s.db.GetDB()

	var rows *sql.Rows
	var err error

	if len(integrationIDs) == 0 {
		rows, err = db.QueryContext(ctx, `SELECT id FROM source_games WHERE `+visibleSourceGameWhere("source_games"))
	} else {
		placeholders := make([]string, len(integrationIDs))
		args := make([]any, len(integrationIDs))
		for i, id := range integrationIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		query := fmt.Sprintf(
			`SELECT id FROM source_games WHERE %s AND integration_id IN (%s)`,
			visibleSourceGameWhere("source_games"),
			strings.Join(placeholders, ","),
		)
		rows, err = db.QueryContext(ctx, query, args...)
	}
	if err != nil {
		return nil, fmt.Errorf("get found source game records: %w", err)
	}

	var sourceGameIDs []string
	for rows.Next() {
		var sourceGameID string
		if err := rows.Scan(&sourceGameID); err != nil {
			rows.Close()
			return nil, err
		}
		sourceGameIDs = append(sourceGameIDs, sourceGameID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	var records []*core.SourceGame
	for _, sourceGameID := range sourceGameIDs {
		record, err := s.loadSourceGame(ctx, db, sourceGameID)
		if err != nil {
			return nil, err
		}
		if isVisibleSourceGame(record) {
			records = append(records, record)
		}
	}
	return records, nil
}

func (s *gameStore) DeleteGamesByIntegrationID(ctx context.Context, integrationID string) error {
	db := s.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Collect source game IDs for this integration.
	rows, err := tx.QueryContext(ctx, `SELECT id FROM source_games WHERE integration_id = ?`, integrationID)
	if err != nil {
		return fmt.Errorf("list source games: %w", err)
	}
	var sgIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		sgIDs = append(sgIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if len(sgIDs) == 0 {
		return tx.Commit()
	}

	if err := s.deleteSourceGamesByID(ctx, tx, sgIDs); err != nil {
		return err
	}

	if err := s.recomputeCanonicalGroups(ctx, tx); err != nil {
		return fmt.Errorf("recompute canonical after integration delete: %w", err)
	}

	return tx.Commit()
}

func (s *gameStore) DeleteSourceGameByID(ctx context.Context, sourceGameID string) error {
	if strings.TrimSpace(sourceGameID) == "" {
		return fmt.Errorf("source game id is required")
	}

	db := s.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var existingID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM source_games WHERE id = ?`, sourceGameID).Scan(&existingID); err != nil {
		if err == sql.ErrNoRows {
			return core.ErrSourceGameDeleteNotFound
		}
		return fmt.Errorf("load source game %s: %w", sourceGameID, err)
	}

	if err := s.deleteSourceGamesByID(ctx, tx, []string{sourceGameID}); err != nil {
		return err
	}
	if err := s.recomputeCanonicalGroups(ctx, tx); err != nil {
		return fmt.Errorf("recompute canonical after source delete: %w", err)
	}
	return tx.Commit()
}

func (s *gameStore) deleteSourceGamesByID(ctx context.Context, tx *sql.Tx, sourceGameIDs []string) error {
	if len(sourceGameIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(sourceGameIDs))
	args := make([]any, len(sourceGameIDs))
	for i, id := range sourceGameIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	inClause := strings.Join(placeholders, ",")

	deleteQueries := []string{
		fmt.Sprintf(`DELETE FROM source_cache_jobs WHERE source_game_id IN (%s)`, inClause),
		fmt.Sprintf(`DELETE FROM source_cache_entry_files WHERE entry_id IN (SELECT id FROM source_cache_entries WHERE source_game_id IN (%s))`, inClause),
		fmt.Sprintf(`DELETE FROM source_cache_entries WHERE source_game_id IN (%s)`, inClause),
		fmt.Sprintf(`DELETE FROM achievements WHERE set_id IN (SELECT id FROM achievement_sets WHERE source_game_id IN (%s))`, inClause),
		fmt.Sprintf(`DELETE FROM achievement_sets WHERE source_game_id IN (%s)`, inClause),
		fmt.Sprintf(`DELETE FROM source_game_media WHERE source_game_id IN (%s)`, inClause),
		fmt.Sprintf(`DELETE FROM metadata_resolver_matches WHERE source_game_id IN (%s)`, inClause),
		fmt.Sprintf(`DELETE FROM game_files WHERE source_game_id IN (%s)`, inClause),
		fmt.Sprintf(`DELETE FROM canonical_source_games_link WHERE source_game_id IN (%s)`, inClause),
	}

	for _, q := range deleteQueries {
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("cascade delete: %w", err)
		}
	}

	q := fmt.Sprintf(`DELETE FROM source_games WHERE id IN (%s)`, inClause)
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("delete source games: %w", err)
	}
	return nil
}

func (s *gameStore) SaveScanReport(ctx context.Context, report *core.ScanReport) error {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal scan report: %w", err)
	}

	metaOnly := 0
	if report.MetadataOnly {
		metaOnly = 1
	}

	_, err = s.db.GetDB().ExecContext(ctx,
		`INSERT INTO scan_reports (id, started_at, finished_at, duration_ms, metadata_only, report_json)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			finished_at = excluded.finished_at,
			duration_ms = excluded.duration_ms,
			report_json = excluded.report_json`,
		report.ID, report.StartedAt.Unix(), report.FinishedAt.Unix(),
		report.DurationMs, metaOnly, string(reportJSON))
	return err
}

func (s *gameStore) GetScanReports(ctx context.Context, limit int) ([]*core.ScanReport, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.GetDB().QueryContext(ctx,
		`SELECT report_json FROM scan_reports ORDER BY finished_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []*core.ScanReport
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var r core.ScanReport
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, fmt.Errorf("unmarshal scan report: %w", err)
		}
		reports = append(reports, &r)
	}
	return reports, rows.Err()
}

func (s *gameStore) GetScanReport(ctx context.Context, id string) (*core.ScanReport, error) {
	row := s.db.GetDB().QueryRowContext(ctx,
		`SELECT report_json FROM scan_reports WHERE id = ?`, id)

	var raw string
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	var r core.ScanReport
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, fmt.Errorf("unmarshal scan report: %w", err)
	}
	return &r, nil
}

func (s *gameStore) GetSourceGameCountsByIntegration(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.GetDB().QueryContext(ctx,
		`SELECT integration_id, COUNT(*) FROM source_games WHERE `+visibleSourceGameWhere("source_games")+` GROUP BY integration_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		counts[id] = n
	}
	return counts, rows.Err()
}

func scanGroupCounts(ctx context.Context, db *sql.DB, query string, dest map[string]int) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return err
		}
		dest[k] = n
	}
	return rows.Err()
}

// ── Internal helpers ────────────────────────────────────────────────

type existingSourceGame struct {
	ID               string
	PluginID         string
	ExternalID       string
	RawTitle         string
	Platform         string
	RootPath         string
	Status           string
	ReviewState      core.ManualReviewState
	ManualReviewJSON string
}

func visibleSourceGameWhere(alias string) string {
	if alias == "" {
		alias = "source_games"
	}
	return fmt.Sprintf(
		"%s.status = 'found' AND IFNULL(%s.review_state, 'pending') != 'not_a_game' AND NOT (%s)",
		alias,
		alias,
		activeUndetectedSourceGameWhere(alias),
	)
}

func activeUndetectedSourceGameWhere(alias string) string {
	if alias == "" {
		alias = "source_games"
	}
	resolverMatchCountExpr := fmt.Sprintf(`(SELECT COUNT(*) FROM metadata_resolver_matches m WHERE m.source_game_id = %s.id)`, alias)
	resolverTitleCountExpr := fmt.Sprintf(`(SELECT COUNT(*) FROM metadata_resolver_matches m WHERE m.source_game_id = %s.id AND m.outvoted = 0 AND IFNULL(m.title, '') != '')`, alias)
	return fmt.Sprintf(
		"%s.status = 'found' AND IFNULL(%s.review_state, 'pending') = 'pending' AND ((%s) = 0 OR (%s) = 0 OR %s.platform = '%s' OR %s.group_kind = '%s')",
		alias,
		alias,
		resolverMatchCountExpr,
		resolverTitleCountExpr,
		alias,
		core.PlatformUnknown,
		alias,
		core.GroupKindUnknown,
	)
}

func isVisibleSourceGame(sg *core.SourceGame) bool {
	return sg != nil && sg.Status == "found" && sg.ReviewState != core.ManualReviewStateNotAGame && !isActiveUndetectedSourceGame(sg)
}

func isActiveUndetectedSourceGame(sg *core.SourceGame) bool {
	if sg == nil || sg.Status != "found" || sg.ReviewState != core.ManualReviewStatePending {
		return false
	}
	resolverTitleCount := 0
	for _, match := range sg.ResolverMatches {
		if !match.Outvoted && strings.TrimSpace(match.Title) != "" {
			resolverTitleCount++
		}
	}
	return len(reviewReasonsForCandidate(sg.Platform, sg.GroupKind, len(sg.ResolverMatches), resolverTitleCount)) > 0
}

func resolveManualReviewPersistence(sg *core.SourceGame, existing existingSourceGame) (core.ManualReviewState, string, error) {
	state := sg.ReviewState
	if state == "" {
		if existing.ReviewState != "" {
			state = existing.ReviewState
		} else {
			state = core.ManualReviewStatePending
		}
	}

	decision := sg.ManualReview
	if decision == nil && existing.ManualReviewJSON != "" {
		parsed, err := parseManualReviewDecisionJSON(existing.ManualReviewJSON)
		if err != nil {
			return "", "", err
		}
		decision = parsed
	}
	if state != core.ManualReviewStateMatched {
		decision = nil
	}

	raw, err := manualReviewDecisionJSON(decision)
	if err != nil {
		return "", "", err
	}
	return state, raw, nil
}

func manualReviewDecisionJSON(decision *core.ManualReviewDecision) (string, error) {
	if decision == nil {
		return "", nil
	}
	payload, err := json.Marshal(decision)
	if err != nil {
		return "", err
	}
	if string(payload) == "null" {
		return "", nil
	}
	return string(payload), nil
}

func parseManualReviewDecisionJSON(raw string) (*core.ManualReviewDecision, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var decision core.ManualReviewDecision
	if err := json.Unmarshal([]byte(raw), &decision); err != nil {
		return nil, fmt.Errorf("parse manual review json: %w", err)
	}
	return &decision, nil
}

func applyPersistedManualReview(sg *core.SourceGame, matches []core.ResolverMatch, existing existingSourceGame) ([]core.ResolverMatch, error) {
	if sg != nil && sg.ReviewState == core.ManualReviewStateMatched && sg.ManualReview != nil && sg.ManualReview.Selected != nil {
		return enforceManualSelection(matches, *sg.ManualReview.Selected), nil
	}
	if existing.ReviewState != core.ManualReviewStateMatched || strings.TrimSpace(existing.ManualReviewJSON) == "" {
		return matches, nil
	}
	decision, err := parseManualReviewDecisionJSON(existing.ManualReviewJSON)
	if err != nil {
		return nil, err
	}
	if decision == nil || decision.Selected == nil {
		return matches, nil
	}
	return enforceManualSelection(matches, *decision.Selected), nil
}

func enforceManualSelection(matches []core.ResolverMatch, selection core.ManualReviewSelection) []core.ResolverMatch {
	locked := manualSelectionToResolverMatch(selection)
	normalizedSelectedKey := manualSelectionConsensusKey(selection)
	out := make([]core.ResolverMatch, 0, len(matches)+1)
	out = append(out, locked)
	for _, match := range matches {
		if match.PluginID == locked.PluginID && match.ExternalID == locked.ExternalID {
			continue
		}
		normalizedMatchKey := normalizeManualReviewConsensusKey(match.Title)
		match.ManualSelection = false
		if normalizedSelectedKey != "" && normalizedMatchKey != normalizedSelectedKey {
			match.Outvoted = true
		} else {
			match.Outvoted = false
		}
		out = append(out, match)
	}
	return out
}

func manualSelectionToResolverMatch(selection core.ManualReviewSelection) core.ResolverMatch {
	var media []core.MediaItem
	if strings.TrimSpace(selection.ImageURL) != "" {
		media = append(media, core.MediaItem{
			Type:   core.MediaTypeCover,
			URL:    strings.TrimSpace(selection.ImageURL),
			Source: selection.ProviderPluginID,
		})
	}
	return core.ResolverMatch{
		PluginID:        strings.TrimSpace(selection.ProviderPluginID),
		Title:           strings.TrimSpace(selection.Title),
		Platform:        strings.TrimSpace(selection.Platform),
		Kind:            strings.TrimSpace(selection.Kind),
		ParentGameID:    strings.TrimSpace(selection.ParentGameID),
		ExternalID:      strings.TrimSpace(selection.ExternalID),
		URL:             strings.TrimSpace(selection.URL),
		Description:     strings.TrimSpace(selection.Description),
		ReleaseDate:     strings.TrimSpace(selection.ReleaseDate),
		Genres:          append([]string(nil), selection.Genres...),
		Developer:       strings.TrimSpace(selection.Developer),
		Publisher:       strings.TrimSpace(selection.Publisher),
		Rating:          selection.Rating,
		MaxPlayers:      selection.MaxPlayers,
		Media:           media,
		ManualSelection: true,
	}
}

func manualSelectionConsensusKey(selection core.ManualReviewSelection) string {
	return normalizeManualReviewConsensusKey(selection.Title)
}

func normalizeManualReviewConsensusKey(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.ReplaceAll(title, "-", " ")
	return strings.Join(strings.Fields(title), " ")
}

func sourceGameNaturalKey(pluginID, externalID string) string {
	return pluginID + "\x00" + externalID
}

func (s *gameStore) normalizeDuplicateSourceGames(batch *core.ScanBatch) *core.ScanBatch {
	if batch == nil || len(batch.SourceGames) == 0 {
		return batch
	}

	changed := false
	orderedKeys := make([]string, 0, len(batch.SourceGames))
	latestByNaturalKey := make(map[string]*core.SourceGame, len(batch.SourceGames))

	for _, original := range batch.SourceGames {
		sg, fileChanged := s.normalizeDuplicateFiles(batch.IntegrationID, original)
		changed = changed || fileChanged

		naturalKey := sourceGameNaturalKey(sg.PluginID, sg.ExternalID)
		if prev, ok := latestByNaturalKey[naturalKey]; ok && prev.ID != sg.ID {
			changed = true
			s.logger.Warn(
				"duplicate source game in scan batch; later entry overwriting earlier entry",
				"integration_id", batch.IntegrationID,
				"plugin_id", sg.PluginID,
				"external_id", sg.ExternalID,
				"discarded_id", prev.ID,
				"discarded_title", prev.RawTitle,
				"kept_id", sg.ID,
				"kept_title", sg.RawTitle,
			)
		} else if !ok {
			orderedKeys = append(orderedKeys, naturalKey)
		}
		latestByNaturalKey[naturalKey] = sg
	}

	if !changed && len(orderedKeys) == len(batch.SourceGames) {
		return batch
	}

	dedupedSourceGames := make([]*core.SourceGame, 0, len(orderedKeys))
	dedupedResolverMatches := make(map[string][]core.ResolverMatch, len(orderedKeys))
	dedupedMediaItems := make(map[string][]core.MediaRef, len(orderedKeys))

	for _, naturalKey := range orderedKeys {
		sg := latestByNaturalKey[naturalKey]
		dedupedSourceGames = append(dedupedSourceGames, sg)
		if matches, ok := batch.ResolverMatches[sg.ID]; ok {
			dedupedResolverMatches[sg.ID] = matches
		}
		if refs, ok := batch.MediaItems[sg.ID]; ok {
			dedupedMediaItems[sg.ID] = refs
		}
	}

	return &core.ScanBatch{
		IntegrationID:   batch.IntegrationID,
		SourceGames:     dedupedSourceGames,
		ResolverMatches: dedupedResolverMatches,
		MediaItems:      dedupedMediaItems,
	}
}

func (s *gameStore) normalizeDuplicateFiles(integrationID string, sg *core.SourceGame) (*core.SourceGame, bool) {
	if sg == nil || len(sg.Files) == 0 {
		return sg, false
	}

	orderedPaths := make([]string, 0, len(sg.Files))
	latestByPath := make(map[string]core.GameFile, len(sg.Files))
	changed := false

	for _, original := range sg.Files {
		normalized := original
		normalized.Path = filepath.ToSlash(strings.TrimSpace(normalized.Path))
		if normalized.Path != original.Path {
			changed = true
		}

		key := normalized.Path
		if prev, ok := latestByPath[key]; ok {
			changed = true
			if !sameGameFile(prev, normalized) {
				s.logger.Warn(
					"duplicate game file in scan batch; later entry overwriting earlier entry",
					"integration_id", integrationID,
					"source_game_id", sg.ID,
					"plugin_id", sg.PluginID,
					"external_id", sg.ExternalID,
					"path", key,
					"discarded_file_name", prev.FileName,
					"discarded_role", prev.Role,
					"discarded_file_kind", prev.FileKind,
					"discarded_size", prev.Size,
					"kept_file_name", normalized.FileName,
					"kept_role", normalized.Role,
					"kept_file_kind", normalized.FileKind,
					"kept_size", normalized.Size,
				)
			}
		} else {
			orderedPaths = append(orderedPaths, key)
		}

		latestByPath[key] = normalized
	}

	if !changed {
		return sg, false
	}

	clone := *sg
	clone.Files = make([]core.GameFile, 0, len(orderedPaths))
	for _, key := range orderedPaths {
		clone.Files = append(clone.Files, latestByPath[key])
	}
	return &clone, true
}

func sameGameFile(a, b core.GameFile) bool {
	return filepath.ToSlash(strings.TrimSpace(a.Path)) == filepath.ToSlash(strings.TrimSpace(b.Path)) &&
		a.FileName == b.FileName &&
		a.Role == b.Role &&
		a.FileKind == b.FileKind &&
		a.Size == b.Size &&
		a.IsDir == b.IsDir &&
		a.ObjectID == b.ObjectID &&
		a.Revision == b.Revision
}

func (s *gameStore) loadExistingSourceGames(ctx context.Context, tx *sql.Tx, integrationID string) ([]existingSourceGame, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, plugin_id, external_id, raw_title, platform, COALESCE(root_path,''), status, COALESCE(review_state, 'pending'), COALESCE(manual_review_json, '')
		 FROM source_games WHERE integration_id=?`, integrationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []existingSourceGame
	for rows.Next() {
		var e existingSourceGame
		if err := rows.Scan(&e.ID, &e.PluginID, &e.ExternalID, &e.RawTitle, &e.Platform, &e.RootPath, &e.Status, (*string)(&e.ReviewState), &e.ManualReviewJSON); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *gameStore) reconcileMissingSourceGames(ctx context.Context, tx *sql.Tx, integrationID string, seenIDs map[string]bool, scope *core.FilesystemScanScope) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, COALESCE(root_path,''), status FROM source_games WHERE integration_id=?`, integrationID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var toSoftDelete []string
	var toHardDelete []string
	for rows.Next() {
		var (
			id       string
			rootPath string
			status   string
		)
		if err := rows.Scan(&id, &rootPath, &status); err != nil {
			return err
		}
		if seenIDs[id] {
			continue
		}
		if scope != nil && !scopeContainsRootPath(rootPath, scope) {
			toHardDelete = append(toHardDelete, id)
			continue
		}
		if status != "not_found" {
			toSoftDelete = append(toSoftDelete, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(toHardDelete) > 0 {
		if err := s.deleteSourceGamesByID(ctx, tx, toHardDelete); err != nil {
			return err
		}
		s.logger.Info("hard-deleted out-of-scope source games", "count", len(toHardDelete), "integration_id", integrationID)
	}
	for _, id := range toSoftDelete {
		if _, err := tx.ExecContext(ctx, `UPDATE source_games SET status='not_found' WHERE id=?`, id); err != nil {
			return err
		}
	}
	if len(toSoftDelete) > 0 {
		s.logger.Info("soft-deleted missing source games", "count", len(toSoftDelete), "integration_id", integrationID)
	}
	return nil
}

func scopeContainsRootPath(rootPath string, scope *core.FilesystemScanScope) bool {
	if scope == nil {
		return true
	}
	includes := make([]sourcescope.IncludePath, 0, len(scope.IncludePaths))
	for _, include := range scope.IncludePaths {
		includes = append(includes, sourcescope.IncludePath{
			Path:      include.Path,
			Recursive: include.Recursive,
		})
	}
	return sourcescope.ScopeContainsRootPath(rootPath, includes)
}

func (s *gameStore) recomputeCanonicalGroups(ctx context.Context, tx *sql.Tx) error {
	existingMembership, err := s.loadExistingCanonicalMembership(ctx, tx)
	if err != nil {
		return err
	}
	existingCanonicalGames, err := s.loadExistingCanonicalGames(ctx, tx)
	if err != nil {
		return err
	}

	// Load all active source games.
	rows, err := tx.QueryContext(ctx, `SELECT id FROM source_games WHERE `+visibleSourceGameWhere("source_games"))
	if err != nil {
		return err
	}
	defer rows.Close()

	var allIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		allIDs = append(allIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Build a union-find by shared resolver external IDs.
	// Two source games that share the same (plugin_id, external_id) from
	// a non-outvoted resolver match are the same logical game.
	parent := make(map[string]string)
	for _, id := range allIDs {
		parent[id] = id
	}

	var find func(string) string
	find = func(x string) string {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	// Query all non-outvoted resolver external IDs.
	matchRows, err := tx.QueryContext(ctx, `SELECT source_game_id, plugin_id, external_id
		FROM metadata_resolver_matches WHERE outvoted=0`)
	if err != nil {
		return err
	}
	defer matchRows.Close()

	// Group by (plugin_id, external_id) → list of source_game_ids.
	extGroups := map[string][]string{} // "plugin_id|external_id" -> source_game_ids
	for matchRows.Next() {
		var sgID, pluginID, extID string
		if err := matchRows.Scan(&sgID, &pluginID, &extID); err != nil {
			return err
		}
		if _, ok := parent[sgID]; !ok {
			continue // skip non-active
		}
		key := pluginID + "|" + extID
		extGroups[key] = append(extGroups[key], sgID)
	}
	if err := matchRows.Err(); err != nil {
		return err
	}

	// Union source games that share external IDs.
	for _, members := range extGroups {
		if len(members) < 2 {
			continue
		}
		for i := 1; i < len(members); i++ {
			union(members[0], members[i])
		}
	}

	// Build canonical groups.
	groups := map[string][]string{} // root -> members
	for _, id := range allIDs {
		root := find(id)
		groups[root] = append(groups[root], id)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM canonical_source_games_link`); err != nil {
		return err
	}

	assignments, err := assignStableCanonicalIDs(groups, existingMembership, existingCanonicalGames)
	if err != nil {
		return err
	}

	for _, canonicalID := range collectNewCanonicalIDs(assignments, existingCanonicalGames) {
		createdAt := time.Now().Unix()
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO canonical_games (id, created_at) VALUES (?, ?)`, canonicalID, createdAt); err != nil {
			return err
		}
	}

	for _, group := range assignments {
		canonicalID := group.canonicalID
		members := group.members
		for _, sgID := range members {
			_, err := tx.ExecContext(ctx, `INSERT INTO canonical_source_games_link (canonical_id, source_game_id) VALUES (?, ?)`,
				canonicalID, sgID)
			if err != nil {
				return err
			}
		}
	}

	s.logger.Info("recomputed canonical groups", "source_games", len(allIDs), "canonical_games", len(groups))
	return nil
}

type canonicalGameMeta struct {
	id        string
	createdAt int64
}

type canonicalAssignment struct {
	groupKey    string
	members     []string
	canonicalID string
}

type canonicalCandidate struct {
	groupKey    string
	canonicalID string
	overlap     int
	createdAt   int64
}

func (s *gameStore) loadExistingCanonicalMembership(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT source_game_id, canonical_id FROM canonical_source_games_link`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var sourceGameID, canonicalID string
		if err := rows.Scan(&sourceGameID, &canonicalID); err != nil {
			return nil, err
		}
		out[sourceGameID] = canonicalID
	}
	return out, rows.Err()
}

func (s *gameStore) loadExistingCanonicalGames(ctx context.Context, tx *sql.Tx) (map[string]canonicalGameMeta, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, created_at FROM canonical_games`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]canonicalGameMeta)
	for rows.Next() {
		var meta canonicalGameMeta
		if err := rows.Scan(&meta.id, &meta.createdAt); err != nil {
			return nil, err
		}
		out[meta.id] = meta
	}
	return out, rows.Err()
}

func assignStableCanonicalIDs(
	groups map[string][]string,
	existingMembership map[string]string,
	existingCanonicalGames map[string]canonicalGameMeta,
) ([]canonicalAssignment, error) {
	groupKeys := make([]string, 0, len(groups))
	groupByKey := make(map[string][]string, len(groups))
	candidates := make([]canonicalCandidate, 0)

	for _, members := range groups {
		sortedMembers := append([]string(nil), members...)
		sort.Strings(sortedMembers)
		if len(sortedMembers) == 0 {
			continue
		}
		groupKey := sortedMembers[0]
		groupKeys = append(groupKeys, groupKey)
		groupByKey[groupKey] = sortedMembers

		overlapByCanonical := make(map[string]int)
		for _, member := range sortedMembers {
			if canonicalID, ok := existingMembership[member]; ok && canonicalID != "" {
				overlapByCanonical[canonicalID]++
			}
		}

		for canonicalID, overlap := range overlapByCanonical {
			if strings.HasPrefix(canonicalID, "scan:") {
				continue
			}
			createdAt := int64(^uint64(0) >> 1)
			if meta, ok := existingCanonicalGames[canonicalID]; ok {
				createdAt = meta.createdAt
			}
			candidates = append(candidates, canonicalCandidate{
				groupKey:    groupKey,
				canonicalID: canonicalID,
				overlap:     overlap,
				createdAt:   createdAt,
			})
		}
	}

	sort.Strings(groupKeys)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].overlap != candidates[j].overlap {
			return candidates[i].overlap > candidates[j].overlap
		}
		if candidates[i].createdAt != candidates[j].createdAt {
			return candidates[i].createdAt < candidates[j].createdAt
		}
		if candidates[i].canonicalID != candidates[j].canonicalID {
			return candidates[i].canonicalID < candidates[j].canonicalID
		}
		return candidates[i].groupKey < candidates[j].groupKey
	})

	assignedByGroup := make(map[string]string, len(groupKeys))
	usedCanonicalIDs := make(map[string]bool)
	for _, candidate := range candidates {
		if assignedByGroup[candidate.groupKey] != "" || usedCanonicalIDs[candidate.canonicalID] {
			continue
		}
		assignedByGroup[candidate.groupKey] = candidate.canonicalID
		usedCanonicalIDs[candidate.canonicalID] = true
	}

	assignments := make([]canonicalAssignment, 0, len(groupKeys))
	for _, groupKey := range groupKeys {
		canonicalID := assignedByGroup[groupKey]
		if canonicalID == "" {
			canonicalID = uuid.NewString()
		}
		assignments = append(assignments, canonicalAssignment{
			groupKey:    groupKey,
			members:     groupByKey[groupKey],
			canonicalID: canonicalID,
		})
	}

	return assignments, nil
}

func collectNewCanonicalIDs(
	assignments []canonicalAssignment,
	existingCanonicalGames map[string]canonicalGameMeta,
) []string {
	var out []string
	for _, assignment := range assignments {
		if _, ok := existingCanonicalGames[assignment.canonicalID]; ok {
			continue
		}
		out = append(out, assignment.canonicalID)
	}
	return out
}

func (s *gameStore) loadSourceGame(ctx context.Context, db *sql.DB, sgID string) (*core.SourceGame, error) {
	var sg core.SourceGame
	var lastSeen sql.NullInt64
	var createdAt int64
	var rootPath, url, manualReviewJSON sql.NullString
	err := db.QueryRowContext(ctx, `SELECT id, integration_id, plugin_id, external_id, raw_title,
		platform, kind, group_kind, root_path, url, status, COALESCE(review_state, 'pending'), COALESCE(manual_review_json, ''), last_seen_at, created_at
		FROM source_games WHERE id=?`, sgID).Scan(
		&sg.ID, &sg.IntegrationID, &sg.PluginID, &sg.ExternalID, &sg.RawTitle,
		(*string)(&sg.Platform), (*string)(&sg.Kind), (*string)(&sg.GroupKind),
		&rootPath, &url, &sg.Status, (*string)(&sg.ReviewState), &manualReviewJSON, &lastSeen, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	sg.RootPath = rootPath.String
	sg.URL = url.String
	sg.CreatedAt = time.Unix(createdAt, 0)
	if manualReviewJSON.String != "" {
		decision, err := parseManualReviewDecisionJSON(manualReviewJSON.String)
		if err != nil {
			return nil, err
		}
		sg.ManualReview = decision
	}
	if lastSeen.Valid {
		t := time.Unix(lastSeen.Int64, 0)
		sg.LastSeenAt = &t
	}

	// Load files.
	fileRows, err := db.QueryContext(ctx, `SELECT path, file_name, role, file_kind, size, is_dir, object_id, revision, modified_at
		FROM game_files WHERE source_game_id=?`, sgID)
	if err != nil {
		return nil, err
	}
	defer fileRows.Close()
	for fileRows.Next() {
		var f core.GameFile
		var isDir int
		var objectID, revision sql.NullString
		var modifiedAt sql.NullInt64
		if err := fileRows.Scan(&f.Path, &f.FileName, &f.Role, &f.FileKind, &f.Size, &isDir, &objectID, &revision, &modifiedAt); err != nil {
			return nil, err
		}
		f.GameID = sgID
		f.IsDir = isDir != 0
		f.ObjectID = objectID.String
		f.Revision = revision.String
		if modifiedAt.Valid {
			t := time.Unix(modifiedAt.Int64, 0).UTC()
			f.ModifiedAt = &t
		}
		sg.Files = append(sg.Files, f)
	}

	// Load resolver matches.
	matchRows, err := db.QueryContext(ctx, `SELECT plugin_id, external_id, title, platform, url, outvoted, manual_selection,
		developer, publisher, release_date, rating, metadata_json
		FROM metadata_resolver_matches WHERE source_game_id=? ORDER BY id`, sgID)
	if err != nil {
		return nil, err
	}
	defer matchRows.Close()
	for matchRows.Next() {
		var m core.ResolverMatch
		var title, plat, murl, dev, pub, relDate, metaJSON sql.NullString
		var outvoted, manualSelection int
		if err := matchRows.Scan(&m.PluginID, &m.ExternalID, &title, &plat, &murl, &outvoted, &manualSelection,
			&dev, &pub, &relDate, &m.Rating, &metaJSON); err != nil {
			return nil, err
		}
		m.Title = title.String
		m.Platform = plat.String
		m.URL = murl.String
		m.Outvoted = outvoted != 0
		m.ManualSelection = manualSelection != 0
		m.Developer = dev.String
		m.Publisher = pub.String
		m.ReleaseDate = relDate.String
		if metaJSON.String != "" {
			parseMetadataJSON(metaJSON.String, &m)
			m.MetadataJSON = metaJSON.String
		}
		sg.ResolverMatches = append(sg.ResolverMatches, m)
	}

	return &sg, nil
}

func (s *gameStore) buildCanonicalGame(ctx context.Context, db *sql.DB, canonicalID string, sgIDs []string) (*core.CanonicalGame, error) {
	cg := &core.CanonicalGame{ID: canonicalID}
	hasVisible := false

	for _, sgID := range sgIDs {
		sg, err := s.loadSourceGame(ctx, db, sgID)
		if err != nil {
			return nil, err
		}
		if !isVisibleSourceGame(sg) {
			continue
		}
		hasVisible = true
		cg.SourceGames = append(cg.SourceGames, sg)
	}

	if !hasVisible {
		return nil, nil // all members are not_found/replaced
	}

	s.computeUnifiedView(cg)

	// Load media refs.
	for _, sg := range cg.SourceGames {
		if !isVisibleSourceGame(sg) {
			continue
		}
		mediaRows, err := db.QueryContext(ctx, `SELECT ma.id, ma.url, ma.local_path, ma.hash, ma.mime_type, sgm.type, sgm.source, ma.width, ma.height
			FROM source_game_media sgm
			JOIN media_assets ma ON ma.id = sgm.media_asset_id
			WHERE sgm.source_game_id=?`, sg.ID)
		if err != nil {
			return nil, err
		}
		for mediaRows.Next() {
			var ref core.MediaRef
			var src, lp, h, mt sql.NullString
			if err := mediaRows.Scan(&ref.AssetID, &ref.URL, &lp, &h, &mt, (*string)(&ref.Type), &src, &ref.Width, &ref.Height); err != nil {
				mediaRows.Close()
				return nil, err
			}
			ref.Source = src.String
			ref.LocalPath = lp.String
			ref.Hash = h.String
			ref.MimeType = mt.String
			cg.Media = append(cg.Media, ref)
		}
		mediaRows.Close()
	}
	coverOverride, err := s.loadCanonicalCoverOverride(ctx, db, canonicalID)
	if err != nil {
		return nil, err
	}
	cg.CoverOverride = coverOverride
	hoverOverride, err := s.loadCanonicalHoverOverride(ctx, db, canonicalID)
	if err != nil {
		return nil, err
	}
	cg.HoverOverride = hoverOverride
	backgroundOverride, err := s.loadCanonicalBackgroundOverride(ctx, db, canonicalID)
	if err != nil {
		return nil, err
	}
	cg.BackgroundOverride = backgroundOverride
	if err := s.ensureCanonicalMediaOverrides(ctx, cg); err != nil {
		return nil, err
	}

	// Load external IDs.
	eids, err := s.GetExternalIDsForCanonical(ctx, canonicalID)
	if err != nil {
		return nil, err
	}
	cg.ExternalIDs = eids

	summary, err := s.loadCanonicalAchievementSummary(ctx, db, canonicalID)
	if err != nil {
		return nil, err
	}
	cg.AchievementSummary = summary

	return cg, nil
}

func (s *gameStore) loadCanonicalAchievementSummary(ctx context.Context, db *sql.DB, canonicalID string) (*core.AchievementSummary, error) {
	rows, err := db.QueryContext(ctx, `SELECT s.source, s.external_game_id, s.total_count, s.unlocked_count, s.total_points, s.earned_points
		FROM achievement_sets s
		JOIN canonical_source_games_link l ON l.source_game_id = s.source_game_id
		WHERE l.canonical_id=?
		ORDER BY s.fetched_at DESC, s.id DESC`, canonicalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := &core.AchievementSummary{}
	seen := make(map[string]bool)
	for rows.Next() {
		var source, externalGameID string
		var totalCount, unlockedCount, totalPoints, earnedPoints int
		if err := rows.Scan(&source, &externalGameID, &totalCount, &unlockedCount, &totalPoints, &earnedPoints); err != nil {
			return nil, err
		}
		key := source + "|" + externalGameID
		if seen[key] {
			continue
		}
		seen[key] = true
		summary.SourceCount++
		summary.TotalCount += totalCount
		summary.UnlockedCount += unlockedCount
		summary.TotalPoints += totalPoints
		summary.EarnedPoints += earnedPoints
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if summary.SourceCount == 0 {
		return nil, nil
	}
	return summary, nil
}

func (s *gameStore) loadCanonicalCoverOverride(ctx context.Context, db *sql.DB, canonicalID string) (*core.MediaRef, error) {
	row := db.QueryRowContext(ctx, `
		SELECT ma.id, ma.url, ma.local_path, ma.hash, ma.mime_type, sgm.type, sgm.source, ma.width, ma.height
		FROM canonical_game_cover_overrides o
		JOIN media_assets ma ON ma.id = o.media_asset_id
		JOIN source_game_media sgm ON sgm.media_asset_id = ma.id
		JOIN canonical_source_games_link l ON l.source_game_id = sgm.source_game_id AND l.canonical_id = o.canonical_id
		JOIN source_games sg ON sg.id = l.source_game_id
		WHERE o.canonical_id = ? AND `+visibleSourceGameWhere("sg")+`
		LIMIT 1`, canonicalID)
	var ref core.MediaRef
	var src, lp, h, mt sql.NullString
	if err := row.Scan(&ref.AssetID, &ref.URL, &lp, &h, &mt, (*string)(&ref.Type), &src, &ref.Width, &ref.Height); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	ref.Source = src.String
	ref.LocalPath = lp.String
	ref.Hash = h.String
	ref.MimeType = mt.String
	return &ref, nil
}

func (s *gameStore) loadCanonicalHoverOverride(ctx context.Context, db *sql.DB, canonicalID string) (*core.MediaRef, error) {
	row := db.QueryRowContext(ctx, `
		SELECT ma.id, ma.url, ma.local_path, ma.hash, ma.mime_type, sgm.type, sgm.source, ma.width, ma.height
		FROM canonical_game_hover_overrides o
		JOIN media_assets ma ON ma.id = o.media_asset_id
		JOIN source_game_media sgm ON sgm.media_asset_id = ma.id
		JOIN canonical_source_games_link l ON l.source_game_id = sgm.source_game_id AND l.canonical_id = o.canonical_id
		JOIN source_games sg ON sg.id = l.source_game_id
		WHERE o.canonical_id = ? AND `+visibleSourceGameWhere("sg")+`
		LIMIT 1`, canonicalID)
	var ref core.MediaRef
	var src, lp, h, mt sql.NullString
	if err := row.Scan(&ref.AssetID, &ref.URL, &lp, &h, &mt, (*string)(&ref.Type), &src, &ref.Width, &ref.Height); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	ref.Source = src.String
	ref.LocalPath = lp.String
	ref.Hash = h.String
	ref.MimeType = mt.String
	return &ref, nil
}

func (s *gameStore) loadCanonicalBackgroundOverride(ctx context.Context, db *sql.DB, canonicalID string) (*core.MediaRef, error) {
	row := db.QueryRowContext(ctx, `
		SELECT ma.id, ma.url, ma.local_path, ma.hash, ma.mime_type, sgm.type, sgm.source, ma.width, ma.height
		FROM canonical_game_background_overrides o
		JOIN media_assets ma ON ma.id = o.media_asset_id
		JOIN source_game_media sgm ON sgm.media_asset_id = ma.id
		JOIN canonical_source_games_link l ON l.source_game_id = sgm.source_game_id AND l.canonical_id = o.canonical_id
		JOIN source_games sg ON sg.id = l.source_game_id
		WHERE o.canonical_id = ? AND `+visibleSourceGameWhere("sg")+`
		LIMIT 1`, canonicalID)
	var ref core.MediaRef
	var src, lp, h, mt sql.NullString
	if err := row.Scan(&ref.AssetID, &ref.URL, &lp, &h, &mt, (*string)(&ref.Type), &src, &ref.Width, &ref.Height); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	ref.Source = src.String
	ref.LocalPath = lp.String
	ref.Hash = h.String
	ref.MimeType = mt.String
	return &ref, nil
}

func (s *gameStore) ensureCanonicalMediaOverrides(ctx context.Context, cg *core.CanonicalGame) error {
	effectiveCover := cg.CoverOverride
	derivedCover := resolveCanonicalMediaRefByIdentity(cg.Media, selectCanonicalCoverMedia(cg.Media))
	coverCleared, err := s.hasClearedCanonicalCoverOverride(ctx, cg.ID)
	if err != nil {
		return err
	}
	if effectiveCover == nil && !coverCleared {
		effectiveCover = derivedCover
		if effectiveCover != nil {
			cg.CoverOverride = cloneMediaRef(effectiveCover)
			if effectiveCover.AssetID > 0 {
				// Rescans alone do not repair legacy override state unless scanning also
				// backfills and persists these override rows. We lazily backfill on load
				// so older canonical games immediately expose explicit cover/hover/background
				// selections without forcing a library-wide rescan.
				if err := s.SetCanonicalCoverOverride(ctx, cg.ID, effectiveCover.AssetID); err != nil {
					return err
				}
			}
		}
	}

	effectiveHover := cg.HoverOverride
	coverForDerivedSelections := cg.CoverOverride
	if coverForDerivedSelections == nil {
		coverForDerivedSelections = derivedCover
	}
	if effectiveHover == nil {
		effectiveHover = resolveCanonicalMediaRefByIdentity(cg.Media, selectCanonicalHoverMedia(cg.Media, coverForDerivedSelections))
		if effectiveHover != nil {
			cg.HoverOverride = cloneMediaRef(effectiveHover)
			if effectiveHover.AssetID > 0 {
				if err := s.SetCanonicalHoverOverride(ctx, cg.ID, effectiveHover.AssetID); err != nil {
					return err
				}
			}
		}
	}

	effectiveBackground := cg.BackgroundOverride
	if effectiveBackground == nil {
		effectiveBackground = resolveCanonicalMediaRefByIdentity(cg.Media, selectCanonicalBackgroundMedia(cg.Media, coverForDerivedSelections))
		if effectiveBackground != nil {
			cg.BackgroundOverride = cloneMediaRef(effectiveBackground)
			if effectiveBackground.AssetID > 0 {
				if err := s.SetCanonicalBackgroundOverride(ctx, cg.ID, effectiveBackground.AssetID); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (s *gameStore) hasClearedCanonicalCoverOverride(ctx context.Context, canonicalID string) (bool, error) {
	var count int
	if err := s.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM canonical_game_cover_override_clears WHERE canonical_id=?`, canonicalID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func cloneMediaRef(ref *core.MediaRef) *core.MediaRef {
	if ref == nil {
		return nil
	}
	cloned := *ref
	return &cloned
}

func selectCanonicalCoverMedia(media []core.MediaRef) *core.MediaRef {
	if match := findFirstMediaOfTypes(media, "cover"); match != nil {
		return match
	}
	return findFirstImageMedia(media)
}

func selectCanonicalHoverMedia(media []core.MediaRef, cover *core.MediaRef) *core.MediaRef {
	if match := findFirstMediaOfTypes(media, "screenshot", "background", "backdrop", "banner", "hero", "artwork", "fanart"); match != nil {
		return match
	}
	return cover
}

func selectCanonicalBackgroundMedia(media []core.MediaRef, cover *core.MediaRef) *core.MediaRef {
	if match := findFirstMediaOfTypes(media, "screenshot", "background", "backdrop", "banner", "artwork", "hero", "fanart", "cover"); match != nil {
		return match
	}
	return cover
}

func findFirstMediaOfTypes(media []core.MediaRef, types ...string) *core.MediaRef {
	for _, targetType := range types {
		for i := range media {
			if strings.EqualFold(string(media[i].Type), targetType) && isImageMediaRef(media[i]) {
				return &media[i]
			}
		}
	}
	return nil
}

func findFirstImageMedia(media []core.MediaRef) *core.MediaRef {
	for i := range media {
		if isImageMediaRef(media[i]) {
			return &media[i]
		}
	}
	return nil
}

func isImageMediaRef(media core.MediaRef) bool {
	mimeType := strings.ToLower(media.MimeType)
	if strings.HasPrefix(mimeType, "image/") {
		return true
	}
	if strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
		return false
	}
	switch strings.ToLower(string(media.Type)) {
	case "video", "trailer", "manual", "document", "audio", "soundtrack":
		return false
	default:
		return true
	}
}

func resolveCanonicalMediaRefByIdentity(media []core.MediaRef, candidate *core.MediaRef) *core.MediaRef {
	if candidate == nil {
		return nil
	}
	if candidate.AssetID > 0 {
		for i := range media {
			if media[i].AssetID == candidate.AssetID {
				return &media[i]
			}
		}
	}

	candidateKeys := canonicalMediaSecondaryIdentityKeys(*candidate)
	if len(candidateKeys) == 0 {
		return nil
	}
	for i := range media {
		for _, candidateKey := range candidateKeys {
			for _, mediaKey := range canonicalMediaSecondaryIdentityKeys(media[i]) {
				if candidateKey == mediaKey {
					return &media[i]
				}
			}
		}
	}
	return nil
}

func canonicalMediaIdentityKey(media core.MediaRef) string {
	if media.AssetID > 0 {
		return fmt.Sprintf("asset:%d", media.AssetID)
	}
	keys := canonicalMediaSecondaryIdentityKeys(media)
	if len(keys) > 0 {
		return keys[0]
	}
	return ""
}

func canonicalMediaSecondaryIdentityKeys(media core.MediaRef) []string {
	var keys []string
	if key := normalizeCanonicalMediaURL(media.URL); key != "" {
		keys = append(keys, "url:"+key)
	}
	if key := normalizeCanonicalMediaPath(media.LocalPath); key != "" {
		keys = append(keys, "path:"+key)
	}
	if key := strings.TrimSpace(strings.ToLower(media.Hash)); key != "" {
		keys = append(keys, "hash:"+key)
	}
	return keys
}

func normalizeCanonicalMediaURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.ToLower(raw)
	}
	parsed.Fragment = ""
	return strings.ToLower(parsed.String())
}

func normalizeCanonicalMediaPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return strings.ToLower(filepath.Clean(raw))
}

// computeUnifiedView fills the canonical game's unified fields from its source games' resolver matches.
func (s *gameStore) computeUnifiedView(cg *core.CanonicalGame) {
	// Collect all non-outvoted matches across all source games, keeping source game order.
	type rankedMatch struct {
		match   core.ResolverMatch
		sgIndex int
	}
	var winners []rankedMatch
	for i, sg := range cg.SourceGames {
		if !isVisibleSourceGame(sg) {
			continue
		}
		for _, m := range sg.ResolverMatches {
			if !m.Outvoted {
				winners = append(winners, rankedMatch{m, i})
			}
		}
	}

	// Sort manual selections ahead of automatic winners, then by source game order.
	sort.SliceStable(winners, func(i, j int) bool {
		if winners[i].match.ManualSelection != winners[j].match.ManualSelection {
			return winners[i].match.ManualSelection
		}
		return winners[i].sgIndex < winners[j].sgIndex
	})

	titleSet := false
	for _, w := range winners {
		m := w.match
		if m.Title != "" && !titleSet {
			cg.Title = m.Title
			titleSet = true
		}
		if m.Platform != "" && cg.Platform == "" {
			cg.Platform = core.Platform(m.Platform)
		}
		if m.Kind != "" && cg.Kind == "" {
			cg.Kind = core.GameKind(m.Kind)
		}
		if m.Description != "" && cg.Description == "" {
			cg.Description = m.Description
		}
		if m.ReleaseDate != "" && cg.ReleaseDate == "" {
			cg.ReleaseDate = m.ReleaseDate
		}
		if len(m.Genres) > 0 && len(cg.Genres) == 0 {
			cg.Genres = m.Genres
		}
		if m.Developer != "" && cg.Developer == "" {
			cg.Developer = m.Developer
		}
		if m.Publisher != "" && cg.Publisher == "" {
			cg.Publisher = m.Publisher
		}
		if m.Rating > 0 && cg.Rating == 0 {
			cg.Rating = m.Rating
		}
		if m.MaxPlayers > 0 && cg.MaxPlayers == 0 {
			cg.MaxPlayers = m.MaxPlayers
		}
		if m.CompletionTime != nil && cg.CompletionTime == nil {
			cg.CompletionTime = m.CompletionTime
		}
		if m.IsGamePass {
			cg.IsGamePass = true
		}
		if m.XcloudAvailable {
			cg.XcloudAvailable = true
		}
		if m.StoreProductID != "" && cg.StoreProductID == "" {
			cg.StoreProductID = m.StoreProductID
		}
		if m.XcloudURL != "" && cg.XcloudURL == "" {
			cg.XcloudURL = m.XcloudURL
		}
	}

	// Fallback: if no resolver set a title, use the first source game's raw title.
	if cg.Title == "" && len(cg.SourceGames) > 0 {
		cg.Title = cg.SourceGames[0].RawTitle
	}
	if cg.Platform == "" && len(cg.SourceGames) > 0 {
		cg.Platform = cg.SourceGames[0].Platform
	}
	if cg.Kind == "" {
		cg.Kind = core.GameKindBaseGame
	}
}

// ── JSON serialization helpers ──────────────────────────────────────

type metadataExtra struct {
	Genres          []string             `json:"genres,omitempty"`
	MaxPlayers      int                  `json:"max_players,omitempty"`
	Kind            string               `json:"kind,omitempty"`
	ParentGameID    string               `json:"parent_game_id,omitempty"`
	Description     string               `json:"description,omitempty"`
	CompletionTime  *core.CompletionTime `json:"completion_time,omitempty"`
	IsGamePass      bool                 `json:"is_game_pass,omitempty"`
	XcloudAvailable bool                 `json:"xcloud_available,omitempty"`
	StoreProductID  string               `json:"store_product_id,omitempty"`
	XcloudURL       string               `json:"xcloud_url,omitempty"`
}

func buildMetadataJSON(m core.ResolverMatch) (string, error) {
	extra := metadataExtra{
		Genres:          m.Genres,
		MaxPlayers:      m.MaxPlayers,
		Kind:            m.Kind,
		ParentGameID:    m.ParentGameID,
		Description:     m.Description,
		CompletionTime:  m.CompletionTime,
		IsGamePass:      m.IsGamePass,
		XcloudAvailable: m.XcloudAvailable,
		StoreProductID:  m.StoreProductID,
		XcloudURL:       m.XcloudURL,
	}
	b, err := json.Marshal(extra)
	if err != nil {
		return "", err
	}
	s := string(b)
	if s == "{}" {
		return "", nil
	}
	return s, nil
}

func parseMetadataJSON(s string, m *core.ResolverMatch) {
	var extra metadataExtra
	if err := json.Unmarshal([]byte(s), &extra); err != nil {
		return
	}
	m.Genres = extra.Genres
	m.MaxPlayers = extra.MaxPlayers
	m.Kind = extra.Kind
	m.ParentGameID = extra.ParentGameID
	if extra.Description != "" && m.Description == "" {
		m.Description = extra.Description
	}
	m.CompletionTime = extra.CompletionTime
	m.IsGamePass = extra.IsGamePass
	m.XcloudAvailable = extra.XcloudAvailable
	m.StoreProductID = extra.StoreProductID
	m.XcloudURL = extra.XcloudURL
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// buildPlaceholderList creates "?,?,?" for n items. Already exists in helpers.go as buildPlaceholders.
func buildPlaceholderList(n int) string {
	if n == 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}
