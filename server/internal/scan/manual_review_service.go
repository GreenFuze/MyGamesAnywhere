package scan

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan/scanner"
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

func (s *manualReviewService) Redetect(ctx context.Context, candidateID string) (*core.ManualReviewRedetectResult, error) {
	if strings.TrimSpace(candidateID) == "" {
		return nil, fmt.Errorf("%w: candidate id is required", core.ErrManualReviewSelectionInvalid)
	}

	candidate, err := s.gameStore.GetManualReviewCandidate(ctx, candidateID)
	if err != nil {
		return nil, fmt.Errorf("get manual review candidate: %w", err)
	}
	if candidate == nil || candidate.Status != "found" {
		archived, archiveErr := s.archivedRedetectCandidateExists(ctx, candidateID)
		if archiveErr != nil {
			return nil, archiveErr
		}
		if archived {
			return nil, fmt.Errorf("%w: review_state is %q", core.ErrManualReviewCandidateNotEligible, core.ManualReviewStateNotAGame)
		}
		return nil, core.ErrManualReviewCandidateNotFound
	}
	if err := validateRedetectCandidate(candidate); err != nil {
		return nil, err
	}

	metaSources, err := s.strictManualReviewMetadataSources(ctx)
	if err != nil {
		return nil, err
	}
	return s.redetectCandidate(ctx, candidate, metaSources)
}

func (s *manualReviewService) archivedRedetectCandidateExists(ctx context.Context, candidateID string) (bool, error) {
	candidates, err := s.gameStore.ListManualReviewCandidates(ctx, core.ManualReviewScopeArchive, 0)
	if err != nil {
		return false, fmt.Errorf("list archived manual review candidates: %w", err)
	}
	for _, candidate := range candidates {
		if candidate != nil && candidate.ID == candidateID {
			return true, nil
		}
	}
	return false, nil
}

func (s *manualReviewService) RedetectActive(ctx context.Context) (*core.ManualReviewRedetectBatchResult, error) {
	candidates, err := s.gameStore.ListManualReviewCandidates(ctx, core.ManualReviewScopeActive, 0)
	if err != nil {
		return nil, fmt.Errorf("list active manual review candidates: %w", err)
	}
	metaSources, err := s.strictManualReviewMetadataSources(ctx)
	if err != nil {
		return nil, err
	}

	result := &core.ManualReviewRedetectBatchResult{
		Results: []core.ManualReviewRedetectResult{},
	}
	for _, candidate := range candidates {
		if candidate == nil || candidate.Status != "found" || candidate.ReviewState != core.ManualReviewStatePending {
			continue
		}
		result.Attempted++
		item, err := s.redetectCandidate(ctx, candidate, metaSources)
		if err != nil {
			result.FailedCandidateID = candidate.ID
			result.Error = err.Error()
			return result, err
		}
		result.Results = append(result.Results, *item)
		switch item.Status {
		case core.ManualReviewRedetectStatusMatched:
			result.Matched++
		case core.ManualReviewRedetectStatusUnidentified, core.ManualReviewRedetectStatusPending:
			result.Unidentified++
		}
	}
	return result, nil
}

func (s *manualReviewService) strictManualReviewMetadataSources(ctx context.Context) ([]MetadataSource, error) {
	integrations, err := s.integrationRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}
	return BuildMetadataLookupSources(s.pluginDiscovery, integrations, s.logger, true)
}

func (s *manualReviewService) redetectCandidate(
	ctx context.Context,
	candidate *core.ManualReviewCandidate,
	metaSources []MetadataSource,
) (*core.ManualReviewRedetectResult, error) {
	if err := validateRedetectCandidate(candidate); err != nil {
		return nil, err
	}

	game := redetectGameFromCandidate(ctx, candidate, s.logger)
	summary, err := s.metadataResolver.EnrichWithPolicy(
		ctx,
		candidate.IntegrationID,
		[]*core.Game{game},
		metaSources,
		manualReviewMetadataFailurePolicy,
	)
	if err != nil {
		return nil, err
	}

	result := &core.ManualReviewRedetectResult{
		CandidateID:   candidate.ID,
		Status:        core.ManualReviewRedetectStatusUnidentified,
		MatchCount:    len(game.ResolverMatches),
		ProviderCount: len(metaSources),
	}
	if summary != nil && summary.Identified == 0 {
		return result, nil
	}
	if game.Status != "identified" || len(game.ResolverMatches) == 0 {
		return result, nil
	}

	sourceGame := sourceGameForRedetectedCandidate(candidate, game)
	if err := s.refreshCoordinator.persistRefreshedSourceGames(ctx, []*core.SourceGame{sourceGame}, []*core.Game{game}); err != nil {
		return nil, fmt.Errorf("persist redetected candidate: %w", err)
	}

	updated, err := s.gameStore.GetManualReviewCandidate(ctx, candidate.ID)
	if err != nil {
		return nil, fmt.Errorf("get redetected manual review candidate: %w", err)
	}
	if updated == nil {
		return nil, core.ErrManualReviewCandidateNotFound
	}
	if updated.ReviewState == core.ManualReviewStateMatched || len(updated.ReviewReasons) == 0 {
		result.Status = core.ManualReviewRedetectStatusMatched
	} else {
		result.Status = core.ManualReviewRedetectStatusPending
	}
	result.MatchCount = updated.ResolverMatchCount
	return result, nil
}

