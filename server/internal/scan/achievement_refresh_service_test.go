package scan

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestAchievementRefreshRetriesRateLimitedProviderThenCachesSuccess(t *testing.T) {
	store := newRefreshTestGameStore(oneAchievementGame("source-a", "Retro Game", "123"))
	host := &refreshTestPluginHost{
		errs: []error{errors.New("plugin error [RATE_LIMITED]: RetroAchievements rate limited request; retry_after_seconds=7")},
	}
	service := newRefreshTestService(store, host)
	sleeper := &refreshTestSleeper{now: time.Date(2026, 5, 13, 8, 0, 0, 0, time.UTC)}
	service.sleeper = sleeper

	var waits []AchievementRefreshWait
	result, err := service.RefreshAll(context.Background(), AchievementRefreshCallbacks{
		Waiting: func(wait AchievementRefreshWait) {
			waits = append(waits, wait)
		},
	})
	if err != nil {
		t.Fatalf("RefreshAll returned error: %v", err)
	}
	if result.Success != 1 || result.Failed != 0 || result.Skipped != 0 {
		t.Fatalf("result = %+v, want one success", result)
	}
	if host.calls != 2 {
		t.Fatalf("plugin calls = %d, want retry after rate limit", host.calls)
	}
	if len(waits) != 1 || waits[0].Delay != 7*time.Second {
		t.Fatalf("waits = %+v, want one 7s wait", waits)
	}
	if store.cacheCount != 1 {
		t.Fatalf("cache count = %d, want 1", store.cacheCount)
	}
	if got := lastRefreshStateStatus(store); got != core.AchievementRefreshStatusSuccess {
		t.Fatalf("last state = %q, want success", got)
	}
}

func TestAchievementRefreshDefersProviderAfterRepeatedRateLimits(t *testing.T) {
	store := newRefreshTestGameStore(
		oneAchievementGame("source-a", "Retro Game A", "123"),
		oneAchievementGame("source-b", "Retro Game B", "456"),
	)
	host := &refreshTestPluginHost{
		errs: []error{
			errors.New("plugin error [RATE_LIMITED]: RetroAchievements rate limited request; retry_after_seconds=3"),
			errors.New("plugin error [RATE_LIMITED]: RetroAchievements rate limited request; retry_after_seconds=3"),
		},
	}
	service := newRefreshTestService(store, host)
	service.sleeper = &refreshTestSleeper{now: time.Date(2026, 5, 13, 8, 0, 0, 0, time.UTC)}

	result, err := service.RefreshAll(context.Background(), AchievementRefreshCallbacks{})
	if err != nil {
		t.Fatalf("RefreshAll returned error: %v", err)
	}
	if result.Success != 0 || result.Failed != 0 || result.Skipped != 2 {
		t.Fatalf("result = %+v, want two deferred skips", result)
	}
	if host.calls != 2 {
		t.Fatalf("plugin calls = %d, want provider to stop after second rate limit", host.calls)
	}
	if len(store.states) != 2 {
		t.Fatalf("state count = %d, want 2", len(store.states))
	}
	for _, state := range store.states {
		if state.Status != core.AchievementRefreshStatusSkipped {
			t.Fatalf("state = %+v, want skipped", state)
		}
		if state.LastError == "" || containsRawHTML(state.LastError) {
			t.Fatalf("state last_error = %q, want clean deferred message", state.LastError)
		}
	}
}

func TestAchievementRefreshPacesRetroAchievementsCalls(t *testing.T) {
	store := newRefreshTestGameStore(
		oneAchievementGame("source-a", "Retro Game A", "123"),
		oneAchievementGame("source-b", "Retro Game B", "456"),
	)
	host := &refreshTestPluginHost{}
	service := newRefreshTestService(store, host)
	sleeper := &refreshTestSleeper{now: time.Date(2026, 5, 13, 8, 0, 0, 0, time.UTC)}
	service.sleeper = sleeper

	result, err := service.RefreshAll(context.Background(), AchievementRefreshCallbacks{})
	if err != nil {
		t.Fatalf("RefreshAll returned error: %v", err)
	}
	if result.Success != 2 {
		t.Fatalf("success = %d, want 2", result.Success)
	}
	if len(sleeper.sleeps) != 1 || sleeper.sleeps[0] != retroAchievementsMinCallInterval {
		t.Fatalf("sleeps = %+v, want one %s pacing sleep", sleeper.sleeps, retroAchievementsMinCallInterval)
	}
}

