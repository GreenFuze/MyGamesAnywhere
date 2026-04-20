package scan

import (
	"context"
	"fmt"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

var (
	fullScanMetadataFailurePolicy     = MetadataFailurePolicy{Name: "full_scan", Strict: false}
	refreshMetadataFailurePolicy      = MetadataFailurePolicy{Name: "refresh", Strict: true}
	manualReviewMetadataFailurePolicy = MetadataFailurePolicy{Name: "manual_review", Strict: true}
)

type metadataRefreshCoordinator struct {
	gameStore          core.GameStore
	mediaDownloadQueue core.MediaDownloadQueue
	metadataResolver   *MetadataResolver
	logger             core.Logger
}

func newMetadataRefreshCoordinator(
	gameStore core.GameStore,
	mediaDownloadQueue core.MediaDownloadQueue,
	metadataResolver *MetadataResolver,
	logger core.Logger,
) *metadataRefreshCoordinator {
	return &metadataRefreshCoordinator{
		gameStore:          gameStore,
		mediaDownloadQueue: mediaDownloadQueue,
		metadataResolver:   metadataResolver,
		logger:             logger,
	}
}

func (c *metadataRefreshCoordinator) enrichDiscoveredGames(
	ctx context.Context,
	integrationID string,
	games []*core.Game,
	metadataSources []MetadataSource,
) (*MetadataExecutionSummary, error) {
	return c.metadataResolver.EnrichWithPolicy(ctx, integrationID, games, metadataSources, fullScanMetadataFailurePolicy)
}

func (c *metadataRefreshCoordinator) refreshExistingSourceGames(
	ctx context.Context,
	integrationID string,
	sourceGames []*core.SourceGame,
	metadataSources []MetadataSource,
) ([]*core.Game, error) {
	games, err := gamesForSourceRefresh(sourceGames)
	if err != nil {
		return nil, err
	}

	if _, err := c.metadataResolver.EnrichWithPolicy(ctx, integrationID, games, metadataSources, refreshMetadataFailurePolicy); err != nil {
		return nil, err
	}

	if err := c.persistRefreshedSourceGames(ctx, sourceGames, games); err != nil {
		return nil, err
	}
	return games, nil
}

func (c *metadataRefreshCoordinator) applyManualReviewSelection(
	ctx context.Context,
	integrationID string,
	sourceGame *core.SourceGame,
	game *core.Game,
	metadataSources []MetadataSource,
) error {
	if sourceGame == nil || game == nil {
		return fmt.Errorf("source game and game are required")
	}

	applyUnifiedFields(game, metadataSources)
	game.Status = "identified"
	if len(metadataSources) > 0 {
		if _, err := c.metadataResolver.FillWithPolicy(ctx, integrationID, []*core.Game{game}, metadataSources, manualReviewMetadataFailurePolicy); err != nil {
			return err
		}
		runConsensus(game, metadataSources)
	}

	return c.persistRefreshedSourceGames(ctx, []*core.SourceGame{sourceGame}, []*core.Game{game})
}

func (c *metadataRefreshCoordinator) persistRefreshedSourceGames(
	ctx context.Context,
	sourceGames []*core.SourceGame,
	games []*core.Game,
) error {
	batch, err := refreshedSourceGamesToBatch(sourceGames, games)
	if err != nil {
		return err
	}
	if err := c.gameStore.PersistScanResults(ctx, batch); err != nil {
		return err
	}
	if c.mediaDownloadQueue != nil {
		if err := c.mediaDownloadQueue.EnqueuePending(ctx); err != nil {
			c.logger.Warn("metadata refresh: enqueue pending media downloads failed", "error", err)
		}
	}
	return nil
}

func gamesForSourceRefresh(sourceGames []*core.SourceGame) ([]*core.Game, error) {
	if len(sourceGames) == 0 {
		return nil, fmt.Errorf("%w: no source games supplied", core.ErrMetadataRefreshNoEligible)
	}

	games := make([]*core.Game, 0, len(sourceGames))
	for _, sourceGame := range sourceGames {
		if sourceGame == nil {
			continue
		}
		files := append([]core.GameFile(nil), sourceGame.Files...)
		externalIDs := []core.ExternalID{{
			Source:     sourceGame.PluginID,
			ExternalID: sourceGame.ExternalID,
			URL:        sourceGame.URL,
		}}
		games = append(games, &core.Game{
			ID:            sourceGame.ID,
			Title:         sourceGame.RawTitle,
			RawTitle:      sourceGame.RawTitle,
			Platform:      sourceGame.Platform,
			Kind:          sourceGame.Kind,
			GroupKind:     sourceGame.GroupKind,
			RootPath:      sourceGame.RootPath,
			IntegrationID: sourceGame.IntegrationID,
			Status:        "found",
			LastSeenAt:    sourceGame.LastSeenAt,
			Files:         files,
			ExternalIDs:   externalIDs,
		})
	}
	if len(games) == 0 {
		return nil, fmt.Errorf("%w: no source games supplied", core.ErrMetadataRefreshNoEligible)
	}
	return games, nil
}

func refreshedSourceGamesToBatch(
	sourceGames []*core.SourceGame,
	games []*core.Game,
) (*core.ScanBatch, error) {
	if len(sourceGames) == 0 || len(games) == 0 {
		return nil, fmt.Errorf("%w: no source games supplied", core.ErrMetadataRefreshNoEligible)
	}

	sourceByID := make(map[string]*core.SourceGame, len(sourceGames))
	var integrationID string
	for _, sourceGame := range sourceGames {
		if sourceGame == nil || sourceGame.ID == "" {
			continue
		}
		if integrationID == "" {
			integrationID = sourceGame.IntegrationID
		}
		if sourceGame.IntegrationID != integrationID {
			return nil, fmt.Errorf("mixed integration ids in refresh batch: %q and %q", integrationID, sourceGame.IntegrationID)
		}
		sourceByID[sourceGame.ID] = sourceGame
	}
	if len(sourceByID) == 0 {
		return nil, fmt.Errorf("%w: no source games supplied", core.ErrMetadataRefreshNoEligible)
	}

	batch := &core.ScanBatch{
		IntegrationID:   integrationID,
		SourceGames:     make([]*core.SourceGame, 0, len(games)),
		ResolverMatches: make(map[string][]core.ResolverMatch, len(games)),
		MediaItems:      make(map[string][]core.MediaRef, len(games)),
	}

	for _, game := range games {
		if game == nil {
			continue
		}
		base, ok := sourceByID[game.ID]
		if !ok {
			return nil, fmt.Errorf("refresh source game %q missing base record", game.ID)
		}

		persistedSource := &core.SourceGame{
			ID:            base.ID,
			IntegrationID: base.IntegrationID,
			PluginID:      base.PluginID,
			ExternalID:    base.ExternalID,
			RawTitle:      game.RawTitle,
			Platform:      game.Platform,
			Kind:          game.Kind,
			GroupKind:     game.GroupKind,
			RootPath:      game.RootPath,
			URL:           base.URL,
			Status:        "found",
			ReviewState:   base.ReviewState,
			LastSeenAt:    base.LastSeenAt,
			ManualReview:  base.ManualReview,
			Files:         append([]core.GameFile(nil), base.Files...),
		}
		batch.SourceGames = append(batch.SourceGames, persistedSource)

		if len(game.ResolverMatches) > 0 {
			batch.ResolverMatches[game.ID] = append([]core.ResolverMatch(nil), game.ResolverMatches...)
		}

		mediaRefs := gameMediaToRefs(game)
		if len(mediaRefs) > 0 {
			batch.MediaItems[game.ID] = mediaRefs
		}
	}

	return batch, nil
}