func validateRedetectCandidate(candidate *core.ManualReviewCandidate) error {
	if candidate == nil || candidate.Status != "found" {
		return core.ErrManualReviewCandidateNotFound
	}
	if candidate.ReviewState != core.ManualReviewStatePending {
		return fmt.Errorf("%w: review_state is %q", core.ErrManualReviewCandidateNotEligible, candidate.ReviewState)
	}
	return nil
}

func redetectGameFromCandidate(ctx context.Context, candidate *core.ManualReviewCandidate, logger core.Logger) *core.Game {
	files := manualReviewCandidateFileEntries(candidate)
	if len(files) == 0 {
		return manualReviewCandidateToGame(candidate)
	}

	groups, err := scanner.New(logger).ScanFiles(ctx, files)
	if err != nil {
		logger.Warn("manual review redetect: scanner failed; falling back to candidate snapshot", "candidate_id", candidate.ID, "error", err)
		return manualReviewCandidateToGame(candidate)
	}
	if len(groups) != 1 {
		return manualReviewCandidateToGame(candidate)
	}

	games := buildGames(candidate.IntegrationID, candidate.PluginID, groups)
	if len(games) != 1 || games[0] == nil {
		return manualReviewCandidateToGame(candidate)
	}
	game := games[0]
	game.ID = candidate.ID
	game.IntegrationID = candidate.IntegrationID
	game.ExternalIDs = candidateExternalIDs(candidate)
	if game.LastSeenAt == nil {
		game.LastSeenAt = candidate.LastSeenAt
	}
	for i := range game.Files {
		game.Files[i].GameID = candidate.ID
	}
	return game
}

func manualReviewCandidateFileEntries(candidate *core.ManualReviewCandidate) []core.FileEntry {
	if candidate == nil || len(candidate.Files) == 0 {
		return nil
	}
	files := make([]core.FileEntry, 0, len(candidate.Files))
	for _, file := range candidate.Files {
		path := filepath.ToSlash(strings.TrimSpace(file.Path))
		if path == "" {
			continue
		}
		name := strings.TrimSpace(file.FileName)
		if name == "" {
			name = filepath.Base(path)
		}
		var modTime time.Time
		if file.ModifiedAt != nil {
			modTime = *file.ModifiedAt
		}
		files = append(files, core.FileEntry{
			Path:     path,
			Name:     name,
			IsDir:    file.IsDir,
			Size:     file.Size,
			ModTime:  modTime,
			ObjectID: file.ObjectID,
			Revision: file.Revision,
		})
	}
	return files
}

func sourceGameForRedetectedCandidate(candidate *core.ManualReviewCandidate, game *core.Game) *core.SourceGame {
	files := append([]core.GameFile(nil), game.Files...)
	if len(files) == 0 {
		files = append([]core.GameFile(nil), candidate.Files...)
	}
	return &core.SourceGame{
		ID:            candidate.ID,
		IntegrationID: candidate.IntegrationID,
		PluginID:      candidate.PluginID,
		ExternalID:    candidate.ExternalID,
		RawTitle:      game.RawTitle,
		Platform:      game.Platform,
		Kind:          game.Kind,
		GroupKind:     game.GroupKind,
		RootPath:      game.RootPath,
		URL:           candidate.URL,
		Status:        "found",
		ReviewState:   core.ManualReviewStatePending,
		LastSeenAt:    candidate.LastSeenAt,
		Files:         files,
	}
}

func buildManualReviewMetadataSources(
	discovery PluginDiscovery,
	integrations []*core.Integration,
	logger core.Logger,
) []MetadataSource {
	sources, _ := BuildMetadataLookupSources(discovery, integrations, logger, false)
	return sources
}

type manualReviewMetadataSourceBuilder struct {
	discovery PluginDiscovery
	logger    core.Logger
	strict    bool
}

func BuildMetadataLookupSources(
	discovery PluginDiscovery,
	integrations []*core.Integration,
	logger core.Logger,
	strict bool,
) ([]MetadataSource, error) {
	return manualReviewMetadataSourceBuilder{
		discovery: discovery,
		logger:    logger,
		strict:    strict,
	}.Build(integrations)
}

func (b manualReviewMetadataSourceBuilder) Build(integrations []*core.Integration) ([]MetadataSource, error) {
	if b.discovery == nil {
		return nil, nil
	}
	metaPluginIDs := b.discovery.GetPluginIDsProviding(metadataGameLookupMethod)
	if len(metaPluginIDs) == 0 {
		return nil, nil
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
		source, err := MetadataSourceFromIntegration(integ)
		if err != nil {
			if b.strict {
				return nil, fmt.Errorf("%w: invalid metadata config for integration %q: %v", core.ErrMetadataProvidersUnavailable, integ.ID, err)
			}
			b.logger.Warn("manual review: bad metadata config", "integration_id", integ.ID, "error", err)
			continue
		}
		sources = append(sources, source)
	}
	return sources, nil
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
		ExternalIDs:   candidateExternalIDs(candidate),
	}
}

func candidateExternalIDs(candidate *core.ManualReviewCandidate) []core.ExternalID {
	if candidate == nil || strings.TrimSpace(candidate.ExternalID) == "" {
		return nil
	}
	return []core.ExternalID{{
		Source:     candidate.PluginID,
		ExternalID: candidate.ExternalID,
		URL:        candidate.URL,
	}}
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
