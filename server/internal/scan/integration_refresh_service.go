package scan

import (
	"context"
	"fmt"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
)

type integrationRefreshPluginHost interface {
	Call(ctx context.Context, pluginID string, method string, params any, result any) error
	GetPlugin(pluginID string) (*core.Plugin, bool)
	GetPluginIDsProviding(method string) []string
}

type IntegrationRefreshCallbacks struct {
	SetPhase func(phase string, total int)
	Progress func(completed, total int, item string)
	Warning  func(message string)
}

type IntegrationRefreshService struct {
	integrationRepo    core.IntegrationRepository
	gameStore          core.GameStore
	pluginHost         integrationRefreshPluginHost
	mediaDownloadQueue core.MediaDownloadQueue
	metadataResolver   *MetadataResolver
	achievementFetcher *AchievementFetchService
	config             core.Configuration
	logger             core.Logger
}

func NewIntegrationRefreshService(
	integrationRepo core.IntegrationRepository,
	gameStore core.GameStore,
	pluginHost integrationRefreshPluginHost,
	mediaDownloadQueue core.MediaDownloadQueue,
	config core.Configuration,
	logger core.Logger,
) *IntegrationRefreshService {
	return &IntegrationRefreshService{
		integrationRepo:    integrationRepo,
		gameStore:          gameStore,
		pluginHost:         pluginHost,
		mediaDownloadQueue: mediaDownloadQueue,
		metadataResolver:   NewMetadataResolver(pluginHost, logger),
		achievementFetcher: NewAchievementFetchService(gameStore, pluginHost, logger),
		config:             config,
		logger:             logger,
	}
}

func (s *IntegrationRefreshService) RunIntegrationRefresh(ctx context.Context, integration *core.Integration, callbacks IntegrationRefreshCallbacks) error {
	if integration == nil || strings.TrimSpace(integration.ID) == "" {
		return fmt.Errorf("integration is required")
	}

	plugin, ok := s.pluginHost.GetPlugin(integration.PluginID)
	if !ok {
		return fmt.Errorf("plugin not found: %s", integration.PluginID)
	}

	hasMetadata := pluginProvides(plugin, metadataGameLookupMethod)
	hasAchievements := pluginProvides(plugin, achievementGameGetMethod)
	if !hasMetadata && !hasAchievements {
		return fmt.Errorf("integration %q has no refreshable derived data", integration.Label)
	}

	if err := s.validateIntegration(ctx, integration); err != nil {
		return err
	}

	var allIntegrations []*core.Integration
	var err error
	if hasMetadata {
		allIntegrations, err = s.integrationRepo.List(ctx)
		if err != nil {
			return fmt.Errorf("list integrations: %w", err)
		}
		if err := s.refreshMetadata(ctx, integration, allIntegrations, callbacks); err != nil {
			return err
		}
	}
	if hasAchievements {
		if err := s.refreshAchievements(ctx, integration, hasMetadata, callbacks); err != nil {
			return err
		}
	}
	return nil
}

func (s *IntegrationRefreshService) validateIntegration(ctx context.Context, integration *core.Integration) error {
	configMap, err := parseConfig(integration.ConfigJSON)
	if err != nil {
		return fmt.Errorf("invalid integration config: %w", err)
	}
	configMap = sourcescope.NormalizeConfig(integration.PluginID, configMap)

	var checkResult struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := s.pluginHost.Call(ctx, integration.PluginID, "plugin.check_config", map[string]any{
		"config":       configMap,
		"redirect_uri": s.oauthRedirectURI(integration.PluginID),
	}, &checkResult); err != nil {
		return fmt.Errorf("plugin validation failed: %w", err)
	}
	if checkResult.Status != "" && checkResult.Status != "ok" {
		message := strings.TrimSpace(checkResult.Message)
		if message == "" {
			message = checkResult.Status
		}
		return fmt.Errorf("integration is not refreshable: %s", message)
	}
	return nil
}

func (s *IntegrationRefreshService) oauthRedirectURI(pluginID string) string {
	port := ""
	if s.config != nil {
		port = s.config.Get("PORT")
	}
	if port == "" {
		port = "8900"
	}
	return fmt.Sprintf("http://localhost:%s/api/auth/callback/%s", port, pluginID)
}

