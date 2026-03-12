package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// PluginGamePersistenceService persists plugin-sourced games to the games table.
type PluginGamePersistenceService struct {
	gameRepo core.GameRepository
	logger   core.Logger
}

// NewPluginGamePersistenceService returns a PluginGamePersistence that upserts plugin games and marks stale ones not_found.
func NewPluginGamePersistenceService(gameRepo core.GameRepository, logger core.Logger) PluginGamePersistence {
	return &PluginGamePersistenceService{
		gameRepo: gameRepo,
		logger:   logger,
	}
}

func pluginGameID(integrationID, sourceGameKey string) string {
	h := sha256.Sum256([]byte(sourceGameKey))
	short := hex.EncodeToString(h[:])[:16]
	return "plugin:" + integrationID + ":" + short
}

// PersistPluginGames upserts one Game per entry, then marks plugin-sourced games for this integration that are no longer in the list as not_found.
func (s *PluginGamePersistenceService) PersistPluginGames(ctx context.Context, integrationID, sourceLabel string, entries []core.GameEntry) error {
	if len(entries) == 0 {
		if err := s.gameRepo.MarkPluginGamesNotFoundExcept(ctx, integrationID, nil); err != nil {
			return fmt.Errorf("mark plugin games not_found: %w", err)
		}
		return nil
	}

	games := make([]*core.Game, 0, len(entries))
	keepGameIDs := make([]string, 0, len(entries))

	for _, e := range entries {
		gameID := pluginGameID(integrationID, e.SourceGameKey)
		keepGameIDs = append(keepGameIDs, gameID)

		title := e.DisplayName
		if title == "" {
			title = e.SourceGameKey
		}

		games = append(games, &core.Game{
			ID:            gameID,
			Title:         title,
			Platform:      core.PlatformUnknown,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindUnknown,
			IntegrationID: integrationID,
			Confidence:    "unknown",
			Status:        "found",
		})
	}

	if err := s.gameRepo.UpsertGames(ctx, games, nil); err != nil {
		return fmt.Errorf("upsert plugin games: %w", err)
	}
	if err := s.gameRepo.MarkPluginGamesNotFoundExcept(ctx, integrationID, keepGameIDs); err != nil {
		return fmt.Errorf("mark plugin games not_found except: %w", err)
	}
	s.logger.Info("persisted plugin games", "integration_id", integrationID, "label", sourceLabel, "count", len(entries))
	return nil
}
