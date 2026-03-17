package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
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

	seenIDs := make(map[string]bool, len(batch.SourceGames))

	// 2. Upsert source games.
	for _, sg := range batch.SourceGames {
		seenIDs[sg.ID] = true

		// Detect move: same integration, a not_found game shares the same raw_title+platform.
		if sg.RootPath != "" {
			for _, ex := range existing {
				if ex.Status == "not_found" && ex.RawTitle == sg.RawTitle &&
					ex.Platform == string(sg.Platform) && ex.ID != sg.ID {
					s.logger.Info("detected game move",
						"old_id", ex.ID, "new_id", sg.ID,
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

		_, err := tx.ExecContext(ctx, `INSERT INTO source_games
			(id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, root_path, url, status, last_seen_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'found', ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				raw_title = excluded.raw_title,
				platform = excluded.platform,
				kind = excluded.kind,
				group_kind = excluded.group_kind,
				root_path = excluded.root_path,
				url = excluded.url,
				status = 'found',
				last_seen_at = excluded.last_seen_at`,
			sg.ID, sg.IntegrationID, sg.PluginID, sg.ExternalID,
			sg.RawTitle, string(sg.Platform), string(sg.Kind), string(sg.GroupKind),
			nullEmpty(sg.RootPath), nullEmpty(sg.URL), lastSeen, now)
		if err != nil {
			return fmt.Errorf("upsert source game %s: %w", sg.ID, err)
		}

		// 3. Replace files for this source game.
		if _, err := tx.ExecContext(ctx, `DELETE FROM game_files WHERE source_game_id=?`, sg.ID); err != nil {
			return fmt.Errorf("delete files for %s: %w", sg.ID, err)
		}
		for _, f := range sg.Files {
			isDir := 0
			if f.IsDir {
				isDir = 1
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO game_files
				(source_game_id, path, file_name, role, file_kind, size, is_dir)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				sg.ID, f.Path, f.FileName, string(f.Role), nullEmpty(f.FileKind), f.Size, isDir)
			if err != nil {
				return fmt.Errorf("insert file for %s: %w", sg.ID, err)
			}
		}
	}

	// 4. Insert resolver matches.
	for sgID, matches := range batch.ResolverMatches {
		if !seenIDs[sgID] {
			continue
		}
		// Clear old matches for this source game.
		if _, err := tx.ExecContext(ctx, `DELETE FROM metadata_resolver_matches WHERE source_game_id=?`, sgID); err != nil {
			return fmt.Errorf("delete matches for %s: %w", sgID, err)
		}
		for _, m := range matches {
			metaJSON, _ := buildMetadataJSON(m)
			_, err := tx.ExecContext(ctx, `INSERT INTO metadata_resolver_matches
				(source_game_id, plugin_id, external_id, title, platform, url, outvoted,
				 developer, publisher, release_date, rating, metadata_json, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				sgID, m.PluginID, m.ExternalID,
				nullEmpty(m.Title), nullEmpty(m.Platform), nullEmpty(m.URL),
				boolToInt(m.Outvoted),
				nullEmpty(m.Developer), nullEmpty(m.Publisher), nullEmpty(m.ReleaseDate),
				m.Rating, nullEmpty(metaJSON), now)
			if err != nil {
				return fmt.Errorf("insert match for %s/%s: %w", sgID, m.PluginID, err)
			}
		}
	}

	// 5. Upsert media assets + link to source games.
	for sgID, refs := range batch.MediaItems {
		if !seenIDs[sgID] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM source_game_media WHERE source_game_id=?`, sgID); err != nil {
			return fmt.Errorf("delete media links for %s: %w", sgID, err)
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
				sgID, assetID, string(ref.Type), nullEmpty(ref.Source))
			if err != nil {
				return fmt.Errorf("link media for %s: %w", sgID, err)
			}
		}
	}

	// 6. Soft-delete source games from this integration not seen in this batch.
	if err := s.softDeleteMissing(ctx, tx, batch.IntegrationID, seenIDs); err != nil {
		return fmt.Errorf("soft delete: %w", err)
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

func (s *gameStore) UpdateMediaAsset(ctx context.Context, assetID int, localPath, hash string) error {
	if assetID <= 0 {
		return fmt.Errorf("assetID must be positive")
	}
	_, err := s.db.GetDB().ExecContext(ctx,
		`UPDATE media_assets SET local_path=?, hash=? WHERE id=?`,
		localPath, hash, assetID)
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
	db := s.db.GetDB()

	// Load all canonical groups.
	rows, err := db.QueryContext(ctx, `SELECT canonical_id, source_game_id FROM canonical_source_games_link
		ORDER BY canonical_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := map[string][]string{} // canonical_id -> []source_game_id
	var order []string
	for rows.Next() {
		var cid, sgid string
		if err := rows.Scan(&cid, &sgid); err != nil {
			return nil, err
		}
		if _, ok := groups[cid]; !ok {
			order = append(order, cid)
		}
		groups[cid] = append(groups[cid], sgid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var result []*core.CanonicalGame
	for _, cid := range order {
		sgIDs := groups[cid]
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

func (s *gameStore) GetCanonicalGameByID(ctx context.Context, canonicalID string) (*core.CanonicalGame, error) {
	db := s.db.GetDB()
	rows, err := db.QueryContext(ctx, `SELECT source_game_id FROM canonical_source_games_link WHERE canonical_id=?`, canonicalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sgIDs []string
	for rows.Next() {
		var sgid string
		if err := rows.Scan(&sgid); err != nil {
			return nil, err
		}
		sgIDs = append(sgIDs, sgid)
	}
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
	defer rows.Close()

	var out []*core.SourceGame
	for rows.Next() {
		var sgid string
		if err := rows.Scan(&sgid); err != nil {
			return nil, err
		}
		sg, err := s.loadSourceGame(ctx, db, sgid)
		if err != nil {
			return nil, err
		}
		if sg != nil {
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
		WHERE l.canonical_id=? AND sg.status='found'`, canonicalID)
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

// ── Internal helpers ────────────────────────────────────────────────

type existingSourceGame struct {
	ID       string
	RawTitle string
	Platform string
	RootPath string
	Status   string
}

func (s *gameStore) loadExistingSourceGames(ctx context.Context, tx *sql.Tx, integrationID string) ([]existingSourceGame, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, raw_title, platform, COALESCE(root_path,''), status FROM source_games WHERE integration_id=?`, integrationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []existingSourceGame
	for rows.Next() {
		var e existingSourceGame
		if err := rows.Scan(&e.ID, &e.RawTitle, &e.Platform, &e.RootPath, &e.Status); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *gameStore) softDeleteMissing(ctx context.Context, tx *sql.Tx, integrationID string, seenIDs map[string]bool) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM source_games WHERE integration_id=? AND status='found'`, integrationID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var toDelete []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if !seenIDs[id] {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		if _, err := tx.ExecContext(ctx, `UPDATE source_games SET status='not_found' WHERE id=?`, id); err != nil {
			return err
		}
	}
	if len(toDelete) > 0 {
		s.logger.Info("soft-deleted missing source games", "count", len(toDelete), "integration_id", integrationID)
	}
	return nil
}

func (s *gameStore) recomputeCanonicalGroups(ctx context.Context, tx *sql.Tx) error {
	// Clear existing groupings.
	if _, err := tx.ExecContext(ctx, `DELETE FROM canonical_source_games_link`); err != nil {
		return err
	}

	// Load all active source games.
	rows, err := tx.QueryContext(ctx, `SELECT id FROM source_games WHERE status='found'`)
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

	// Insert canonical links. The canonical_id is the root of the group (first source game ID).
	for canonicalID, members := range groups {
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

func (s *gameStore) loadSourceGame(ctx context.Context, db *sql.DB, sgID string) (*core.SourceGame, error) {
	var sg core.SourceGame
	var lastSeen sql.NullInt64
	var createdAt int64
	var rootPath, url sql.NullString
	err := db.QueryRowContext(ctx, `SELECT id, integration_id, plugin_id, external_id, raw_title,
		platform, kind, group_kind, root_path, url, status, last_seen_at, created_at
		FROM source_games WHERE id=?`, sgID).Scan(
		&sg.ID, &sg.IntegrationID, &sg.PluginID, &sg.ExternalID, &sg.RawTitle,
		(*string)(&sg.Platform), (*string)(&sg.Kind), (*string)(&sg.GroupKind),
		&rootPath, &url, &sg.Status, &lastSeen, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	sg.RootPath = rootPath.String
	sg.URL = url.String
	sg.CreatedAt = time.Unix(createdAt, 0)
	if lastSeen.Valid {
		t := time.Unix(lastSeen.Int64, 0)
		sg.LastSeenAt = &t
	}

	// Load files.
	fileRows, err := db.QueryContext(ctx, `SELECT path, file_name, role, file_kind, size, is_dir
		FROM game_files WHERE source_game_id=?`, sgID)
	if err != nil {
		return nil, err
	}
	defer fileRows.Close()
	for fileRows.Next() {
		var f core.GameFile
		var isDir int
		if err := fileRows.Scan(&f.Path, &f.FileName, &f.Role, &f.FileKind, &f.Size, &isDir); err != nil {
			return nil, err
		}
		f.GameID = sgID
		f.IsDir = isDir != 0
		sg.Files = append(sg.Files, f)
	}

	// Load resolver matches.
	matchRows, err := db.QueryContext(ctx, `SELECT plugin_id, external_id, title, platform, url, outvoted,
		developer, publisher, release_date, rating, metadata_json
		FROM metadata_resolver_matches WHERE source_game_id=? ORDER BY id`, sgID)
	if err != nil {
		return nil, err
	}
	defer matchRows.Close()
	for matchRows.Next() {
		var m core.ResolverMatch
		var title, plat, murl, dev, pub, relDate, metaJSON sql.NullString
		var outvoted int
		if err := matchRows.Scan(&m.PluginID, &m.ExternalID, &title, &plat, &murl, &outvoted,
			&dev, &pub, &relDate, &m.Rating, &metaJSON); err != nil {
			return nil, err
		}
		m.Title = title.String
		m.Platform = plat.String
		m.URL = murl.String
		m.Outvoted = outvoted != 0
		m.Developer = dev.String
		m.Publisher = pub.String
		m.ReleaseDate = relDate.String
		if metaJSON.String != "" {
			parseMetadataJSON(metaJSON.String, &m)
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
		if sg == nil {
			continue
		}
		if sg.Status == "found" {
			hasVisible = true
		}
		cg.SourceGames = append(cg.SourceGames, sg)
	}

	if !hasVisible {
		return nil, nil // all members are not_found/replaced
	}

	s.computeUnifiedView(cg)

	// Load media refs.
	for _, sg := range cg.SourceGames {
		if sg.Status != "found" {
			continue
		}
		mediaRows, err := db.QueryContext(ctx, `SELECT ma.id, ma.url, sgm.type, sgm.source, ma.width, ma.height
			FROM source_game_media sgm
			JOIN media_assets ma ON ma.id = sgm.media_asset_id
			WHERE sgm.source_game_id=?`, sg.ID)
		if err != nil {
			return nil, err
		}
		for mediaRows.Next() {
			var ref core.MediaRef
			var src sql.NullString
			if err := mediaRows.Scan(&ref.AssetID, &ref.URL, (*string)(&ref.Type), &src, &ref.Width, &ref.Height); err != nil {
				mediaRows.Close()
				return nil, err
			}
			ref.Source = src.String
			cg.Media = append(cg.Media, ref)
		}
		mediaRows.Close()
	}

	// Load external IDs.
	eids, err := s.GetExternalIDsForCanonical(ctx, canonicalID)
	if err != nil {
		return nil, err
	}
	cg.ExternalIDs = eids

	return cg, nil
}

// computeUnifiedView fills the canonical game's unified fields from its source games' resolver matches.
func (s *gameStore) computeUnifiedView(cg *core.CanonicalGame) {
	// Collect all non-outvoted matches across all source games, keeping source game order.
	type rankedMatch struct {
		match    core.ResolverMatch
		sgIndex  int
	}
	var winners []rankedMatch
	for i, sg := range cg.SourceGames {
		if sg.Status != "found" {
			continue
		}
		for _, m := range sg.ResolverMatches {
			if !m.Outvoted {
				winners = append(winners, rankedMatch{m, i})
			}
		}
	}

	// Sort by source game index (earlier = higher priority within group).
	sort.SliceStable(winners, func(i, j int) bool {
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
	Genres         []string             `json:"genres,omitempty"`
	MaxPlayers     int                  `json:"max_players,omitempty"`
	Kind           string               `json:"kind,omitempty"`
	ParentGameID   string               `json:"parent_game_id,omitempty"`
	Description    string               `json:"description,omitempty"`
	CompletionTime *core.CompletionTime `json:"completion_time,omitempty"`
}

func buildMetadataJSON(m core.ResolverMatch) (string, error) {
	extra := metadataExtra{
		Genres:         m.Genres,
		MaxPlayers:     m.MaxPlayers,
		Kind:           m.Kind,
		ParentGameID:   m.ParentGameID,
		Description:    m.Description,
		CompletionTime: m.CompletionTime,
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