func (s *IntegrationRefreshService) refreshMetadata(ctx context.Context, integration *core.Integration, allIntegrations []*core.Integration, callbacks IntegrationRefreshCallbacks) error {
	selectedSource, err := MetadataSourceFromIntegration(integration)
	if err != nil {
		return fmt.Errorf("metadata refresh config: %w", err)
	}
	allSources := s.metadataSourcesFromIntegrations(allIntegrations)
	if len(allSources) == 0 {
		allSources = []MetadataSource{selectedSource}
	}

	sourceGames, err := s.gameStore.GetFoundSourceGameRecords(ctx, nil)
	if err != nil {
		return fmt.Errorf("load known source records: %w", err)
	}
	if callbacks.SetPhase != nil {
		callbacks.SetPhase("refreshing_metadata", len(sourceGames))
	}

	updated := make([]*core.SourceGame, 0, len(sourceGames))
	for idx, sourceGame := range sourceGames {
		if callbacks.Progress != nil {
			callbacks.Progress(idx, len(sourceGames), sourceGame.RawTitle)
		}
		refreshed, warning, refreshErr := s.refreshSourceGameForMetadataProvider(ctx, sourceGame, selectedSource, allSources)
		if refreshErr != nil {
			return refreshErr
		}
		if warning != "" {
			if callbacks.Warning != nil {
				callbacks.Warning(warning)
			}
			continue
		}
		updated = append(updated, refreshed)
		if callbacks.Progress != nil {
			callbacks.Progress(idx+1, len(sourceGames), sourceGame.RawTitle)
		}
	}

	if len(updated) > 0 {
		if err := s.gameStore.SaveRefreshedMetadataProviderResults(ctx, updated); err != nil {
			return fmt.Errorf("persist provider refresh results: %w", err)
		}
		if s.mediaDownloadQueue != nil {
			if err := s.mediaDownloadQueue.EnqueuePending(ctx); err != nil {
				s.logger.Warn("integration refresh: enqueue pending media downloads failed", "error", err)
			}
		}
	}

	return nil
}

func (s *IntegrationRefreshService) refreshSourceGameForMetadataProvider(
	ctx context.Context,
	sourceGame *core.SourceGame,
	selectedSource MetadataSource,
	allSources []MetadataSource,
) (*core.SourceGame, string, error) {
	games, err := gamesForSourceRefresh([]*core.SourceGame{sourceGame})
	if err != nil || len(games) == 0 {
		return nil, "", fmt.Errorf("build refresh game for %s: %w", sourceGame.ID, err)
	}

	query := MetadataLookupQuery{
		Index:     0,
		Title:     sourceGame.RawTitle,
		Platform:  string(sourceGame.Platform),
		RootPath:  sourceGame.RootPath,
		GroupKind: string(sourceGame.GroupKind),
	}
	results := LookupMetadataSources(ctx, s.pluginHost, []MetadataSource{selectedSource}, []MetadataLookupQuery{query})
	if len(results) == 0 {
		return nil, "", nil
	}
	result := results[0]
	if result.Error != nil {
		return nil, fmt.Sprintf("%s: %s", sourceGame.RawTitle, result.Error.Error()), nil
	}

	refreshed := *sourceGame
	refreshed.ResolverMatches = mergeResolverMatchesForPlugin(sourceGame.ResolverMatches, selectedSource.PluginID, result.Matches)

	game := games[0]
	game.ResolverMatches = append([]core.ResolverMatch(nil), refreshed.ResolverMatches...)
	game.Media = nil
	if len(game.ResolverMatches) > 0 {
		runConsensus(game, allSources)
	}
	refreshed.ResolverMatches = append([]core.ResolverMatch(nil), game.ResolverMatches...)
	refreshed.Media = mergeMediaRefsForProvider(sourceGame.Media, selectedSource.PluginID, game)
	return &refreshed, "", nil
}

