package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type achievementRefreshIntegrationRepo interface {
	List(ctx context.Context) ([]*core.Integration, error)
}

type AchievementRefreshCallbacks struct {
	SetTotal       func(total int)
	Progress       func(completed, total int, item string)
	ProgressDetail func(progress AchievementRefreshProgress)
	Waiting        func(wait AchievementRefreshWait)
	Warning        func(message string)
	Skipped        func(item string)
}

type AchievementRefreshProgress struct {
	Completed     int
	Total         int
	Item          string
	ProviderID    string
	ProviderLabel string
	Message       string
	WaitingUntil  string
}

type AchievementRefreshWait struct {
	Completed     int
	Total         int
	Item          string
	ProviderID    string
	ProviderLabel string
	Message       string
	WaitingUntil  string
	Delay         time.Duration
}

type AchievementRefreshResult struct {
	Targets int
	Success int
	Failed  int
	Skipped int
}

type refreshTarget struct {
	game      *core.CanonicalGame
	source    AchievementSource
	candidate AchievementQueryCandidate
}

type AchievementRefreshService struct {
	integrationRepo    achievementRefreshIntegrationRepo
	gameStore          core.GameStore
	pluginHost         achievementPluginHost
	achievementFetcher *AchievementFetchService
	sleeper            achievementRefreshSleeper
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
		sleeper:            realAchievementRefreshSleeper{},
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

	targets := make([]refreshTarget, 0)
	result := &AchievementRefreshResult{}
	policy := NewAchievementProviderRefreshPolicy(s.sleeper)
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
		if deferred, ok := policy.DeferredMessage(target.source.PluginID); ok {
			if err := s.saveRefreshState(ctx, target, core.AchievementRefreshStatusSkipped, deferred); err != nil {
				return nil, err
			}
			result.Skipped++
			if callbacks.Skipped != nil {
				callbacks.Skipped(target.game.Title)
			}
			s.emitProgress(callbacks, idx+1, result.Targets, target, deferred, "")
			continue
		}
		s.emitProgress(callbacks, idx, result.Targets, target, "", "")
		sets, errs, deferred, err := s.refreshTarget(ctx, policy, target, idx, result.Targets, callbacks)
		if err != nil {
			return nil, err
		}
		if deferred != "" {
			result.Skipped++
			if callbacks.Skipped != nil {
				callbacks.Skipped(target.game.Title)
			}
		} else if len(errs) > 0 {
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
		s.emitProgress(callbacks, idx+1, result.Targets, target, "", "")
	}

	return result, nil
}

func (s *AchievementRefreshService) refreshTarget(ctx context.Context, policy *AchievementProviderRefreshPolicy, target refreshTarget, completed, total int, callbacks AchievementRefreshCallbacks) ([]*core.AchievementSet, map[string]error, string, error) {
	for {
		if err := policy.WaitBeforeAttempt(ctx, target.source); err != nil {
			return nil, nil, "", err
		}
		sets, errs := s.achievementFetcher.FetchAndCacheWithCandidatesOptions(ctx, target.game, []AchievementSource{target.source}, map[string][]AchievementQueryCandidate{
			target.source.PluginID: {target.candidate},
		}, AchievementFetchOptions{PersistProviderFailures: false})
		if fatal := firstCacheError(errs); fatal != nil {
			return nil, nil, "", fatal
		}
		if len(errs) == 0 {
			return sets, nil, "", nil
		}
		providerErr := firstProviderError(errs)
		decision := policy.DecideAfterError(target.source, providerErr)
		if decision.Retry {
			waitingUntil := policy.Now().Add(decision.Delay).UTC().Format(time.RFC3339)
			if callbacks.Waiting != nil {
				callbacks.Waiting(AchievementRefreshWait{
					Completed:     completed,
					Total:         total,
					Item:          target.game.Title,
					ProviderID:    target.source.IntegrationID,
					ProviderLabel: target.source.Label,
					Message:       decision.Message,
					WaitingUntil:  waitingUntil,
					Delay:         decision.Delay,
				})
			}
			s.emitProgress(callbacks, completed, total, target, decision.Message, waitingUntil)
			if err := policy.Sleep(ctx, decision.Delay); err != nil {
				return nil, nil, "", err
			}
			continue
		}
		if decision.DeferProvider {
			if err := s.saveRefreshState(ctx, target, core.AchievementRefreshStatusSkipped, decision.Message); err != nil {
				return nil, nil, "", err
			}
			return nil, nil, decision.Message, nil
		}
		if err := s.saveRefreshState(ctx, target, core.AchievementRefreshStatusFailed, firstErrorMessage(errs)); err != nil {
			return nil, nil, "", err
		}
		return nil, errs, "", nil
	}
}

func (s *AchievementRefreshService) emitProgress(callbacks AchievementRefreshCallbacks, completed, total int, target refreshTarget, message, waitingUntil string) {
	if callbacks.ProgressDetail != nil {
		callbacks.ProgressDetail(AchievementRefreshProgress{
			Completed:     completed,
			Total:         total,
			Item:          target.game.Title,
			ProviderID:    target.source.IntegrationID,
			ProviderLabel: target.source.Label,
			Message:       message,
			WaitingUntil:  waitingUntil,
		})
		return
	}
	if callbacks.Progress != nil {
		callbacks.Progress(completed, total, target.game.Title)
	}
}

