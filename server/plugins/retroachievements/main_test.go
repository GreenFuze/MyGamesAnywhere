package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRAGetUsesBrowserLikeHeaders(t *testing.T) {
	origBase := raAPIBase
	origClient := raHTTPClient
	origTicker := rateLimiter
	defer func() {
		raAPIBase = origBase
		raHTTPClient = origClient
		rateLimiter = origTicker
	}()

	cfg = raConfig{Username: "retro-user", APIKey: "retro-key"}
	rateLimiter = time.NewTicker(time.Microsecond)
	t.Cleanup(rateLimiter.Stop)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != raUserAgent {
			t.Fatalf("user-agent = %q, want %q", got, raUserAgent)
		}
		if got := r.Header.Get("Accept"); !strings.Contains(got, "application/json") {
			t.Fatalf("accept = %q, want JSON-capable header", got)
		}
		if got := r.Header.Get("Accept-Language"); got == "" {
			t.Fatal("accept-language should be set")
		}
		if got := r.Header.Get("Referer"); got != "https://retroachievements.org/" {
			t.Fatalf("referer = %q, want RetroAchievements origin", got)
		}
		if got := r.Header.Get("Origin"); got != "https://retroachievements.org" {
			t.Fatalf("origin = %q, want RetroAchievements origin", got)
		}
		values := r.URL.Query()
		if values.Get("z") != cfg.Username || values.Get("y") != cfg.APIKey {
			t.Fatalf("query = %v, want credentials included", values)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `[{"ID":1,"Name":"NES"}]`)
	}))
	defer server.Close()

	raAPIBase = server.URL
	raHTTPClient = server.Client()

	body, err := raGet("API_GetConsoleIDs.php", url.Values{"i": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `[{"ID":1,"Name":"NES"}]` {
		t.Fatalf("body = %q", body)
	}
}

func TestRAGetIncludesResponseBodyOnFailure(t *testing.T) {
	origBase := raAPIBase
	origClient := raHTTPClient
	origTicker := rateLimiter
	defer func() {
		raAPIBase = origBase
		raHTTPClient = origClient
		rateLimiter = origTicker
	}()

	cfg = raConfig{Username: "retro-user", APIKey: "retro-key"}
	rateLimiter = time.NewTicker(time.Microsecond)
	t.Cleanup(rateLimiter.Stop)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "cloudflare block", http.StatusForbidden)
	}))
	defer server.Close()

	raAPIBase = server.URL
	raHTTPClient = server.Client()

	_, err := raGet("API_GetConsoleIDs.php", url.Values{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("error = %v, want status code", err)
	}
	if !strings.Contains(err.Error(), "cloudflare block") {
		t.Fatalf("error = %v, want response body", err)
	}
}

func TestClassifyCheckConfigErrorCloudflareBlockIsUnavailable(t *testing.T) {
	status, message := classifyCheckConfigError(
		errors.New("RA API API_GetConsoleIDs.php: status 403: <title>Attention Required! | Cloudflare</title>"),
	)
	if status != "unavailable" {
		t.Fatalf("status = %q, want %q", status, "unavailable")
	}
	if !strings.Contains(strings.ToLower(message), "blocked or unavailable") {
		t.Fatalf("message = %q, want upstream unavailable wording", message)
	}
}

func TestValidateCheckConfigMissingCredentialsIsError(t *testing.T) {
	result := validateCheckConfig(map[string]any{})
	if got, _ := result["status"].(string); got != "error" {
		t.Fatalf("status = %q, want %q", got, "error")
	}
}

