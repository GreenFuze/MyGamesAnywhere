package scan

import (
	"context"
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
	ExternalGameID string
	SourceGameID   string
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

	candidatesByPlugin := BuildAchievementQueryCandidates(game, pluginIDs)
	return s.FetchAndCacheWithCandidates(ctx, game, pluginIDs, candidatesByPlugin)
}

func (s *AchievementFetchService) FetchAndCacheWithCandidates(ctx context.Context, game *core.CanonicalGame, pluginIDs []string, candidatesByPlugin map[string]AchievementQueryCandidate) ([]*core.AchievementSet, map[string]error) {
	if game == nil || len(pluginIDs) == 0 {
		return nil, nil
	}

	var sets []*core.AchievementSet
	var errs map[string]error
	for _, pluginID := range pluginIDs {
		candidate, ok := candidatesByPlugin[pluginID]
		if !ok {
			continue
		}

		var result rawAchievementPluginResult
		if err := s.pluginHost.Call(ctx, pluginID, achievementGameGetMethod, map[string]any{
			"external_game_id": candidate.ExternalGameID,
		}, &result); err != nil {
			if errs == nil {
				errs = make(map[string]error)
			}
			errs[pluginID] = err
			continue
		}

		set := normalizeAchievementResult(pluginID, candidate.ExternalGameID, result, time.Now())
		cacheSourceGameID := candidate.SourceGameID
		if cacheSourceGameID == "" {
			cacheSourceGameID = FindAchievementCacheSourceGameID(game, set.Source, set.ExternalGameID)
		}
		if cacheSourceGameID != "" {
			if err := s.gameStore.CacheAchievements(ctx, cacheSourceGameID, set); err != nil {
				s.logger.Error("cache achievements", err, "plugin_id", pluginID, "game_id", game.ID, "source_game_id", cacheSourceGameID)
			}
		}
		sets = append(sets, set)
	}
	return sets, errs
}

func BuildAchievementQueryCandidates(game *core.CanonicalGame, pluginIDs []string) map[string]AchievementQueryCandidate {
	candidates := make(map[string]AchievementQueryCandidate, len(pluginIDs))
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

	for _, sg := range game.SourceGames {
		if sg == nil || sg.Status != "found" {
			continue
		}
		if _, ok := wanted[sg.PluginID]; !ok || sg.ExternalID == "" {
			continue
		}
		if _, exists := candidates[sg.PluginID]; exists {
			continue
		}
		candidates[sg.PluginID] = AchievementQueryCandidate{
			ExternalGameID: sg.ExternalID,
			SourceGameID:   sg.ID,
		}
	}

	type scoredCandidate struct {
		candidate AchievementQueryCandidate
		priority  int
	}

	scored := make(map[string]scoredCandidate, len(pluginIDs))
	for _, sg := range game.SourceGames {
		if sg == nil || sg.Status != "found" {
			continue
		}
		for _, match := range sg.ResolverMatches {
			if _, ok := wanted[match.PluginID]; !ok || match.ExternalID == "" {
				continue
			}
			priority := 1
			if match.ManualSelection {
				priority = 3
			} else if !match.Outvoted {
				priority = 2
			}
			existing, exists := scored[match.PluginID]
			if exists && existing.priority >= priority {
				continue
			}
			scored[match.PluginID] = scoredCandidate{
				candidate: AchievementQueryCandidate{
					ExternalGameID: match.ExternalID,
					SourceGameID:   sg.ID,
				},
				priority: priority,
			}
		}
	}

	for pluginID, candidate := range scored {
		candidates[pluginID] = candidate.candidate
	}
	return candidates
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
