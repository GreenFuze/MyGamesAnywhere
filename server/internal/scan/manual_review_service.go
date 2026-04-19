package scan

import (
	"context"
	"fmt"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type manualReviewService struct {
	pluginDiscovery    PluginDiscovery
	integrationRepo    core.IntegrationRepository
	gameStore          core.GameStore
	mediaDownloadQueue core.MediaDownloadQueue
	metadataResolver   *MetadataResolver
	refreshCoordinator *metadataRefreshCoordinator
	logger             core.Logger
}

func NewManualReviewService(
	caller PluginCaller,
	discovery PluginDiscovery,
	integrationRepo core.IntegrationRepository,
	gameStore core.GameStore,
	mediaDownloadQueue core.MediaDownloadQueue,
	logger core.Logger,
) core.ManualReviewService {
	resolver := NewMetadataResolver(caller, logger)
	return &manualReviewService{
		pluginDiscovery:    discovery,
		integrationRepo:    integrationRepo,
		gameStore:          gameStore,
		mediaDownloadQueue: mediaDownloadQueue,
		metadataResolver:   resolver,
		refreshCoordinator: newMetadataRefreshCoordinator(gameStore, mediaDownloadQueue, resolver, logger),
		logger:             logger,
	}
}

func (s *manualReviewService) Apply(ctx context.Context, candidateID string, selection core.ManualReviewSelection) error {
	if strings.TrimSpace(candidateID) == "" {
		return fmt.Errorf("%w: candidate id is required", core.ErrManualReviewSelectionInvalid)
	}
	if strings.TrimSpace(selection.ProviderPluginID) == "" {
		return fmt.Errorf("%w: provider_plugin_id is required", core.ErrManualReviewSelectionInvalid)
	}
	if strings.TrimSpace(selection.ExternalID) == "" {
		return fmt.Errorf("%w: external_id is required", core.ErrManualReviewSelectionInvalid)
	}

	candidate, err := s.gameStore.GetManualReviewCandidate(ctx, candidateID)
	if err != nil {
		return fmt.Errorf("get manual review candidate: %w", err)
	}
	if candidate == nil || candidate.Status != "found" {
		return core.ErrManualReviewCandidateNotFound
	}

	integrations, err := s.integrationRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("list integrations: %w", err)
	}
	metaSources := buildManualReviewMetadataSources(s.pluginDiscovery, integrations, s.logger)

	game := manualReviewCandidateToGame(candidate)
	game.ResolverMatches = []core.ResolverMatch{manualReviewSelectionToResolverMatch(selection)}

	sourceGame := &core.SourceGame{
		ID:            candidate.ID,
		IntegrationID: candidate.IntegrationID,
		PluginID:      candidate.PluginID,
		ExternalID:    candidate.ExternalID,
		RawTitle:      candidate.RawTitle,
		Platform:      game.Platform,
		Kind:          game.Kind,
		GroupKind:     candidate.GroupKind,
		RootPath:      candidate.RootPath,
		URL:           candidate.URL,
		Status:        candidate.Status,
		ReviewState:   core.ManualReviewStateMatched,
		LastSeenAt:    candidate.LastSeenAt,
		ManualReview: &core.ManualReviewDecision{
			State:    core.ManualReviewStateMatched,
			Selected: &selection,
		},
		Files: append([]core.GameFile(nil), candidate.Files...),
	}

	if err := s.refreshCoordinator.applyManualReviewSelection(ctx, candidate.IntegrationID, sourceGame, game, metaSources); err != nil {
		return fmt.Errorf("save manual review result: %w", err)
	}
	return nil
}

func buildManualReviewMetadataSources(
	discovery PluginDiscovery,
	integrations []*core.Integration,
	logger core.Logger,
) []MetadataSource {
	metaPluginIDs := discovery.GetPluginIDsProviding(metadataGameLookupMethod)
	if len(metaPluginIDs) == 0 {
		return nil
	}
	metaSet := make(map[string]bool, len(metaPluginIDs))
	for _, id := range metaPluginIDs {
		metaSet[id] = true
	}

	var sources []MetadataSource
	for _, integ := range integrations {
		if integ == nil || !metaSet[integ.PluginID] {
			continue
		}
		config, err := parseConfig(integ.ConfigJSON)
		if err != nil {
			logger.Warn("manual review: bad metadata config", "integration_id", integ.ID, "error", err)
			continue
		}
		sources = append(sources, MetadataSource{
			IntegrationID: integ.ID,
			Label:         integ.Label,
			PluginID:      integ.PluginID,
			Config:        config,
		})
	}
	return sources
}

func manualReviewCandidateToGame(candidate *core.ManualReviewCandidate) *core.Game {
	files := append([]core.GameFile(nil), candidate.Files...)
	return &core.Game{
		ID:            candidate.ID,
		Title:         candidate.RawTitle,
		RawTitle:      candidate.RawTitle,
		Platform:      candidate.Platform,
		Kind:          candidate.Kind,
		GroupKind:     candidate.GroupKind,
		RootPath:      candidate.RootPath,
		IntegrationID: candidate.IntegrationID,
		Status:        "found",
		Files:         files,
	}
}

func manualReviewSelectionToResolverMatch(selection core.ManualReviewSelection) core.ResolverMatch {
	var media []core.MediaItem
	if strings.TrimSpace(selection.ImageURL) != "" {
		media = append(media, core.MediaItem{
			Type:   core.MediaTypeCover,
			URL:    strings.TrimSpace(selection.ImageURL),
			Source: strings.TrimSpace(selection.ProviderPluginID),
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
		Media:           media,
		Rating:          selection.Rating,
		MaxPlayers:      selection.MaxPlayers,
		ManualSelection: true,
	}
}

func gameMediaToRefs(game *core.Game) []core.MediaRef {
	if game == nil {
		return nil
	}
	seen := map[string]bool{}
	refs := make([]core.MediaRef, 0, len(game.Media))
	addMedia := func(media core.MediaItem) {
		if strings.TrimSpace(media.URL) == "" || seen[media.URL] {
			return
		}
		seen[media.URL] = true
		refs = append(refs, core.MediaRef{
			Type:   media.Type,
			URL:    media.URL,
			Source: media.Source,
			Width:  media.Width,
			Height: media.Height,
		})
	}
	for _, media := range game.Media {
		addMedia(media)
	}
	for _, match := range game.ResolverMatches {
		for _, media := range match.Media {
			addMedia(media)
		}
	}
	return refs
}
