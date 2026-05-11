package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type achievementRefreshIntegrationRepo interface {
	List(ctx context.Context) ([]*core.Integration, error)
}

type AchievementRefreshCallbacks struct {
	SetTotal func(total int)
	Progress func(completed, total int, item string)
	Warning  func(message string)
	Skipped  func(item string)
}

type AchievementRefreshResult struct {
	Targets int
	Success int
	Failed  int
	Skipped int
}

type AchievementRefreshService struct {
	integrationRepo    achievementRefreshIntegrationRepo
	gameStore          core.GameStore
	pluginHost         achievementPluginHost
	achievementFetcher *AchievementFetchService
	logger             core.Logger
}

func NewAchievementRefreshService(
	integrationRepo achievementRefreshIntegrationRepo,
	gameStore core.GameStore,
	pluginHost achievementPluginHost,
	logger core.Logger,
) *AchievementRefreshService {
	return &AchievementRefreshService{
		integrationRepo:    integrationRepo,
		gameStore:          gameStore,
		pluginHost:         pluginHost,
		achievementFetcher: NewAchievementFetchService(gameStore, pluginHost, logger),
		logger:             logger,
	}
}

func (s *AchievementRefreshService) RefreshAll(ctx context.Context, callbacks AchievementRefreshCallbacks) (*AchievementRefreshResult, error) {
	if s.integrationRepo == nil {
		return nil, fmt.Errorf("integration repository is required")
	}
	sources, err := s.configuredAchievementSources(ctx)
	if err != nil {
		return nil, err
	}
	games, err := s.gameStore.GetCanonicalGames(ctx)
	if err != nil {
		return nil, fmt.Errorf("load canonical games: %w", err)
	}

	type refreshTarget struct {
		game      *core.CanonicalGame
		source    AchievementSource
		candidate AchievementQueryCandidate
	}

	targets := make([]refreshTarget, 0)
	result := &AchievementRefreshResult{}
	for _, game := range games {
		if game == nil {
			continue
		}
		gameTargets := 0
		for _, source := range sources {
			candidates := BuildAchievementQueryCandidates(game, []string{source.PluginID})[source.PluginID]
			for _, candidate := range candidates {
				if candidate.IntegrationID == "" {
					candidate.IntegrationID = source.IntegrationID
				}
				if candidate.IntegrationLabel == "" {
					candidate.IntegrationLabel = source.Label
				}
				targets = append(targets, refreshTarget{game: game, source: source, candidate: candidate})
				gameTargets++
			}
		}
		if gameTargets == 0 {
			result.Skipped++
			if callbacks.Skipped != nil {
				callbacks.Skipped(game.Title)
			}
		}
	}

	result.Targets = len(targets)
	if callbacks.SetTotal != nil {
		callbacks.SetTotal(result.Targets)
	}

	for idx, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if callbacks.Progress != nil {
			callbacks.Progress(idx, result.Targets, target.game.Title)
		}
		sets, errs := s.achievementFetcher.FetchAndCacheWithCandidates(ctx, target.game, []AchievementSource{target.source}, map[string][]AchievementQueryCandidate{
			target.source.PluginID: {target.candidate},
		})
		if fatal := firstCacheError(errs); fatal != nil {
			return nil, fatal
		}
		if len(errs) > 0 {
			result.Failed++
			if callbacks.Warning != nil {
				callbacks.Warning(fmt.Sprintf("%s: %s", target.game.Title, firstErrorMessage(errs)))
			}
		} else if len(sets) == 0 {
			result.Skipped++
			if callbacks.Skipped != nil {
				callbacks.Skipped(target.game.Title)
			}
		} else {
			result.Success++
		}
		if callbacks.Progress != nil {
			callbacks.Progress(idx+1, result.Targets, target.game.Title)
		}
	}

	return result, nil
}

func (s *AchievementRefreshService) configuredAchievementSources(ctx context.Context) ([]AchievementSource, error) {
	pluginIDs := s.pluginHost.GetPluginIDsProviding(achievementGameGetMethod)
	if len(pluginIDs) == 0 {
		return nil, nil
	}
	provides := make(map[string]struct{}, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		if strings.TrimSpace(pluginID) != "" {
			provides[pluginID] = struct{}{}
		}
	}

	integrations, err := s.integrationRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list achievement integrations: %w", err)
	}
	sources := make([]AchievementSource, 0, len(integrations))
	seen := map[string]struct{}{}
	for _, integration := range integrations {
		if integration == nil {
			continue
		}
		if _, ok := provides[integration.PluginID]; !ok {
			continue
		}
		configMap, err := decodeAchievementRefreshConfig(integration.ConfigJSON)
		if err != nil {
			return nil, fmt.Errorf("decode achievement integration config %s: %w", integration.ID, err)
		}
		key := integration.ID
		if key == "" {
			key = integration.PluginID
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		sources = append(sources, AchievementSource{
			IntegrationID: integration.ID,
			Label:         integration.Label,
			PluginID:      integration.PluginID,
			Config:        configMap,
		})
	}
	return sources, nil
}

func decodeAchievementRefreshConfig(configJSON string) (map[string]any, error) {
	if strings.TrimSpace(configJSON) == "" {
		return map[string]any{}, nil
	}
	var configMap map[string]any
	if err := json.Unmarshal([]byte(configJSON), &configMap); err != nil {
		return nil, err
	}
	if configMap == nil {
		configMap = map[string]any{}
	}
	return configMap, nil
}

func firstCacheError(errs map[string]error) error {
	for _, err := range errs {
		var cacheErr *AchievementCacheError
		if errors.As(err, &cacheErr) {
			return cacheErr
		}
	}
	return nil
}

func firstErrorMessage(errs map[string]error) string {
	for _, err := range errs {
		if err != nil {
			return err.Error()
		}
	}
	return "achievement refresh failed"
}
