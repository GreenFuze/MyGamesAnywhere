package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

type fakeGameMetadataRefreshService struct {
	game *core.CanonicalGame
	err  error
}

func (f *fakeGameMetadataRefreshService) RefreshGameMetadata(context.Context, string) (*core.CanonicalGame, error) {
	return f.game, f.err
}

type fakeGameDeletionService struct {
	result *core.DeleteSourceGameResult
	err    error
}

func (f *fakeGameDeletionService) DeleteSourceGame(context.Context, string, string) (*core.DeleteSourceGameResult, error) {
	return f.result, f.err
}

func TestGameControllerRefreshMetadataReturnsRefreshedGame(t *testing.T) {
	controller := NewGameController(
		&fakeGameStore{},
		&fakeGameMetadataRefreshService{
			game: &core.CanonicalGame{
				ID:    "game-1",
				Title: "Refreshed Game",
			},
		},
		nil,
		nil,
		nil,
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/games/{id}/refresh-metadata", controller.RefreshMetadata)

	req := httptest.NewRequest(http.MethodPost, "/api/games/game-1/refresh-metadata", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var item GameDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if item.ID != "game-1" {
		t.Fatalf("id = %q, want %q", item.ID, "game-1")
	}
	if item.Title != "Refreshed Game" {
		t.Fatalf("title = %q, want %q", item.Title, "Refreshed Game")
	}
}

func TestGameControllerRefreshMetadataMapsFastFailErrors(t *testing.T) {
	tests := []struct {
		name       string
		serviceErr error
		wantStatus int
	}{
		{
			name:       "no eligible source records",
			serviceErr: core.ErrMetadataRefreshNoEligible,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "providers unavailable",
			serviceErr: core.ErrMetadataProvidersUnavailable,
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			controller := NewGameController(
				&fakeGameStore{},
				&fakeGameMetadataRefreshService{err: tc.serviceErr},
				nil,
				nil,
				nil,
				noopLogger{},
			)

			router := chi.NewRouter()
			router.Post("/api/games/{id}/refresh-metadata", controller.RefreshMetadata)

			req := httptest.NewRequest(http.MethodPost, "/api/games/game-1/refresh-metadata", bytes.NewBufferString(`{}`))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
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

func TestGameControllerCoverOverrideReturnsUpdatedGame(t *testing.T) {
	store := &fakeGameStore{game: &core.CanonicalGame{
		ID:    "game-1",
		Title: "Game One",
		CoverOverride: &core.MediaRef{
			AssetID: 42,
			Type:    core.MediaTypeArtwork,
			URL:     "https://example.com/artwork.png",
		},
	}}
	controller := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.Put("/api/games/{id}/cover-override", controller.SetCoverOverride)

	req := httptest.NewRequest(http.MethodPut, "/api/games/game-1/cover-override", bytes.NewBufferString(`{"media_asset_id":42}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.setCoverOverrideID != "game-1" || store.setCoverOverrideAsset != 42 {
		t.Fatalf("cover override call = %q/%d, want game-1/42", store.setCoverOverrideID, store.setCoverOverrideAsset)
	}
	var resp GameDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.CoverOverride == nil || resp.CoverOverride.AssetID != 42 {
		t.Fatalf("cover_override = %+v, want asset 42", resp.CoverOverride)
	}
}

func TestGameControllerHoverOverrideReturnsUpdatedGame(t *testing.T) {
	store := &fakeGameStore{game: &core.CanonicalGame{
		ID:    "game-1",
		Title: "Game One",
		HoverOverride: &core.MediaRef{
			AssetID: 77,
			Type:    core.MediaTypeScreenshot,
			URL:     "https://example.com/hover.png",
		},
	}}
	controller := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.Put("/api/games/{id}/hover-override", controller.SetHoverOverride)

	req := httptest.NewRequest(http.MethodPut, "/api/games/game-1/hover-override", bytes.NewBufferString(`{"media_asset_id":77}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.setHoverOverrideID != "game-1" || store.setHoverOverrideAsset != 77 {
		t.Fatalf("hover override call = %q/%d, want game-1/77", store.setHoverOverrideID, store.setHoverOverrideAsset)
	}
	var resp GameDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.HoverOverride == nil || resp.HoverOverride.AssetID != 77 {
		t.Fatalf("hover_override = %+v, want asset 77", resp.HoverOverride)
	}
}

func TestGameControllerBackgroundOverrideReturnsUpdatedGame(t *testing.T) {
	store := &fakeGameStore{game: &core.CanonicalGame{
		ID:    "game-1",
		Title: "Game One",
		BackgroundOverride: &core.MediaRef{
			AssetID: 91,
			Type:    core.MediaTypeBackground,
			URL:     "https://example.com/background.png",
		},
	}}
	controller := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.Put("/api/games/{id}/background-override", controller.SetBackgroundOverride)

	req := httptest.NewRequest(http.MethodPut, "/api/games/game-1/background-override", bytes.NewBufferString(`{"media_asset_id":91}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.setBackgroundOverrideID != "game-1" || store.setBackgroundOverrideAsset != 91 {
		t.Fatalf("background override call = %q/%d, want game-1/91", store.setBackgroundOverrideID, store.setBackgroundOverrideAsset)
	}
	var resp GameDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.BackgroundOverride == nil || resp.BackgroundOverride.AssetID != 91 {
		t.Fatalf("background_override = %+v, want asset 91", resp.BackgroundOverride)
	}
}

func TestGameControllerSetFavoriteReturnsUpdatedGame(t *testing.T) {
	store := &fakeGameStore{game: &core.CanonicalGame{
		ID:       "game-1",
		Title:    "Game One",
		Favorite: false,
	}}
	controller := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.Put("/api/games/{id}/favorite", controller.SetFavorite)

	req := httptest.NewRequest(http.MethodPut, "/api/games/game-1/favorite", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.setFavoriteID != "game-1" {
		t.Fatalf("favorite call = %q, want game-1", store.setFavoriteID)
	}
	var resp GameDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Favorite {
		t.Fatalf("favorite = %v, want true", resp.Favorite)
	}
}

func TestGameControllerClearFavoriteReturnsUpdatedGame(t *testing.T) {
	store := &fakeGameStore{game: &core.CanonicalGame{
		ID:       "game-1",
		Title:    "Game One",
		Favorite: true,
	}}
	controller := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.Delete("/api/games/{id}/favorite", controller.ClearFavorite)

	req := httptest.NewRequest(http.MethodDelete, "/api/games/game-1/favorite", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.clearFavoriteID != "game-1" {
		t.Fatalf("clear favorite call = %q, want game-1", store.clearFavoriteID)
	}
	var resp GameDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Favorite {
		t.Fatalf("favorite = %v, want false", resp.Favorite)
	}
}

func TestGameControllerSetFavoriteReturnsNotFoundForUnknownGame(t *testing.T) {
	controller := NewGameController(&fakeGameStore{}, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.Put("/api/games/{id}/favorite", controller.SetFavorite)

	req := httptest.NewRequest(http.MethodPut, "/api/games/missing/favorite", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestGameControllerAchievementsDashboardReturnsCachedOnlySummary(t *testing.T) {
	store := &fakeGameStore{achievementDashboard: &core.CachedAchievementsDashboard{
		Totals: core.AchievementSummary{SourceCount: 1, TotalCount: 10, UnlockedCount: 4},
		Systems: []core.CachedAchievementSystemSummary{{
			Source:        "retroachievements",
			GameCount:     1,
			TotalCount:    10,
			UnlockedCount: 4,
		}},
		Games: []core.CachedAchievementGameSummary{{
			Game: &core.CanonicalGame{
				ID:    "game-1",
				Title: "Game One",
				AchievementSummary: &core.AchievementSummary{
					SourceCount:   1,
					TotalCount:    10,
					UnlockedCount: 4,
				},
			},
			Systems: []core.CachedAchievementSystemSummary{{
				Source:        "retroachievements",
				GameCount:     1,
				TotalCount:    10,
				UnlockedCount: 4,
			}},
		}},
	}}
	controller := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.Get("/api/achievements", controller.AchievementsDashboard)

	req := httptest.NewRequest(http.MethodGet, "/api/achievements", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp AchievementsDashboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Totals.TotalCount != 10 || len(resp.Systems) != 1 || len(resp.Games) != 1 {
		t.Fatalf("dashboard = %+v, want cached totals, one system, one game", resp)
	}
}

func TestGameControllerAchievementsExplorerReturnsCachedSetsOnly(t *testing.T) {
	store := &fakeGameStore{achievementExplorer: &core.CachedAchievementsExplorer{
		Games: []core.CachedAchievementGameExplorer{{
			Game: &core.CanonicalGame{
				ID:    "game-1",
				Title: "Game One",
				AchievementSummary: &core.AchievementSummary{
					SourceCount:   1,
					TotalCount:    2,
					UnlockedCount: 1,
				},
			},
			Systems: []core.AchievementSet{{
				Source:         "retroachievements",
				ExternalGameID: "ra-1",
				TotalCount:     2,
				UnlockedCount:  1,
				Achievements: []core.Achievement{
					{ExternalID: "ach-1", Title: "Alpha", Unlocked: true, UnlockedAt: time.Unix(1710000000, 0).UTC()},
					{ExternalID: "ach-2", Title: "Beta", Unlocked: false},
				},
			}},
		}},
	}}
	controller := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.Get("/api/achievements/explorer", controller.AchievementsExplorer)

	req := httptest.NewRequest(http.MethodGet, "/api/achievements/explorer", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp AchievementsExplorerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Games) != 1 || len(resp.Games[0].Systems) != 1 {
		t.Fatalf("explorer = %+v, want one game and one cached system", resp)
	}
	if len(resp.Games[0].Systems[0].Achievements) != 2 {
		t.Fatalf("achievements = %+v, want 2 cached achievements", resp.Games[0].Systems[0].Achievements)
	}
	if resp.Games[0].Systems[0].Achievements[0].UnlockedAt == "" {
		t.Fatal("expected cached unlocked achievement timestamp in explorer response")
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

func TestBuildAchievementQueryCandidatesPrefersManualMetadataMatch(t *testing.T) {
	game := &core.CanonicalGame{
		ID: "game-1",
		SourceGames: []*core.SourceGame{
			{
				ID:         "source-1",
				PluginID:   "game-source-steam",
				ExternalID: "220",
				Status:     "found",
				ResolverMatches: []core.ResolverMatch{
					{PluginID: "retroachievements", ExternalID: "ra-active", Outvoted: false},
					{PluginID: "retroachievements", ExternalID: "ra-manual", ManualSelection: true},
				},
			},
		},
	}

	candidates := buildAchievementQueryCandidates(game, []string{"retroachievements"})
	candidate, ok := candidates["retroachievements"]
	if !ok {
		t.Fatal("expected retroachievements candidate")
	}
	if candidate.ExternalGameID != "ra-manual" {
		t.Fatalf("external_game_id = %q, want %q", candidate.ExternalGameID, "ra-manual")
	}
	if candidate.SourceGameID != "source-1" {
		t.Fatalf("source_game_id = %q, want %q", candidate.SourceGameID, "source-1")
	}
}

func TestAchievementControllerGetAchievementsQueriesMetadataProviderFallbackCandidate(t *testing.T) {
	game := &core.CanonicalGame{
		ID: "game-1",
		ExternalIDs: []core.ExternalID{
			{Source: "metadata-igdb", ExternalID: "igdb-1"},
		},
		SourceGames: []*core.SourceGame{
			{
				ID:         "source-1",
				PluginID:   "game-source-smb",
				ExternalID: "share-1",
				Status:     "found",
				ResolverMatches: []core.ResolverMatch{
					{PluginID: "metadata-igdb", ExternalID: "igdb-1", Outvoted: false},
					{PluginID: "retroachievements", ExternalID: "ra-42", Outvoted: true},
				},
			},
		},
	}
	store := &fakeGameStore{game: game}
	host := &fakePluginHost{
		provides: map[string][]string{
			"achievements.game.get": {"retroachievements"},
		},
		results: map[string]rawAchievementPluginResult{
			"retroachievements": {
				Source:         "retroachievements",
				ExternalGameID: "ra-42",
				Achievements: []rawAchievementPluginEntry{
					{
						ExternalID: "ra-ach-1",
						Title:      "Unlocked",
						Points:     5,
						Unlocked:   true,
						UnlockedAt: "2024-03-09T16:00:00Z",
					},
					{
						ExternalID: "ra-ach-2",
						Title:      "Locked",
						Points:     10,
						Unlocked:   false,
						UnlockedAt: "2024-03-10T18:00:00Z",
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

	if len(host.calls) != 1 {
		t.Fatalf("plugin calls = %d, want 1", len(host.calls))
	}
	if host.calls[0].pluginID != "retroachievements" {
		t.Fatalf("plugin_id = %q, want retroachievements", host.calls[0].pluginID)
	}
	if host.calls[0].externalGameID != "ra-42" {
		t.Fatalf("external_game_id = %q, want %q", host.calls[0].externalGameID, "ra-42")
	}

	var sets []AchievementSetDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &sets); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("len(sets) = %d, want 1", len(sets))
	}
	if sets[0].UnlockedCount != 1 || sets[0].TotalCount != 2 {
		t.Fatalf("set counts = %+v, want 1/2", sets[0])
	}
	if sets[0].Achievements[1].UnlockedAt != "" {
		t.Fatalf("locked achievement unlocked_at = %q, want empty", sets[0].Achievements[1].UnlockedAt)
	}
	if len(store.cached) != 1 {
		t.Fatalf("cache writes = %d, want 1", len(store.cached))
	}
	if store.cached[0].sourceGameID != "source-1" {
		t.Fatalf("cached source_game_id = %q, want %q", store.cached[0].sourceGameID, "source-1")
	}
	if store.cached[0].set.Source != "retroachievements" || store.cached[0].set.ExternalGameID != "ra-42" {
		t.Fatalf("cached set = %+v, want retroachievements ra-42", store.cached[0].set)
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

type fakePluginCall struct {
	pluginID       string
	method         string
	externalGameID string
}

type fakeGameStore struct {
	game                       *core.CanonicalGame
	cached                     []cachedAchievementCall
	achievementDashboard       *core.CachedAchievementsDashboard
	achievementExplorer        *core.CachedAchievementsExplorer
	setFavoriteID              string
	clearFavoriteID            string
	setCoverOverrideID         string
	setCoverOverrideAsset      int
	setHoverOverrideID         string
	setHoverOverrideAsset      int
	setBackgroundOverrideID    string
	setBackgroundOverrideAsset int
	clearCoverOverrideID       string
	manualReviewCandidates     []*core.ManualReviewCandidate
	manualReviewByID           map[string]*core.ManualReviewCandidate
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
func (f *fakeGameStore) UpdateMediaAssetMetadata(context.Context, int, int, int, string) error {
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
func (f *fakeGameStore) GetCachedAchievementsDashboard(context.Context) (*core.CachedAchievementsDashboard, error) {
	if f.achievementDashboard != nil {
		return f.achievementDashboard, nil
	}
	return &core.CachedAchievementsDashboard{}, nil
}
func (f *fakeGameStore) GetCachedAchievementsExplorer(context.Context) (*core.CachedAchievementsExplorer, error) {
	if f.achievementExplorer != nil {
		return f.achievementExplorer, nil
	}
	return &core.CachedAchievementsExplorer{}, nil
}
func (f *fakeGameStore) GetGamesByIntegrationID(context.Context, string, int) ([]core.GameListItem, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetEnrichedGamesByPluginID(context.Context, string, int) ([]core.GameListItem, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) ListManualReviewCandidates(_ context.Context, scope core.ManualReviewScope, _ int) ([]*core.ManualReviewCandidate, error) {
	if scope == core.ManualReviewScopeArchive {
		var archived []*core.ManualReviewCandidate
		for _, candidate := range f.manualReviewCandidates {
			if candidate != nil && candidate.ReviewState == core.ManualReviewStateNotAGame {
				archived = append(archived, candidate)
			}
		}
		return archived, nil
	}
	return f.manualReviewCandidates, nil
}
func (f *fakeGameStore) GetManualReviewCandidate(_ context.Context, id string) (*core.ManualReviewCandidate, error) {
	if f.manualReviewByID == nil {
		return nil, nil
	}
	return f.manualReviewByID[id], nil
}
func (f *fakeGameStore) SaveManualReviewResult(_ context.Context, sourceGame *core.SourceGame, resolverMatches []core.ResolverMatch, _ []core.MediaRef) error {
	if sourceGame == nil {
		return nil
	}
	candidate := &core.ManualReviewCandidate{
		ID:                 sourceGame.ID,
		CurrentTitle:       sourceGame.RawTitle,
		RawTitle:           sourceGame.RawTitle,
		Platform:           sourceGame.Platform,
		Kind:               sourceGame.Kind,
		GroupKind:          sourceGame.GroupKind,
		IntegrationID:      sourceGame.IntegrationID,
		PluginID:           sourceGame.PluginID,
		ExternalID:         sourceGame.ExternalID,
		RootPath:           sourceGame.RootPath,
		URL:                sourceGame.URL,
		Status:             sourceGame.Status,
		ReviewState:        sourceGame.ReviewState,
		FileCount:          len(sourceGame.Files),
		ResolverMatchCount: len(resolverMatches),
		Files:              append([]core.GameFile(nil), sourceGame.Files...),
		ResolverMatches:    append([]core.ResolverMatch(nil), resolverMatches...),
		CreatedAt:          time.Now().UTC(),
		LastSeenAt:         sourceGame.LastSeenAt,
	}
	if f.manualReviewByID == nil {
		f.manualReviewByID = map[string]*core.ManualReviewCandidate{}
	}
	f.manualReviewByID[sourceGame.ID] = candidate
	return nil
}
func (f *fakeGameStore) SetManualReviewState(_ context.Context, candidateID string, state core.ManualReviewState) error {
	if f.manualReviewByID != nil {
		if candidate := f.manualReviewByID[candidateID]; candidate != nil {
			candidate.ReviewState = state
		}
	}
	for _, candidate := range f.manualReviewCandidates {
		if candidate != nil && candidate.ID == candidateID {
			candidate.ReviewState = state
		}
	}
	return nil
}
func (f *fakeGameStore) GetFoundSourceGames(context.Context, []string) ([]*core.FoundSourceGame, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) GetFoundSourceGameRecords(context.Context, []string) ([]*core.SourceGame, error) {
	panic("unexpected call")
}
func (f *fakeGameStore) DeleteGamesByIntegrationID(context.Context, string) error {
	panic("unexpected call")
}
func (f *fakeGameStore) DeleteSourceGameByID(context.Context, string) error {
	panic("unexpected call")
}
func (f *fakeGameStore) SaveScanReport(context.Context, *core.ScanReport) error {
	panic("unexpected call")
}
func (f *fakeGameStore) SaveRefreshedMetadataProviderResults(context.Context, []*core.SourceGame) error {
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
func (f *fakeGameStore) SetCanonicalCoverOverride(_ context.Context, canonicalID string, mediaAssetID int) error {
	f.setCoverOverrideID = canonicalID
	f.setCoverOverrideAsset = mediaAssetID
	return nil
}
func (f *fakeGameStore) SetCanonicalHoverOverride(_ context.Context, canonicalID string, mediaAssetID int) error {
	f.setHoverOverrideID = canonicalID
	f.setHoverOverrideAsset = mediaAssetID
	return nil
}
func (f *fakeGameStore) SetCanonicalBackgroundOverride(_ context.Context, canonicalID string, mediaAssetID int) error {
	f.setBackgroundOverrideID = canonicalID
	f.setBackgroundOverrideAsset = mediaAssetID
	return nil
}
func (f *fakeGameStore) SetCanonicalFavorite(_ context.Context, canonicalID string) error {
	f.setFavoriteID = canonicalID
	if f.game == nil {
		return core.ErrCanonicalGameNotFound
	}
	if f.game != nil {
		f.game.Favorite = true
	}
	return nil
}
func (f *fakeGameStore) ClearCanonicalFavorite(_ context.Context, canonicalID string) error {
	f.clearFavoriteID = canonicalID
	if f.game == nil {
		return core.ErrCanonicalGameNotFound
	}
	if f.game != nil {
		f.game.Favorite = false
	}
	return nil
}
func (f *fakeGameStore) ClearCanonicalCoverOverride(_ context.Context, canonicalID string) error {
	f.clearCoverOverrideID = canonicalID
	return nil
}

type fakePluginHost struct {
	provides          map[string][]string
	results           map[string]rawAchievementPluginResult
	metadataResults   map[string]reviewMetadataLookupResponse
	metadataCallError map[string]error
	calls             []fakePluginCall
}

func (f *fakePluginHost) Discover(context.Context) error { panic("unexpected call") }
func (f *fakePluginHost) Call(_ context.Context, pluginID, method string, params any, result any) error {
	switch payload := params.(type) {
	case map[string]any:
		if externalGameID, ok := payload["external_game_id"].(string); ok {
			f.calls = append(f.calls, fakePluginCall{
				pluginID:       pluginID,
				method:         method,
				externalGameID: externalGameID,
			})
		}
	}
	switch method {
	case "achievements.game.get":
		payload, ok := f.results[pluginID]
		if !ok {
			return nil
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, result)
	case reviewMetadataLookupMethod:
		if err, ok := f.metadataCallError[pluginID]; ok {
			return err
		}
		payload, ok := f.metadataResults[pluginID]
		if !ok {
			return nil
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, result)
	default:
		panic("unexpected call")
	}
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

func TestPluginControllerStartIntegrationAuthReturnsOAuthRequired(t *testing.T) {
	repo := &fakeControllerIntegrationRepo{
		byID: map[string]*core.Integration{
			"int-1": {
				ID:         "int-1",
				PluginID:   "plugin.oauth",
				Label:      "OAuth Integration",
				ConfigJSON: `{"root_path":"games"}`,
			},
		},
	}
	host := &fakeControllerIntegrationPluginHost{
		plugins: map[string]*core.Plugin{
			"plugin.oauth": {
				Manifest: core.PluginManifest{
					ID:           "plugin.oauth",
					ConfigSchema: map[string]any{},
				},
			},
		},
		checkResults: map[string]integrationCheckResult{
			"plugin.oauth": {
				Status:       "oauth_required",
				Message:      "Sign in required",
				AuthorizeURL: "https://example.com/auth",
				State:        "state-123",
			},
		},
	}
	controller := NewPluginController(repo, host, &fakeGameStore{}, staticConfig{values: map[string]string{"PORT": "8900"}}, noopLogger{}, nil)

	router := chi.NewRouter()
	router.Post("/api/integrations/{id}/authorize", controller.StartIntegrationAuth)

	req := httptest.NewRequest(http.MethodPost, "/api/integrations/int-1/authorize", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := body["status"]; got != "oauth_required" {
		t.Fatalf("status payload = %v, want oauth_required", got)
	}
	if got := body["plugin_id"]; got != "plugin.oauth" {
		t.Fatalf("plugin_id = %v, want plugin.oauth", got)
	}
	if got := body["authorize_url"]; got != "https://example.com/auth" {
		t.Fatalf("authorize_url = %v, want https://example.com/auth", got)
	}
}

func TestPluginControllerUpdateIntegrationReturnsOAuthRequired(t *testing.T) {
	repo := &fakeControllerIntegrationRepo{
		byID: map[string]*core.Integration{
			"int-1": {
				ID:         "int-1",
				PluginID:   "plugin.oauth",
				Label:      "OAuth Integration",
				ConfigJSON: `{}`,
			},
		},
	}
	host := &fakeControllerIntegrationPluginHost{
		plugins: map[string]*core.Plugin{
			"plugin.oauth": {
				Manifest: core.PluginManifest{
					ID:           "plugin.oauth",
					ConfigSchema: map[string]any{},
				},
			},
		},
		checkResults: map[string]integrationCheckResult{
			"plugin.oauth": {
				Status:       "oauth_required",
				Message:      "Need browser auth",
				AuthorizeURL: "https://example.com/auth",
				State:        "state-456",
			},
		},
	}
	controller := NewPluginController(repo, host, &fakeGameStore{}, staticConfig{values: map[string]string{"PORT": "8900"}}, noopLogger{}, nil)

	router := chi.NewRouter()
	router.Put("/api/integrations/{id}", controller.UpdateIntegration)

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/integrations/int-1",
		bytes.NewBufferString(`{"config":{"root_path":"updated"}}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := body["status"]; got != "oauth_required" {
		t.Fatalf("status payload = %v, want oauth_required", got)
	}
	if got := body["state"]; got != "state-456" {
		t.Fatalf("state = %v, want state-456", got)
	}
	if repo.updated != nil {
		t.Fatal("update should not persist while oauth is still required")
	}
}

func TestPluginControllerCreateRejectsDuplicateFilesystemSourceIdentity(t *testing.T) {
	repo := &fakeControllerIntegrationRepo{
		byID: map[string]*core.Integration{
			"existing-drive": {
				ID:         "existing-drive",
				PluginID:   "game-source-google-drive",
				Label:      "Primary Drive",
				ConfigJSON: `{"include_paths":[{"path":"Games","recursive":true}]}`,
			},
		},
	}
	host := &fakeControllerIntegrationPluginHost{
		plugins: map[string]*core.Plugin{
			"game-source-google-drive": {
				Manifest: core.PluginManifest{
					ID:           "game-source-google-drive",
					ConfigSchema: map[string]any{},
				},
			},
		},
		checkResults: map[string]integrationCheckResult{
			"game-source-google-drive": {
				Status:         "ok",
				SourceIdentity: "gdrive:acct-123",
			},
		},
	}
	controller := NewPluginController(repo, host, &fakeGameStore{}, staticConfig{values: map[string]string{"PORT": "8900"}}, noopLogger{}, nil)

	router := chi.NewRouter()
	router.Post("/api/integrations", controller.Create)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/integrations",
		bytes.NewBufferString(`{"plugin_id":"game-source-google-drive","label":"Second Drive","integration_type":"source","config":{"include_paths":[{"path":"Retro","recursive":false}]}}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if !strings.Contains(rec.Body.String(), "Edit the existing integration and add include paths there.") {
		t.Fatalf("body = %q, want duplicate guidance", rec.Body.String())
	}
}

func TestPluginControllerUpdateRejectsDuplicateFilesystemSourceIdentity(t *testing.T) {
	repo := &fakeControllerIntegrationRepo{
		byID: map[string]*core.Integration{
			"existing-drive": {
				ID:         "existing-drive",
				PluginID:   "game-source-google-drive",
				Label:      "Primary Drive",
				ConfigJSON: `{"include_paths":[{"path":"Games","recursive":true}]}`,
			},
			"editing-drive": {
				ID:         "editing-drive",
				PluginID:   "game-source-google-drive",
				Label:      "Other Drive",
				ConfigJSON: `{"include_paths":[{"path":"Arcade","recursive":true}]}`,
			},
		},
	}
	host := &fakeControllerIntegrationPluginHost{
		plugins: map[string]*core.Plugin{
			"game-source-google-drive": {
				Manifest: core.PluginManifest{
					ID:           "game-source-google-drive",
					ConfigSchema: map[string]any{},
				},
			},
		},
		checkResults: map[string]integrationCheckResult{
			"game-source-google-drive": {
				Status:         "ok",
				SourceIdentity: "gdrive:acct-123",
			},
		},
	}
	controller := NewPluginController(repo, host, &fakeGameStore{}, staticConfig{values: map[string]string{"PORT": "8900"}}, noopLogger{}, nil)

	router := chi.NewRouter()
	router.Put("/api/integrations/{id}", controller.UpdateIntegration)

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/integrations/editing-drive",
		bytes.NewBufferString(`{"config":{"include_paths":[{"path":"Retro","recursive":false}]}}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if repo.updated != nil {
		t.Fatal("update should not persist duplicate source identities")
	}
}

type staticConfig struct {
	values map[string]string
}

func (c staticConfig) Get(key string) string { return c.values[key] }
func (c staticConfig) GetInt(string) int     { return 0 }
func (c staticConfig) GetBool(string) bool   { return false }
func (c staticConfig) Validate() error       { return nil }

type fakeControllerIntegrationRepo struct {
	byID    map[string]*core.Integration
	updated *core.Integration
}

func (r *fakeControllerIntegrationRepo) Create(context.Context, *core.Integration) error {
	panic("unexpected call")
}
func (r *fakeControllerIntegrationRepo) Update(_ context.Context, integration *core.Integration) error {
	copy := *integration
	r.updated = &copy
	if r.byID == nil {
		r.byID = map[string]*core.Integration{}
	}
	r.byID[integration.ID] = &copy
	return nil
}
func (r *fakeControllerIntegrationRepo) Delete(context.Context, string) error {
	panic("unexpected call")
}
func (r *fakeControllerIntegrationRepo) List(context.Context) ([]*core.Integration, error) {
	var integrations []*core.Integration
	for _, integration := range r.byID {
		copy := *integration
		integrations = append(integrations, &copy)
	}
	return integrations, nil
}
func (r *fakeControllerIntegrationRepo) GetByID(_ context.Context, id string) (*core.Integration, error) {
	if integration, ok := r.byID[id]; ok {
		copy := *integration
		return &copy, nil
	}
	return nil, nil
}
func (r *fakeControllerIntegrationRepo) ListByPluginID(_ context.Context, pluginID string) ([]*core.Integration, error) {
	var integrations []*core.Integration
	for _, integration := range r.byID {
		if integration.PluginID != pluginID {
			continue
		}
		copy := *integration
		integrations = append(integrations, &copy)
	}
	return integrations, nil
}

type fakeControllerIntegrationPluginHost struct {
	plugins      map[string]*core.Plugin
	checkResults map[string]integrationCheckResult
}

func (f *fakeControllerIntegrationPluginHost) Discover(context.Context) error {
	panic("unexpected call")
}
func (f *fakeControllerIntegrationPluginHost) Call(_ context.Context, pluginID, method string, _ any, result any) error {
	if method != "plugin.check_config" {
		panic("unexpected call")
	}
	payload := f.checkResults[pluginID]
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}
func (f *fakeControllerIntegrationPluginHost) Close() error { return nil }
func (f *fakeControllerIntegrationPluginHost) GetPluginIDs() []string {
	return nil
}
func (f *fakeControllerIntegrationPluginHost) GetPlugin(pluginID string) (*core.Plugin, bool) {
	plugin, ok := f.plugins[pluginID]
	return plugin, ok
}
func (f *fakeControllerIntegrationPluginHost) ListPlugins() []plugins.PluginInfo { return nil }
func (f *fakeControllerIntegrationPluginHost) GetPluginIDsProviding(string) []string {
	return nil
}
