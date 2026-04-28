package scan

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const achievementGameGetMethod = "achievements.game.get"

type achievementPluginHost interface {
	Call(ctx context.Context, pluginID string, method string, params any, result any) error
	GetPluginIDsProviding(method string) []string
}

type AchievementQueryCandidate struct {
	PluginID         string
	ExternalGameID   string
	SourceGameID     string
	SourceTitle      string
	Platform         string
	IntegrationID    string
	IntegrationLabel string
	SourcePluginID   string
	priority         int
}

type AchievementSource struct {
	IntegrationID string
	Label         string
	PluginID      string
	Config        map[string]any
}

type AchievementFetchService struct {
	gameStore  core.GameStore
	pluginHost achievementPluginHost
	logger     core.Logger
}

func NewAchievementFetchService(gameStore core.GameStore, pluginHost achievementPluginHost, logger core.Logger) *AchievementFetchService {
	return &AchievementFetchService{
		gameStore:  gameStore,
		pluginHost: pluginHost,
		logger:     logger,
	}
}

func (s *AchievementFetchService) FetchAndCacheForPlugins(ctx context.Context, game *core.CanonicalGame, pluginIDs []string) ([]*core.AchievementSet, map[string]error) {
	if game == nil || len(pluginIDs) == 0 {
		return nil, nil
	}

	sources := make([]AchievementSource, 0, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		sources = append(sources, AchievementSource{PluginID: pluginID})
	}
	return s.FetchAndCacheForSources(ctx, game, sources)
}

func (s *AchievementFetchService) FetchAndCacheForSources(ctx context.Context, game *core.CanonicalGame, sources []AchievementSource) ([]*core.AchievementSet, map[string]error) {
	if game == nil || len(sources) == 0 {
		return nil, nil
	}

	pluginIDs := make([]string, 0, len(sources))
	for _, source := range sources {
		if strings.TrimSpace(source.PluginID) == "" {
			continue
		}
		pluginIDs = append(pluginIDs, source.PluginID)
	}
	candidatesByPlugin := BuildAchievementQueryCandidates(game, pluginIDs)
	return s.FetchAndCacheWithCandidates(ctx, game, sources, candidatesByPlugin)
}

func (s *AchievementFetchService) FetchAndCacheWithCandidates(ctx context.Context, game *core.CanonicalGame, sources []AchievementSource, candidatesByPlugin map[string][]AchievementQueryCandidate) ([]*core.AchievementSet, map[string]error) {
	if game == nil || len(sources) == 0 {
		return nil, nil
	}

	var sets []*core.AchievementSet
	var errs map[string]error
	fetched := make(map[string]*core.AchievementSet)
	for _, source := range sources {
		pluginID := source.PluginID
		candidates := candidatesByPlugin[pluginID]
		if len(candidates) == 0 {
			continue
		}

		for _, candidate := range candidates {
			if candidate.PluginID == "" {
				candidate.PluginID = pluginID
			}
			if candidate.IntegrationID == "" {
				candidate.IntegrationID = source.IntegrationID
			}
			if candidate.IntegrationLabel == "" {
				candidate.IntegrationLabel = source.Label
			}
			fetchKey := pluginID + "|" + candidate.ExternalGameID
			baseSet, ok := fetched[fetchKey]
			if !ok {
				var result rawAchievementPluginResult
				if err := s.pluginHost.Call(ctx, pluginID, achievementGameGetMethod, map[string]any{
					"external_game_id": candidate.ExternalGameID,
					"config":           source.Config,
				}, &result); err != nil {
					if errs == nil {
						errs = make(map[string]error)
					}
					errs[fetchKey] = err
					continue
				}
				baseSet = normalizeAchievementResult(pluginID, candidate.ExternalGameID, result, time.Now())
				fetched[fetchKey] = baseSet
			}

			set := cloneAchievementSetWithCandidate(baseSet, candidate)
			cacheSourceGameID := candidate.SourceGameID
			if cacheSourceGameID == "" {
				cacheSourceGameID = FindAchievementCacheSourceGameID(game, set.Source, set.ExternalGameID)
				set.SourceGameID = cacheSourceGameID
			}
			if cacheSourceGameID != "" {
				if err := s.gameStore.CacheAchievements(ctx, cacheSourceGameID, set); err != nil {
					s.logger.Error("cache achievements", err, "plugin_id", pluginID, "game_id", game.ID, "source_game_id", cacheSourceGameID)
				}
			}
			sets = append(sets, set)
		}
	}
	return sets, errs
}

