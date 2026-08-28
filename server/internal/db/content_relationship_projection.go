package db

import (
	"context"
	"database/sql"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// GetCanonicalContentRelationshipProjectionGames returns the complete
// relationship graph as a single lightweight read. Game detail previously
// loaded every canonical game's files, media, achievements, delivery state,
// and identity merely to resolve parent/add-on links.
func (s *gameStore) GetCanonicalContentRelationshipProjectionGames(ctx context.Context) ([]*core.CanonicalGame, error) {
	rows, err := s.db.GetDB().QueryContext(ctx, `SELECT l.canonical_id,
			sg.id, sg.integration_id, sg.plugin_id, sg.external_id, sg.raw_title,
			sg.platform, sg.kind, sg.group_kind, sg.status,
			m.plugin_id, m.external_id, m.title, m.platform, m.outvoted,
			m.manual_selection, m.metadata_json
		FROM canonical_source_games_link l
		JOIN source_games sg ON sg.id = l.source_game_id
		LEFT JOIN metadata_resolver_matches m ON m.source_game_id = sg.id
		WHERE `+visibleSourceGameWhere(ctx, "sg")+`
		ORDER BY l.canonical_id, sg.id, m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	gamesByID := make(map[string]*core.CanonicalGame)
	gameOrder := make([]string, 0)
	sourcesByID := make(map[string]*core.SourceGame)
	for rows.Next() {
		var canonicalID string
		var source core.SourceGame
		var matchPlugin, matchExternalID, matchTitle, matchPlatform, metadataJSON sql.NullString
		var outvoted, manualSelection sql.NullInt64
		if err := rows.Scan(
			&canonicalID,
			&source.ID, &source.IntegrationID, &source.PluginID, &source.ExternalID, &source.RawTitle,
			(*string)(&source.Platform), (*string)(&source.Kind), (*string)(&source.GroupKind), &source.Status,
			&matchPlugin, &matchExternalID, &matchTitle, &matchPlatform, &outvoted,
			&manualSelection, &metadataJSON,
		); err != nil {
			return nil, err
		}

		game := gamesByID[canonicalID]
		if game == nil {
			game = &core.CanonicalGame{ID: canonicalID}
			gamesByID[canonicalID] = game
			gameOrder = append(gameOrder, canonicalID)
		}
		storedSource := sourcesByID[source.ID]
		if storedSource == nil {
			storedSource = &source
			sourcesByID[source.ID] = storedSource
			game.SourceGames = append(game.SourceGames, storedSource)
		}
		if matchPlugin.Valid && matchExternalID.Valid {
			match := core.ResolverMatch{
				PluginID:        matchPlugin.String,
				ExternalID:      matchExternalID.String,
				Title:           matchTitle.String,
				Platform:        matchPlatform.String,
				Outvoted:        outvoted.Int64 != 0,
				ManualSelection: manualSelection.Int64 != 0,
				MetadataJSON:    metadataJSON.String,
			}
			if metadataJSON.String != "" {
				parseMetadataJSON(metadataJSON.String, &match)
			}
			storedSource.ResolverMatches = append(storedSource.ResolverMatches, match)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]*core.CanonicalGame, 0, len(gameOrder))
	for _, canonicalID := range gameOrder {
		game := gamesByID[canonicalID]
		s.computeUnifiedView(game)
		result = append(result, game)
	}
	return result, nil
}
