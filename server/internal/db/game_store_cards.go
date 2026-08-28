package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const libraryCardBatchSize = 300

// GetCanonicalGameCardsByIDs builds the canonical fields needed by Library and
// Play without loading detail-only identity and external-ID collections. All
// persisted collections are loaded in set-based batches, so query count stays
// bounded as a page grows.
func (s *gameStore) GetCanonicalGameCardsByIDs(ctx context.Context, canonicalIDs []string) ([]*core.CanonicalGame, error) {
	if len(canonicalIDs) == 0 {
		return []*core.CanonicalGame{}, nil
	}
	db := s.db.GetDB()
	sourceIDsByCanonical := make(map[string][]string, len(canonicalIDs))
	var allSourceIDs []string
	for _, chunk := range stringChunks(canonicalIDs, libraryCardBatchSize) {
		rows, err := db.QueryContext(ctx, `SELECT l.canonical_id, l.source_game_id
			FROM canonical_source_games_link l
			JOIN source_games sg ON sg.id = l.source_game_id
			WHERE l.canonical_id IN (`+buildPlaceholderList(len(chunk))+`)`+profileFilterSQL(ctx, "sg")+`
			ORDER BY l.canonical_id, l.source_game_id`, stringsToAny(chunk)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var canonicalID, sourceGameID string
			if err := rows.Scan(&canonicalID, &sourceGameID); err != nil {
				rows.Close()
				return nil, err
			}
			sourceIDsByCanonical[canonicalID] = append(sourceIDsByCanonical[canonicalID], sourceGameID)
			allSourceIDs = append(allSourceIDs, sourceGameID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	sourceGames, err := s.loadSourceGames(ctx, db, allSourceIDs)
	if err != nil {
		return nil, err
	}
	enrichment, err := loadLibraryCardEnrichment(ctx, db, canonicalIDs)
	if err != nil {
		return nil, err
	}

	result := make([]*core.CanonicalGame, 0, len(canonicalIDs))
	for _, canonicalID := range canonicalIDs {
		sourceIDs := sourceIDsByCanonical[canonicalID]
		if len(sourceIDs) == 0 {
			continue
		}
		game := buildLibraryCanonicalGame(canonicalID, sourceIDs, sourceGames, enrichment)
		if game != nil {
			result = append(result, game)
		}
	}
	return result, nil
}

type libraryCardEnrichment struct {
	covers       map[string]*core.MediaRef
	hovers       map[string]*core.MediaRef
	backgrounds  map[string]*core.MediaRef
	coverCleared map[string]bool
	favorites    map[string]bool
	achievements map[string]*core.AchievementSummary
}

func loadLibraryCardEnrichment(ctx context.Context, db *sql.DB, canonicalIDs []string) (*libraryCardEnrichment, error) {
	data := &libraryCardEnrichment{
		covers:       make(map[string]*core.MediaRef),
		hovers:       make(map[string]*core.MediaRef),
		backgrounds:  make(map[string]*core.MediaRef),
		coverCleared: make(map[string]bool),
		favorites:    make(map[string]bool),
		achievements: make(map[string]*core.AchievementSummary),
	}

	var err error
	if data.covers, err = loadCanonicalMediaOverrideBatch(ctx, db, canonicalIDs, "canonical_game_cover_overrides"); err != nil {
		return nil, err
	}
	if data.hovers, err = loadCanonicalMediaOverrideBatch(ctx, db, canonicalIDs, "canonical_game_hover_overrides"); err != nil {
		return nil, err
	}
	if data.backgrounds, err = loadCanonicalMediaOverrideBatch(ctx, db, canonicalIDs, "canonical_game_background_overrides"); err != nil {
		return nil, err
	}

	for _, chunk := range stringChunks(canonicalIDs, libraryCardBatchSize) {
		args := stringsToAny(chunk)
		rows, err := db.QueryContext(ctx, `SELECT canonical_id FROM canonical_game_cover_override_clears
			WHERE canonical_id IN (`+buildPlaceholderList(len(chunk))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var canonicalID string
			if err := rows.Scan(&canonicalID); err != nil {
				rows.Close()
				return nil, err
			}
			data.coverCleared[canonicalID] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()

		favoriteQuery := `SELECT canonical_id FROM canonical_game_favorites
			WHERE canonical_id IN (` + buildPlaceholderList(len(chunk)) + `)`
		favoriteArgs := append([]any{}, args...)
		if profileID := core.ProfileIDFromContext(ctx); profileID != "" {
			favoriteQuery += ` AND profile_id = ?`
			favoriteArgs = append(favoriteArgs, profileID)
		}
		rows, err = db.QueryContext(ctx, favoriteQuery, favoriteArgs...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var canonicalID string
			if err := rows.Scan(&canonicalID); err != nil {
				rows.Close()
				return nil, err
			}
			data.favorites[canonicalID] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()

		rows, err = db.QueryContext(ctx, `SELECT l.canonical_id, s.source, s.external_game_id,
			s.total_count, s.unlocked_count, s.total_points, s.earned_points
			FROM achievement_sets s
			JOIN canonical_source_games_link l ON l.source_game_id = s.source_game_id
			WHERE l.canonical_id IN (`+buildPlaceholderList(len(chunk))+`)
			ORDER BY l.canonical_id, s.fetched_at DESC, s.id DESC`, args...)
		if err != nil {
			return nil, err
		}
		seen := make(map[string]bool)
		for rows.Next() {
			var canonicalID, source, externalGameID string
			var totalCount, unlockedCount, totalPoints, earnedPoints int
			if err := rows.Scan(&canonicalID, &source, &externalGameID, &totalCount, &unlockedCount, &totalPoints, &earnedPoints); err != nil {
				rows.Close()
				return nil, err
			}
			key := canonicalID + "\x00" + source + "\x00" + externalGameID
			if seen[key] {
				continue
			}
			seen[key] = true
			summary := data.achievements[canonicalID]
			if summary == nil {
				summary = &core.AchievementSummary{}
				data.achievements[canonicalID] = summary
			}
			summary.SourceCount++
			summary.TotalCount += totalCount
			summary.UnlockedCount += unlockedCount
			summary.TotalPoints += totalPoints
			summary.EarnedPoints += earnedPoints
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return data, nil
}

func loadCanonicalMediaOverrideBatch(ctx context.Context, db *sql.DB, canonicalIDs []string, table string) (map[string]*core.MediaRef, error) {
	allowed := map[string]bool{
		"canonical_game_cover_overrides":      true,
		"canonical_game_hover_overrides":      true,
		"canonical_game_background_overrides": true,
	}
	if !allowed[table] {
		return nil, fmt.Errorf("unsupported canonical media override table %q", table)
	}
	result := make(map[string]*core.MediaRef)
	for _, chunk := range stringChunks(canonicalIDs, libraryCardBatchSize) {
		query := `SELECT DISTINCT o.canonical_id, ma.id, ma.url, ma.local_path, ma.hash, ma.mime_type,
			sgm.type, sgm.source, ma.width, ma.height
			FROM ` + table + ` o
			JOIN media_assets ma ON ma.id = o.media_asset_id
			JOIN source_game_media sgm ON sgm.media_asset_id = ma.id
			JOIN canonical_source_games_link l ON l.source_game_id = sgm.source_game_id AND l.canonical_id = o.canonical_id
			JOIN source_games sg ON sg.id = l.source_game_id
			WHERE o.canonical_id IN (` + buildPlaceholderList(len(chunk)) + `)
			  AND ` + visibleSourceGameWhere(ctx, "sg")
		rows, err := db.QueryContext(ctx, query, stringsToAny(chunk)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var canonicalID string
			var ref core.MediaRef
			var localPath, hash, mimeType sql.NullString
			if err := rows.Scan(&canonicalID, &ref.AssetID, &ref.URL, &localPath, &hash, &mimeType, &ref.Type, &ref.Source, &ref.Width, &ref.Height); err != nil {
				rows.Close()
				return nil, err
			}
			ref.LocalPath = localPath.String
			ref.Hash = hash.String
			ref.MimeType = mimeType.String
			result[canonicalID] = &ref
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return result, nil
}

func buildLibraryCanonicalGame(canonicalID string, sourceIDs []string, sourceGames map[string]*core.SourceGame, enrichment *libraryCardEnrichment) *core.CanonicalGame {
	game := &core.CanonicalGame{ID: canonicalID}
	for _, sourceID := range sourceIDs {
		sourceGame := sourceGames[sourceID]
		if sourceGame == nil || !isVisibleSourceGame(sourceGame) {
			continue
		}
		game.SourceGames = append(game.SourceGames, sourceGame)
	}
	if len(game.SourceGames) == 0 {
		return nil
	}

	new(gameStore).computeUnifiedView(game)
	for _, sourceGame := range game.SourceGames {
		game.Media = append(game.Media, sourceGame.Media...)
	}
	game.CoverOverride = cloneMediaRef(enrichment.covers[canonicalID])
	game.HoverOverride = cloneMediaRef(enrichment.hovers[canonicalID])
	game.BackgroundOverride = cloneMediaRef(enrichment.backgrounds[canonicalID])
	if game.CoverOverride == nil && !enrichment.coverCleared[canonicalID] {
		game.CoverOverride = cloneMediaRef(resolveCanonicalMediaRefByIdentity(game.Media, selectCanonicalCoverMedia(game.Media)))
	}
	coverForDerived := game.CoverOverride
	if game.HoverOverride == nil {
		game.HoverOverride = cloneMediaRef(resolveCanonicalMediaRefByIdentity(game.Media, selectCanonicalHoverMedia(game.Media, coverForDerived)))
	}
	if game.BackgroundOverride == nil {
		game.BackgroundOverride = cloneMediaRef(resolveCanonicalMediaRefByIdentity(game.Media, selectCanonicalBackgroundMedia(game.Media, coverForDerived)))
	}
	game.Favorite = enrichment.favorites[canonicalID]
	game.AchievementSummary = enrichment.achievements[canonicalID]
	return game
}