func BuildAchievementQueryCandidates(game *core.CanonicalGame, pluginIDs []string) map[string][]AchievementQueryCandidate {
	candidates := make(map[string][]AchievementQueryCandidate, len(pluginIDs))
	if game == nil || len(pluginIDs) == 0 {
		return candidates
	}

	wanted := make(map[string]struct{}, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		if strings.TrimSpace(pluginID) == "" {
			continue
		}
		wanted[pluginID] = struct{}{}
	}

	selected := make(map[string]AchievementQueryCandidate)
	addCandidate := func(pluginID string, sg *core.SourceGame, externalID string, priority int) {
		if sg == nil || strings.TrimSpace(pluginID) == "" || strings.TrimSpace(externalID) == "" {
			return
		}
		key := pluginID + "|" + sg.ID
		candidate := AchievementQueryCandidate{
			PluginID:       pluginID,
			ExternalGameID: externalID,
			SourceGameID:   sg.ID,
			SourceTitle:    sg.RawTitle,
			Platform:       string(sg.Platform),
			IntegrationID:  sg.IntegrationID,
			SourcePluginID: sg.PluginID,
			priority:       priority,
		}
		if existing, ok := selected[key]; ok && existing.priority >= priority {
			return
		}
		selected[key] = candidate
	}

	for _, sg := range game.SourceGames {
		if sg == nil || sg.Status != "found" {
			continue
		}
		if _, ok := wanted[sg.PluginID]; ok && sg.ExternalID != "" {
			addCandidate(sg.PluginID, sg, sg.ExternalID, 2)
		}
		for _, match := range sg.ResolverMatches {
			if _, ok := wanted[match.PluginID]; !ok || match.ExternalID == "" {
				continue
			}
			if match.Outvoted && !match.ManualSelection {
				continue
			}
			priority := 2
			if match.ManualSelection {
				priority = 3
			}
			addCandidate(match.PluginID, sg, match.ExternalID, priority)
		}
	}

	keys := make([]string, 0, len(selected))
	for key := range selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	seenSet := make(map[string]bool)
	for _, key := range keys {
		candidate := selected[key]
		setKey := candidate.PluginID + "|" + candidate.ExternalGameID + "|" + candidate.SourceGameID
		if seenSet[setKey] {
			continue
		}
		seenSet[setKey] = true
		candidates[candidate.PluginID] = append(candidates[candidate.PluginID], candidate)
	}

	return candidates
}

func cloneAchievementSetWithCandidate(base *core.AchievementSet, candidate AchievementQueryCandidate) *core.AchievementSet {
	if base == nil {
		return nil
	}
	cloned := *base
	cloned.GameID = candidate.SourceGameID
	cloned.SourceGameID = candidate.SourceGameID
	cloned.SourceTitle = candidate.SourceTitle
	cloned.Platform = candidate.Platform
	cloned.IntegrationID = candidate.IntegrationID
	cloned.IntegrationLabel = candidate.IntegrationLabel
	cloned.PluginID = candidate.PluginID
	cloned.Achievements = append([]core.Achievement(nil), base.Achievements...)
	return &cloned
}