func TestHandleLookupUsesRequestConfigAndMatchesAlteredBeast(t *testing.T) {
	origBase := raAPIBase
	origClient := raHTTPClient
	origTicker := rateLimiter
	origCfg := cfg
	origCache := gameListCache
	origFailureCache := gameListFailureCache
	origCacheRoot := raGameListCacheRoot
	defer func() {
		raAPIBase = origBase
		raHTTPClient = origClient
		rateLimiter = origTicker
		cfg = origCfg
		gameListCache = origCache
		gameListFailureCache = origFailureCache
		raGameListCacheRoot = origCacheRoot
	}()

	cfg = raConfig{}
	gameListCache = make(map[int][]raGameListEntry)
	gameListFailureCache = make(map[int]cachedFailure)
	raGameListCacheRoot = t.TempDir()
	rateLimiter = time.NewTicker(time.Microsecond)
	t.Cleanup(rateLimiter.Stop)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.URL.Query()
		if values.Get("z") != "retro-user" || values.Get("y") != "retro-key" {
			t.Fatalf("query credentials = %v, want request config credentials", values)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/API_GetGameList.php":
			if values.Get("i") != "1" {
				t.Fatalf("console id = %q, want genesis console 1", values.Get("i"))
			}
			if values.Get("f") != "1" {
				t.Fatalf("filter f = %q, want 1", values.Get("f"))
			}
			if values.Get("h") != "" {
				t.Fatalf("hash flag h = %q, want empty for title lookup", values.Get("h"))
			}
			_ = json.NewEncoder(w).Encode([]raGameListEntry{{
				ID:        1,
				Title:     "Altered Beast",
				ConsoleID: 1,
				ImageIcon: "/Images/000001.png",
			}})
		case "/API_GetGameExtended.php":
			if values.Get("i") != "1" {
				t.Fatalf("game id = %q, want 1", values.Get("i"))
			}
			_ = json.NewEncoder(w).Encode(raGameExtended{
				ID:          1,
				Title:       "Altered Beast",
				ConsoleID:   1,
				ConsoleName: "Genesis/Mega Drive",
				Genre:       "Action",
				Developer:   "Sega",
				Publisher:   "Sega",
				Released:    "1988",
				ImageBoxArt: "/Images/BoxArt/000001.png",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	raAPIBase = server.URL
	raHTTPClient = server.Client()

	result, errObj := handleLookup(lookupParams{
		Config: map[string]any{
			"api_key":  "retro-key",
			"username": "retro-user",
		},
		Games: []gameQuery{{
			Index:    0,
			Title:    "Altered Beast (USA)",
			Platform: "genesis",
		}},
	})
	if errObj != nil {
		t.Fatalf("handleLookup error = %+v", errObj)
	}

	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Results []lookupResult `json:"results"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Results) != 1 {
		t.Fatalf("result count = %d, want 1: %s", len(decoded.Results), payload)
	}
	match := decoded.Results[0]
	if match.Title != "Altered Beast" || match.ExternalID != "1" || match.Platform != "genesis" {
		t.Fatalf("match = %+v, want Altered Beast genesis result", match)
	}
	if len(match.Media) != 1 || match.Media[0].URL != "https://retroachievements.org/Images/BoxArt/000001.png" {
		t.Fatalf("media = %+v, want box art URL", match.Media)
	}
	if cfg.APIKey != "" || cfg.Username != "" {
		t.Fatalf("global config leaked after lookup: %+v", cfg)
	}
}

func TestFetchGameListRequestsFilteredGamesWithoutHashes(t *testing.T) {
	origBase := raAPIBase
	origClient := raHTTPClient
	origTicker := rateLimiter
	origCfg := cfg
	defer func() {
		raAPIBase = origBase
		raHTTPClient = origClient
		rateLimiter = origTicker
		cfg = origCfg
	}()

	cfg = raConfig{Username: "retro-user", APIKey: "retro-key"}
	rateLimiter = time.NewTicker(time.Microsecond)
	t.Cleanup(rateLimiter.Stop)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.URL.Query()
		if values.Get("i") != "27" {
			t.Fatalf("console id = %q, want 27", values.Get("i"))
		}
		if values.Get("f") != "1" {
			t.Fatalf("filter f = %q, want 1", values.Get("f"))
		}
		if values.Get("h") != "" {
			t.Fatalf("hash flag h = %q, want empty", values.Get("h"))
		}
		_, _ = io.WriteString(w, `[{"ID":1,"Title":"Altered Beast"}]`)
	}))
	defer server.Close()

	raAPIBase = server.URL
	raHTTPClient = server.Client()

	games, err := fetchGameList(27)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 || games[0].Title != "Altered Beast" {
		t.Fatalf("games = %+v, want Altered Beast", games)
	}
}

func TestGetGameListUsesPersistentCache(t *testing.T) {
	origBase := raAPIBase
	origClient := raHTTPClient
	origTicker := rateLimiter
	origCfg := cfg
	origCache := gameListCache
	origFailureCache := gameListFailureCache
	origCacheRoot := raGameListCacheRoot
	defer func() {
		raAPIBase = origBase
		raHTTPClient = origClient
		rateLimiter = origTicker
		cfg = origCfg
		gameListCache = origCache
		gameListFailureCache = origFailureCache
		raGameListCacheRoot = origCacheRoot
	}()

	cfg = raConfig{Username: "retro-user", APIKey: "retro-key"}
	gameListCache = make(map[int][]raGameListEntry)
	gameListFailureCache = make(map[int]cachedFailure)
	raGameListCacheRoot = t.TempDir()
	rateLimiter = time.NewTicker(time.Microsecond)
	t.Cleanup(rateLimiter.Stop)

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = io.WriteString(w, `[{"ID":1,"Title":"Altered Beast"}]`)
	}))
	defer server.Close()

	raAPIBase = server.URL
	raHTTPClient = server.Client()

	if _, err := getGameList(1); err != nil {
		t.Fatal(err)
	}
	gameListCache = make(map[int][]raGameListEntry)
	games, err := getGameList(1)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1 due persistent cache", requests)
	}
	if len(games) != 1 || games[0].Title != "Altered Beast" {
		t.Fatalf("cached games = %+v, want Altered Beast", games)
	}
}

func TestGetGameListBacksOffAfterFailure(t *testing.T) {
	origBase := raAPIBase
	origClient := raHTTPClient
	origTicker := rateLimiter
	origCfg := cfg
	origCache := gameListCache
	origFailureCache := gameListFailureCache
	origCacheRoot := raGameListCacheRoot
	defer func() {
		raAPIBase = origBase
		raHTTPClient = origClient
		rateLimiter = origTicker
		cfg = origCfg
		gameListCache = origCache
		gameListFailureCache = origFailureCache
		raGameListCacheRoot = origCacheRoot
	}()

	cfg = raConfig{Username: "retro-user", APIKey: "retro-key"}
	gameListCache = make(map[int][]raGameListEntry)
	gameListFailureCache = make(map[int]cachedFailure)
	raGameListCacheRoot = t.TempDir()
	rateLimiter = time.NewTicker(time.Microsecond)
	t.Cleanup(rateLimiter.Stop)

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "cloudflare block", http.StatusForbidden)
	}))
	defer server.Close()

	raAPIBase = server.URL
	raHTTPClient = server.Client()

	if _, err := getGameList(1); err == nil {
		t.Fatal("expected first request to fail")
	}
	if _, err := getGameList(1); err == nil {
		t.Fatal("expected cached failure to fail")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1 due failure backoff", requests)
	}
}

func TestHandleAchievementsUsesRequestConfig(t *testing.T) {
	origBase := raAPIBase
	origClient := raHTTPClient
	origTicker := rateLimiter
	origCfg := cfg
	defer func() {
		raAPIBase = origBase
		raHTTPClient = origClient
		rateLimiter = origTicker
		cfg = origCfg
	}()

	cfg = raConfig{}
	rateLimiter = time.NewTicker(time.Microsecond)
	t.Cleanup(rateLimiter.Stop)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.URL.Query()
		if values.Get("z") != "retro-user" || values.Get("y") != "retro-key" {
			t.Fatalf("query credentials = %v, want request config credentials", values)
		}
		if values.Get("g") != "8751" || values.Get("u") != "retro-user" {
			t.Fatalf("query = %v, want game and username", values)
		}
		_ = json.NewEncoder(w).Encode(raGameExtended{
			ID:              8751,
			Title:           "Altered Beast",
			NumAchievements: 1,
			Achievements: map[string]raAchievement{
				"1": {
					ID:         1,
					Title:      "Rise from Your Grave",
					Points:     5,
					BadgeName:  "00001",
					DateEarned: "2024-03-09T16:00:00Z",
				},
			},
		})
	}))
	defer server.Close()

	raAPIBase = server.URL
	raHTTPClient = server.Client()

	params, err := json.Marshal(map[string]any{
		"external_game_id": "8751",
		"config": map[string]any{
			"api_key":  "retro-key",
			"username": "retro-user",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, errObj := handleAchievementsGet(params)
	if errObj != nil {
		t.Fatalf("handleAchievementsGet error = %+v", errObj)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Source         string             `json:"source"`
		ExternalGameID string             `json:"external_game_id"`
		UnlockedCount  int                `json:"unlocked_count"`
		Achievements   []achievementEntry `json:"achievements"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Source != "retroachievements" || decoded.ExternalGameID != "8751" {
		t.Fatalf("result = %+v, want RetroAchievements 8751", decoded)
	}
	if decoded.UnlockedCount != 1 || len(decoded.Achievements) != 1 {
		t.Fatalf("achievement result = %+v, want one unlocked achievement", decoded)
	}
	if cfg.APIKey != "" || cfg.Username != "" {
		t.Fatalf("global config leaked after achievements: %+v", cfg)
	}
}

func TestBuildRetroAchievementEntriesPreservesMixedUnlockStateAndPoints(t *testing.T) {
	game := &raGameExtended{
		NumDistinctPlayersCasual: 10,
		Achievements: map[string]raAchievement{
			"1": {
				ID:          1,
				Title:       "Unlocked One",
				Description: "First achievement",
				Points:      10,
				BadgeName:   "badge1",
				NumAwarded:  5,
				DateEarned:  "2024-03-09T16:00:00Z",
			},
			"2": {
				ID:          2,
				Title:       "Still Locked",
				Description: "Second achievement",
				Points:      25,
				BadgeName:   "badge2",
				NumAwarded:  2,
			},
		},
	}

	achievements, unlocked, totalPoints, earnedPoints := buildRetroAchievementEntries(game)

	if len(achievements) != 2 {
		t.Fatalf("len(achievements) = %d, want 2", len(achievements))
	}
	if unlocked != 1 {
		t.Fatalf("unlocked = %d, want 1", unlocked)
	}
	if totalPoints != 35 {
		t.Fatalf("total_points = %d, want 35", totalPoints)
	}
	if earnedPoints != 10 {
		t.Fatalf("earned_points = %d, want 10", earnedPoints)
	}
	if !achievements[0].Unlocked || achievements[0].UnlockedAt == "" {
		t.Fatalf("first achievement = %+v, want unlocked with timestamp", achievements[0])
	}
	if achievements[1].Unlocked || achievements[1].UnlockedAt != "" {
		t.Fatalf("second achievement = %+v, want locked without timestamp", achievements[1])
	}
}