func (s *AchievementRefreshService) saveRefreshState(ctx context.Context, target refreshTarget, status core.AchievementRefreshStatus, message string) error {
	if err := s.gameStore.SaveAchievementRefreshState(ctx, &core.AchievementRefreshState{
		SourceGameID:    target.candidate.SourceGameID,
		IntegrationID:   target.candidate.IntegrationID,
		PluginID:        target.source.PluginID,
		ExternalGameID:  target.candidate.ExternalGameID,
		Status:          status,
		LastAttemptedAt: time.Now().UTC(),
		LastError:       message,
	}); err != nil {
		return fmt.Errorf("save achievement refresh state: %w", err)
	}
	return nil
}

type achievementRefreshSleeper interface {
	Now() time.Time
	Sleep(ctx context.Context, delay time.Duration) error
}

type realAchievementRefreshSleeper struct{}

func (realAchievementRefreshSleeper) Now() time.Time {
	return time.Now().UTC()
}

func (realAchievementRefreshSleeper) Sleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type AchievementProviderRefreshPolicy struct {
	sleeper achievementRefreshSleeper

	lastAttemptByProvider map[string]time.Time
	deferredByProvider    map[string]string
	rateLimitCount        map[string]int
}

type providerErrorDecision struct {
	Retry         bool
	DeferProvider bool
	Delay         time.Duration
	Message       string
}

const (
	retroAchievementsPluginID        = "retroachievements"
	retroAchievementsMinCallInterval = 2 * time.Second
	providerRateLimitFallbackDelay   = 5 * time.Minute
	maxProviderRateLimitRetries      = 1
)

func NewAchievementProviderRefreshPolicy(sleeper achievementRefreshSleeper) *AchievementProviderRefreshPolicy {
	if sleeper == nil {
		sleeper = realAchievementRefreshSleeper{}
	}
	return &AchievementProviderRefreshPolicy{
		sleeper:               sleeper,
		lastAttemptByProvider: make(map[string]time.Time),
		deferredByProvider:    make(map[string]string),
		rateLimitCount:        make(map[string]int),
	}
}

func (p *AchievementProviderRefreshPolicy) Now() time.Time {
	return p.sleeper.Now().UTC()
}

func (p *AchievementProviderRefreshPolicy) Sleep(ctx context.Context, delay time.Duration) error {
	return p.sleeper.Sleep(ctx, delay)
}

func (p *AchievementProviderRefreshPolicy) WaitBeforeAttempt(ctx context.Context, source AchievementSource) error {
	if source.PluginID != retroAchievementsPluginID {
		return nil
	}
	last, ok := p.lastAttemptByProvider[source.PluginID]
	if ok {
		next := last.Add(retroAchievementsMinCallInterval)
		if wait := next.Sub(p.Now()); wait > 0 {
			if err := p.Sleep(ctx, wait); err != nil {
				return err
			}
		}
	}
	p.lastAttemptByProvider[source.PluginID] = p.Now()
	return nil
}

func (p *AchievementProviderRefreshPolicy) DeferredMessage(pluginID string) (string, bool) {
	message, ok := p.deferredByProvider[pluginID]
	return message, ok
}

func (p *AchievementProviderRefreshPolicy) DecideAfterError(source AchievementSource, err error) providerErrorDecision {
	if err == nil || !isProviderRateLimitError(err) {
		return providerErrorDecision{}
	}
	delay := providerRetryDelay(err)
	label := source.Label
	if strings.TrimSpace(label) == "" {
		label = source.PluginID
	}
	message := fmt.Sprintf("%s is rate-limited. Waiting %s before retrying achievement refresh.", label, delay.Round(time.Second))
	p.rateLimitCount[source.PluginID]++
	if p.rateLimitCount[source.PluginID] <= maxProviderRateLimitRetries {
		return providerErrorDecision{Retry: true, Delay: delay, Message: message}
	}
	deferMessage := fmt.Sprintf("%s is rate-limited. Remaining achievement refresh work for this provider was deferred until a later refresh.", label)
	p.deferredByProvider[source.PluginID] = deferMessage
	return providerErrorDecision{DeferProvider: true, Message: deferMessage}
}

var retryAfterSecondsPattern = regexp.MustCompile(`retry_after_seconds=(\d+)`)

func isProviderRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "plugin error [rate_limited]") ||
		strings.Contains(msg, "status 429") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "rate limit")
}

func providerRetryDelay(err error) time.Duration {
	if err != nil {
		if match := retryAfterSecondsPattern.FindStringSubmatch(err.Error()); len(match) == 2 {
			var seconds int
			if _, scanErr := fmt.Sscanf(match[1], "%d", &seconds); scanErr == nil && seconds > 0 {
				return time.Duration(seconds) * time.Second
			}
		}
	}
	return providerRateLimitFallbackDelay
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

func firstProviderError(errs map[string]error) error {
	for _, err := range errs {
		var cacheErr *AchievementCacheError
		if errors.As(err, &cacheErr) {
			continue
		}
		if err != nil {
			return err
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