func FindAchievementCacheSourceGameID(game *core.CanonicalGame, source, externalGameID string) string {
	for _, sg := range game.SourceGames {
		if sg == nil || sg.Status != "found" {
			continue
		}
		if sg.PluginID == source && sg.ExternalID == externalGameID {
			return sg.ID
		}
	}
	for _, sg := range game.SourceGames {
		if sg == nil || sg.Status != "found" {
			continue
		}
		for _, match := range sg.ResolverMatches {
			if match.Outvoted {
				continue
			}
			if match.PluginID == source && match.ExternalID == externalGameID {
				return sg.ID
			}
		}
	}
	return ""
}

type rawAchievementPluginEntry struct {
	ExternalID   string  `json:"external_id"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	LockedIcon   string  `json:"locked_icon"`
	UnlockedIcon string  `json:"unlocked_icon"`
	Points       int     `json:"points"`
	Rarity       float64 `json:"rarity"`
	Unlocked     bool    `json:"unlocked"`
	UnlockedAt   any     `json:"unlocked_at"`
}

type rawAchievementPluginResult struct {
	Source         string                      `json:"source"`
	ExternalGameID string                      `json:"external_game_id"`
	TotalCount     int                         `json:"total_count"`
	UnlockedCount  int                         `json:"unlocked_count"`
	TotalPoints    int                         `json:"total_points"`
	EarnedPoints   int                         `json:"earned_points"`
	Achievements   []rawAchievementPluginEntry `json:"achievements"`
}

func normalizeAchievementResult(pluginID, externalGameID string, raw rawAchievementPluginResult, fetchedAt time.Time) *core.AchievementSet {
	source := raw.Source
	if source == "" {
		source = pluginID
	}
	canonicalExternalGameID := raw.ExternalGameID
	if canonicalExternalGameID == "" {
		canonicalExternalGameID = externalGameID
	}

	set := &core.AchievementSet{
		Source:         source,
		ExternalGameID: canonicalExternalGameID,
		Achievements:   make([]core.Achievement, 0, len(raw.Achievements)),
		FetchedAt:      fetchedAt.UTC(),
	}

	for _, a := range raw.Achievements {
		unlockedAt, hasUnlockedAt := parseAchievementUnlockedAt(a.UnlockedAt)
		if !a.Unlocked {
			unlockedAt = time.Time{}
			hasUnlockedAt = false
		}

		achievement := core.Achievement{
			ExternalID:   a.ExternalID,
			Title:        a.Title,
			Description:  a.Description,
			LockedIcon:   a.LockedIcon,
			UnlockedIcon: a.UnlockedIcon,
			Points:       a.Points,
			Rarity:       a.Rarity,
			Unlocked:     a.Unlocked,
		}
		if hasUnlockedAt {
			achievement.UnlockedAt = unlockedAt
		}
		set.Achievements = append(set.Achievements, achievement)

		set.TotalCount++
		if a.Points > 0 {
			set.TotalPoints += a.Points
		}
		if a.Unlocked {
			set.UnlockedCount++
			if a.Points > 0 {
				set.EarnedPoints += a.Points
			}
		}
	}

	return set
}

func parseAchievementUnlockedAt(raw any) (time.Time, bool) {
	switch v := raw.(type) {
	case nil:
		return time.Time{}, false
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return time.Time{}, false
		}
		if numeric, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return unixAchievementTime(numeric)
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return parsed.UTC(), true
		}
		if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return parsed.UTC(), true
		}
		if parsed, err := time.Parse("2006-01-02 15:04:05", trimmed); err == nil {
			return parsed.UTC(), true
		}
		return time.Time{}, false
	case float64:
		return unixAchievementTime(int64(v))
	case int64:
		return unixAchievementTime(v)
	case int:
		return unixAchievementTime(int64(v))
	default:
		return time.Time{}, false
	}
}

func unixAchievementTime(raw int64) (time.Time, bool) {
	if raw <= 0 {
		return time.Time{}, false
	}
	seconds := raw
	if raw > 9999999999 {
		seconds = raw / 1000
	}
	return time.Unix(seconds, 0).UTC(), true
}