func newRefreshTestService(store *refreshTestGameStore, host *refreshTestPluginHost) *AchievementRefreshService {
	return NewAchievementRefreshService(
		refreshTestIntegrationRepo{integrations: []*core.Integration{{
			ID:       "ra-integration",
			Label:    "RetroAchievements",
			PluginID: retroAchievementsPluginID,
		}}},
		store,
		host,
		refreshTestLogger{},
	)
}

func oneAchievementGame(sourceID, title, externalID string) *core.CanonicalGame {
	return &core.CanonicalGame{
		ID:    "game-" + sourceID,
		Title: title,
		SourceGames: []*core.SourceGame{{
			ID:       sourceID,
			RawTitle: title,
			Platform: core.PlatformGenesis,
			PluginID: "game-source-smb",
			Status:   "found",
			ResolverMatches: []core.ResolverMatch{{
				PluginID:   retroAchievementsPluginID,
				Title:      title,
				ExternalID: externalID,
			}},
		}},
	}
}

type refreshTestIntegrationRepo struct {
	integrations []*core.Integration
}

func (r refreshTestIntegrationRepo) List(context.Context) ([]*core.Integration, error) {
	return r.integrations, nil
}

type refreshTestGameStore struct {
	core.GameStore
	games      []*core.CanonicalGame
	states     []core.AchievementRefreshState
	cacheCount int
}

func newRefreshTestGameStore(games ...*core.CanonicalGame) *refreshTestGameStore {
	return &refreshTestGameStore{games: games}
}

func (s *refreshTestGameStore) GetCanonicalGames(context.Context) ([]*core.CanonicalGame, error) {
	return s.games, nil
}

func (s *refreshTestGameStore) SaveAchievementRefreshState(_ context.Context, state *core.AchievementRefreshState) error {
	s.states = append(s.states, *state)
	return nil
}

func (s *refreshTestGameStore) CacheAchievements(context.Context, string, *core.AchievementSet) error {
	s.cacheCount++
	return nil
}

type refreshTestPluginHost struct {
	errs  []error
	calls int
}

func (h *refreshTestPluginHost) GetPluginIDsProviding(method string) []string {
	if method == achievementGameGetMethod {
		return []string{retroAchievementsPluginID}
	}
	return nil
}

func (h *refreshTestPluginHost) Call(_ context.Context, _ string, _ string, _ any, result any) error {
	h.calls++
	if len(h.errs) > 0 {
		err := h.errs[0]
		h.errs = h.errs[1:]
		return err
	}
	out := result.(*rawAchievementPluginResult)
	*out = rawAchievementPluginResult{
		Source:         retroAchievementsPluginID,
		ExternalGameID: "123",
		Achievements: []rawAchievementPluginEntry{{
			ExternalID: "1",
			Title:      "Complete level one",
		}},
	}
	return nil
}

type refreshTestSleeper struct {
	now    time.Time
	sleeps []time.Duration
}

func (s *refreshTestSleeper) Now() time.Time {
	return s.now
}

func (s *refreshTestSleeper) Sleep(_ context.Context, delay time.Duration) error {
	s.sleeps = append(s.sleeps, delay)
	s.now = s.now.Add(delay)
	return nil
}

func lastRefreshStateStatus(store *refreshTestGameStore) core.AchievementRefreshStatus {
	if len(store.states) == 0 {
		return ""
	}
	return store.states[len(store.states)-1].Status
}

func containsRawHTML(value string) bool {
	return strings.Contains(value, "<html>") || strings.Contains(value, "<script>")
}

type refreshTestLogger struct{}

func (refreshTestLogger) Info(string, ...any)         {}
func (refreshTestLogger) Error(string, error, ...any) {}
func (refreshTestLogger) Debug(string, ...any)        {}
func (refreshTestLogger) Warn(string, ...any)         {}