func (s *IntegrationRefreshService) refreshAchievements(ctx context.Context, integration *core.Integration, metadataCapable bool, callbacks IntegrationRefreshCallbacks) error {
	games, err := s.gameStore.GetCanonicalGames(ctx)
	if err != nil {
		return fmt.Errorf("load canonical games: %w", err)
	}

	type refreshTarget struct {
		game      *core.CanonicalGame
		candidate AchievementQueryCandidate
	}
	targets := make([]refreshTarget, 0)
	for _, game := range games {
		if game == nil {
			continue
		}
		candidate, ok := achievementCandidateForIntegration(game, integration, metadataCapable)
		if !ok {
			continue
		}
		targets = append(targets, refreshTarget{game: game, candidate: candidate})
	}

	if callbacks.SetPhase != nil {
		callbacks.SetPhase("refreshing_achievements", len(targets))
	}
	for idx, target := range targets {
		if callbacks.Progress != nil {
			callbacks.Progress(idx, len(targets), target.game.Title)
		}
		_, errs := s.achievementFetcher.FetchAndCacheWithCandidates(ctx, target.game, []string{integration.PluginID}, map[string]AchievementQueryCandidate{
			integration.PluginID: target.candidate,
		})
		if err := errs[integration.PluginID]; err != nil && callbacks.Warning != nil {
			callbacks.Warning(fmt.Sprintf("%s: %s", target.game.Title, err.Error()))
		}
		if callbacks.Progress != nil {
			callbacks.Progress(idx+1, len(targets), target.game.Title)
		}
	}

	return nil
}

func (s *IntegrationRefreshService) metadataSourcesFromIntegrations(integrations []*core.Integration) []MetadataSource {
	metaPluginIDs := s.pluginHost.GetPluginIDsProviding(metadataGameLookupMethod)
	if len(metaPluginIDs) == 0 {
		return nil
	}
	metaSet := make(map[string]bool, len(metaPluginIDs))
	for _, pluginID := range metaPluginIDs {
		metaSet[pluginID] = true
	}

	sources := make([]MetadataSource, 0, len(integrations))
	for _, integration := range integrations {
		if integration == nil || !metaSet[integration.PluginID] {
			continue
		}
		source, err := MetadataSourceFromIntegration(integration)
		if err != nil {
			s.logger.Warn("integration refresh: skipping invalid metadata provider", "integration_id", integration.ID, "error", err)
			continue
		}
		sources = append(sources, source)
	}
	return sources
}

func mergeResolverMatchesForPlugin(existing []core.ResolverMatch, pluginID string, matches []MetadataLookupMatch) []core.ResolverMatch {
	merged := make([]core.ResolverMatch, 0, len(existing)+len(matches))
	for _, match := range existing {
		if match.PluginID == pluginID {
			continue
		}
		merged = append(merged, match)
	}
	for _, match := range matches {
		merged = append(merged, matchToResolver(pluginID, match))
	}
	return merged
}

func mergeMediaRefsForProvider(existing []core.MediaRef, pluginID string, game *core.Game) []core.MediaRef {
	seen := make(map[string]bool)
	merged := make([]core.MediaRef, 0, len(existing))
	add := func(ref core.MediaRef) {
		key := strings.Join([]string{string(ref.Type), ref.Source, ref.URL}, "|")
		if strings.TrimSpace(ref.URL) == "" || seen[key] {
			return
		}
		seen[key] = true
		merged = append(merged, ref)
	}

	for _, ref := range existing {
		if ref.Source == pluginID {
			continue
		}
		add(ref)
	}
	for _, ref := range gameMediaToRefs(game) {
		add(ref)
	}
	return merged
}

func achievementCandidateForIntegration(game *core.CanonicalGame, integration *core.Integration, metadataCapable bool) (AchievementQueryCandidate, bool) {
	if game == nil || integration == nil {
		return AchievementQueryCandidate{}, false
	}
	if !metadataCapable {
		for _, sourceGame := range game.SourceGames {
			if sourceGame == nil || sourceGame.Status != "found" {
				continue
			}
			if sourceGame.IntegrationID == integration.ID && sourceGame.PluginID == integration.PluginID && sourceGame.ExternalID != "" {
				return AchievementQueryCandidate{
					ExternalGameID: sourceGame.ExternalID,
					SourceGameID:   sourceGame.ID,
				}, true
			}
		}
		return AchievementQueryCandidate{}, false
	}

	candidates := BuildAchievementQueryCandidates(game, []string{integration.PluginID})
	candidate, ok := candidates[integration.PluginID]
	return candidate, ok
}
