package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/go-chi/chi/v5"
)

func TestDecodedPathParamUnescapesLegacyGameIDs(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := decodedPathParam(r, "id")
		if err != nil {
			t.Fatalf("decodedPathParam returned error: %v", err)
		}
		if id != "scan:225d313af056fa3e" {
			t.Fatalf("id = %q, want %q", id, "scan:225d313af056fa3e")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/games/scan%3A225d313af056fa3e", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestNormalizeAchievementResultRecomputesCountsAndClearsLockedTimestamps(t *testing.T) {
	fetchedAt := time.Unix(1710000000, 0).UTC()
	raw := rawAchievementPluginResult{
		Source:         "game-source-steam",
		ExternalGameID: "220",
		TotalCount:     99,
		UnlockedCount:  99,
		TotalPoints:    999,
		EarnedPoints:   999,
		Achievements: []rawAchievementPluginEntry{
			{
				ExternalID: "a-1",
				Title:      "Unlocked One",
				Points:     10,
				Unlocked:   true,
				UnlockedAt: float64(1710000000),
			},
			{
				ExternalID: "a-2",
				Title:      "Still Locked",
				Points:     5,
				Unlocked:   false,
				UnlockedAt: "2024-03-09T16:00:00Z",
			},
		},
	}

	set, dto := normalizeAchievementResult("game-source-steam", "220", raw, fetchedAt)

	if set.TotalCount != 2 || dto.TotalCount != 2 {
		t.Fatalf("total_count = %d/%d, want 2", set.TotalCount, dto.TotalCount)
	}
	if set.UnlockedCount != 1 || dto.UnlockedCount != 1 {
		t.Fatalf("unlocked_count = %d/%d, want 1", set.UnlockedCount, dto.UnlockedCount)
	}
	if set.TotalPoints != 15 || dto.TotalPoints != 15 {
		t.Fatalf("total_points = %d/%d, want 15", set.TotalPoints, dto.TotalPoints)
	}
	if set.EarnedPoints != 10 || dto.EarnedPoints != 10 {
		t.Fatalf("earned_points = %d/%d, want 10", set.EarnedPoints, dto.EarnedPoints)
	}
	if dto.Achievements[0].UnlockedAt == "" {
		t.Fatal("expected unlocked achievement timestamp to be normalized to RFC3339")
	}
	if dto.Achievements[1].UnlockedAt != "" {
		t.Fatalf("locked achievement unlocked_at = %q, want empty", dto.Achievements[1].UnlockedAt)
	}
	if !set.Achievements[1].UnlockedAt.IsZero() {
		t.Fatal("locked achievement should not retain unlocked_at in cached set")
	}
}

func TestAchievementControllerGetAchievementsNormalizesMixedStatesAndCaches(t *testing.T) {
	game := &core.CanonicalGame{
		ID: "game-1",
		ExternalIDs: []core.ExternalID{
			{Source: "game-source-steam", ExternalID: "220"},
		},
		SourceGames: []*core.SourceGame{
			{
				ID:         "source-1",
				PluginID:   "game-source-steam",
				ExternalID: "220",
				Status:     "found",
			},
		},
	}
	store := &fakeGameStore{game: game}
	host := &fakePluginHost{
		provides: map[string][]string{
			"achievements.game.get": {"game-source-steam"},
		},
		results: map[string]rawAchievementPluginResult{
			"game-source-steam": {
				Source:         "game-source-steam",
				ExternalGameID: "220",
				UnlockedCount:  2,
				Achievements: []rawAchievementPluginEntry{
					{
						ExternalID: "ach-1",
						Title:      "Unlocked One",
						Points:     10,
						Unlocked:   true,
						UnlockedAt: float64(1710000000),
					},
					{
						ExternalID: "ach-2",
						Title:      "Locked Two",
						Points:     5,
						Unlocked:   false,
						UnlockedAt: "2024-03-09T16:00:00Z",
					},
				},
			},
		},
	}
	controller := NewAchievementController(store, host, noopLogger{}, nil)

	router := chi.NewRouter()
	router.Get("/api/games/{id}/achievements", controller.GetAchievements)

	req := httptest.NewRequest(http.MethodGet, "/api/games/game-1/achievements", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	var sets []AchievementSetDTO
	if err := json.Unmarshal(body, &sets); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("len(sets) = %d, want 1", len(sets))
	}
	set := sets[0]
	if set.UnlockedCount != 1 {
		t.Fatalf("unlocked_count = %d, want 1", set.UnlockedCount)
	}
	if set.TotalCount != 2 {
		t.Fatalf("total_count = %d, want 2", set.TotalCount)
	}
	if set.Achievements[0].UnlockedAt == "" {
		t.Fatal("expected unlocked achievement timestamp in response")
	}
	if set.Achievements[1].UnlockedAt != "" {
		t.Fatalf("locked achievement unlocked_at = %q, want empty", set.Achievements[1].UnlockedAt)
	}
	if len(store.cached) != 1 {
		t.Fatalf("cache writes = %d, want 1", len(store.cached))
	}
	if store.cached[0].sourceGameID != "source-1" {
		t.Fatalf("cached source_game_id = %q, want %q", store.cached[0].sourceGameID, "source-1")
	}
	if store.cached[0].set.UnlockedCount != 1 {
		t.Fatalf("cached unlocked_count = %d, want 1", store.cached[0].set.UnlockedCount)
	}
	if !store.cached[0].set.Achievements[1].UnlockedAt.IsZero() {
		t.Fatal("locked cached achievement should not have unlocked_at")
	}
}

type noopLogger struct{}

func (noopLogger) Info(string, ...any)         {}
func (noopLogger) Error(string, error, ...any) {}
func (noopLogger) Debug(string, ...any)        {}
func (noopLogger) Warn(string, ...any)         {}

type cachedAchievementCall struct {
	sourceGameID string
	set          *core.AchievementSet
}

type fakeGameStore struct {
	game   *core.CanonicalGame
	cached []cachedAchievementCall
}

func (f *fakeGameStore) PersistScanResults(context.Context, *core.ScanBatch) error {
	panic("unexpected call")
}
func (f *fakeGameStore) CacheAchievements(_ context.Context, sourceGameID string, set *core.AchievementSet) error {
	f.cached = append(f.cached, cachedAchievementCall{sourceGameID: sourceGameID, set: set})
	return nil
}
func (f *fakeGameStore) UpdateMediaAsset(context.Context, int, string, string) error {
	panic("unexpected call")
}
func (f *fakeGameStore) DeleteAllGames(context.Context) error { panic("unexpected call") }
func (f *fakeGameStore) GetCanonicalGames(context.Context) ([]*core.CanonicalGame, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetCanonicalGamesByIDs(context.Context, []string) ([]*core.CanonicalGame, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) CountVisibleCanonicalGames(context.Context) (int, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetVisibleCanonicalIDs(context.Context, int, int) ([]string, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetCanonicalGameByID(context.Context, string) (*core.CanonicalGame, error) {
	return f.game, nil
}
func (f *fakeGameStore) GetMediaAssetByID(context.Context, int) (*core.MediaAsset, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetSourceGamesForCanonical(context.Context, string) ([]*core.SourceGame, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetPendingMediaDownloads(context.Context, int) ([]*core.MediaAsset, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetCachedAchievements(context.Context, string, string) (*core.AchievementSet, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetExternalIDsForCanonical(context.Context, string) ([]core.ExternalID, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetLibraryStats(context.Context) (*core.LibraryStats, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetGamesByIntegrationID(context.Context, string, int) ([]core.GameListItem, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetEnrichedGamesByPluginID(context.Context, string, int) ([]core.GameListItem, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetFoundSourceGames(context.Context, []string) ([]*core.FoundSourceGame, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) DeleteGamesByIntegrationID(context.Context, string) error {
	panic("unexpected call")
}
func (f *fakeGameStore) SaveScanReport(context.Context, *core.ScanReport) error {
	panic("unexpected call")
}
func (f *fakeGameStore) GetScanReports(context.Context, int) ([]*core.ScanReport, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetScanReport(context.Context, string) (*core.ScanReport, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetSourceGameCountsByIntegration(context.Context) (map[string]int, error) {
	panic("unexpected call")
}

type fakePluginHost struct {
	provides map[string][]string
	results  map[string]rawAchievementPluginResult
}

func (f *fakePluginHost) Discover(context.Context) error { panic("unexpected call") }
func (f *fakePluginHost) Call(_ context.Context, pluginID, method string, _ any, result any) error {
	if method != "achievements.game.get" {
		panic("unexpected call")
	}
	payload, ok := f.results[pluginID]
	if !ok {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}
func (f *fakePluginHost) Close() error { return nil }
func (f *fakePluginHost) GetPluginIDs() []string {
	return nil
}
func (f *fakePluginHost) GetPlugin(string) (*core.Plugin, bool) { return nil, false }
func (f *fakePluginHost) ListPlugins() []plugins.PluginInfo     { return nil }
func (f *fakePluginHost) GetPluginIDsProviding(method string) []string {
	return f.provides[method]
}
