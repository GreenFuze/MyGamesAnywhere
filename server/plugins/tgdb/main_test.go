package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Duke Nukem 3D", "duke nukem 3d"},
		{"Castlevania: Symphony of the Night", "castlevania symphony of the night"},
		{"Sonic the Hedgehog 2", "sonic the hedgehog 2"},
		{"game 1.0 cs", "game"},
		{"BeamNG.Drive.v0.29.0", "beamng drive"},
	}
	for _, tc := range tests {
		got := normalizeTitle(tc.in)
		if got != tc.want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNewTGDBClientRequiresConnectionAPIKey(t *testing.T) {
	if _, err := newTGDBClient("  "); err == nil {
		t.Fatal("expected empty profile connection API key to fail")
	}
	client, err := newTGDBClient("profile-key")
	if err != nil {
		t.Fatalf("newTGDBClient: %v", err)
	}
	if client.apiKey != "profile-key" {
		t.Fatalf("api key = %q, want profile-key", client.apiKey)
	}
}

func TestHandleLookupFailsFastWithoutConnectionAPIKey(t *testing.T) {
	result, pluginErr := handleLookup(lookupParams{Games: []gameQuery{{Index: 0, Title: "Doom"}}})
	if result != nil || pluginErr == nil || pluginErr.Code != "CONFIG_REQUIRED" {
		t.Fatalf("result=%v error=%+v, want CONFIG_REQUIRED", result, pluginErr)
	}
}

func TestTGDBClientKeepsConnectionCredentialsIsolated(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.URL.Query().Get("apikey"))
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"code":200,"status":"Success","data":{"count":0,"games":[]}}`))
	}))
	defer server.Close()

	ready := make(chan time.Time, 2)
	ready <- time.Now()
	ready <- time.Now()
	for _, key := range []string{"profile-a-key", "profile-b-key"} {
		client, err := newTGDBClient(key)
		if err != nil {
			t.Fatalf("newTGDBClient(%q): %v", key, err)
		}
		client.baseURL = server.URL
		client.rateLimit = ready
		if _, err := client.search("Doom", nil); err != nil {
			t.Fatalf("search with %q: %v", key, err)
		}
	}
	if len(seen) != 2 || seen[0] != "profile-a-key" || seen[1] != "profile-b-key" {
		t.Fatalf("seen keys = %v, want isolated per-connection keys", seen)
	}
}

// --- Integration tests (require real TGDB API key) ---

func loadTestClient(t *testing.T) *tgdbClient {
	t.Helper()
	if os.Getenv("TGDB_INTEGRATION") == "" {
		t.Skip("set TGDB_INTEGRATION=1 to run integration tests")
	}
	client, err := newTGDBClient(os.Getenv("TGDB_API_KEY"))
	if err != nil {
		t.Skip("set TGDB_API_KEY to run integration tests")
	}
	return client
}

func TestMatchGame(t *testing.T) {
	client := loadTestClient(t)

	tests := []struct {
		title    string
		platform string
		wantAny  []string
	}{
		{"Donkey Kong", "arcade", []string{"Donkey Kong"}},
		{"Duke Nukem 3D", "ms_dos", []string{"Duke Nukem 3D"}},
		{"Half-Life 2", "windows_pc", []string{"Half-Life 2"}},
		{"God of War", "ps2", []string{"God of War"}},
	}

	for _, tc := range tests {
		q := gameQuery{Index: 0, Title: tc.title, Platform: tc.platform}
		r, err := client.matchGame(q)
		if err != nil {
			t.Errorf("%q: error: %v", tc.title, err)
			continue
		}
		if r == nil {
			t.Errorf("%q: no match", tc.title)
			continue
		}
		t.Logf("%q → %q (ID: %s, URL: %s)", tc.title, r.Title, r.ExternalID, r.URL)
		found := false
		for _, want := range tc.wantAny {
			if normalizeTitle(r.Title) == normalizeTitle(want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: got %q, want one of %v", tc.title, r.Title, tc.wantAny)
		}
	}
}

// --- TV2 Games coverage test ---

type tv2Entry struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

var regionSuffixRE = regexp.MustCompile(`\s*\([^)]*\)\s*`)

func cleanTitle(raw string) string {
	s := strings.TrimSuffix(raw, filepath.Ext(raw))
	s = regionSuffixRE.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "_", " ")
	return s
}

func TestTV2GamesCoverage(t *testing.T) {
	if os.Getenv("TGDB_INTEGRATION") == "" {
		t.Skip("set TGDB_INTEGRATION=1 to run")
	}
	client := loadTestClient(t)

	tv2Path := filepath.Join("..", "..", "internal", "scan", "scanner", "testdata", "tv2_games.json")
	raw, err := os.ReadFile(tv2Path)
	if err != nil {
		t.Fatalf("read tv2_games.json: %v", err)
	}
	var entries []tv2Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parse tv2_games.json: %v", err)
	}

	type gameCandidate struct {
		title    string
		platform string
		rootPath string
	}
	var candidates []gameCandidate

	platformDirMap := map[string]string{
		"MS DOS":                     "ms_dos",
		"Nintendo Game Boy Advanced": "gba",
		"Playstation":                "ps1",
		"Playstation 2":              "ps2",
		"Playstation 3":              "ps3",
		"Playstation Portable":       "psp",
		"XBox 360":                   "xbox_360",
	}

	seenDirs := map[string]bool{}
	for _, e := range entries {
		parts := strings.Split(e.Path, "\\")

		switch {
		case len(parts) == 2 && parts[0] == "Mame" && !e.IsDir:
			stem := strings.TrimSuffix(parts[1], filepath.Ext(parts[1]))
			candidates = append(candidates, gameCandidate{
				title: stem, platform: "arcade", rootPath: e.Path,
			})

		case len(parts) >= 3 && parts[0] == "Roms":
			plat, ok := platformDirMap[parts[1]]
			if !ok {
				continue
			}
			gameName := parts[2]
			key := parts[0] + "\\" + parts[1] + "\\" + gameName
			if seenDirs[key] {
				continue
			}
			seenDirs[key] = true
			title := cleanTitle(gameName)
			candidates = append(candidates, gameCandidate{
				title: title, platform: plat, rootPath: key,
			})

		case len(parts) == 2 && parts[0] == "ScummVM" && e.IsDir && parts[1] != "Manuals":
			title := cleanTitle(parts[1])
			candidates = append(candidates, gameCandidate{
				title: title, platform: "scummvm",
			})

		case len(parts) == 2 && parts[0] == "Installers" && !e.IsDir:
			name := parts[1]
			if !strings.HasSuffix(strings.ToLower(name), ".exe") &&
				!strings.HasSuffix(strings.ToLower(name), ".zip") {
				continue
			}
			if strings.Contains(name, ".bin") {
				continue
			}
			title := cleanTitle(name)
			title = strings.TrimPrefix(title, "setup ")
			candidates = append(candidates, gameCandidate{
				title: title, platform: "windows_pc", rootPath: e.Path,
			})
		}
	}

	t.Logf("extracted %d game candidates from tv2_games.json", len(candidates))

	type result struct {
		candidate gameCandidate
		matched   *lookupResult
	}
	var matched, unmatched []result
	errors := 0

	for i, c := range candidates {
		if i > 0 && i%50 == 0 {
			pct := float64(len(matched)) / float64(i) * 100
			t.Logf("progress: %d/%d (%.0f%% matched so far, %d errors)", i, len(candidates), pct, errors)
		}

		q := gameQuery{
			Index:    0,
			Title:    c.title,
			Platform: c.platform,
			RootPath: c.rootPath,
		}
		r, err := client.matchGame(q)
		if err != nil {
			errors++
			if errors <= 5 {
				t.Logf("ERROR [%s] %q: %v", c.platform, c.title, err)
			}
			continue
		}
		if r != nil {
			matched = append(matched, result{c, r})
		} else {
			unmatched = append(unmatched, result{c, nil})
		}
	}

	total := len(candidates)
	matchCount := len(matched)
	missCount := len(unmatched)
	pct := float64(matchCount) / float64(total) * 100

	t.Logf("\n=== TGDB TV2 Games Coverage Report ===")
	t.Logf("Total candidates: %d", total)
	t.Logf("Matched:          %d (%.1f%%)", matchCount, pct)
	t.Logf("Unmatched:        %d (%.1f%%)", missCount, 100-pct)
	t.Logf("Errors:           %d", errors)

	sort.Slice(unmatched, func(i, j int) bool {
		if unmatched[i].candidate.platform != unmatched[j].candidate.platform {
			return unmatched[i].candidate.platform < unmatched[j].candidate.platform
		}
		return unmatched[i].candidate.title < unmatched[j].candidate.title
	})

	t.Logf("\n--- Unmatched Games ---")
	byPlatform := map[string][]string{}
	for _, r := range unmatched {
		byPlatform[r.candidate.platform] = append(byPlatform[r.candidate.platform], r.candidate.title)
	}
	platforms := make([]string, 0, len(byPlatform))
	for p := range byPlatform {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)
	for _, p := range platforms {
		titles := byPlatform[p]
		t.Logf("\n  [%s] (%d unmatched):", p, len(titles))
		for _, title := range titles {
			t.Logf("    - %s", title)
		}
	}

	t.Logf("\n--- Matched Games (sample) ---")
	for i, r := range matched {
		if i >= 30 {
			t.Logf("    ... and %d more", len(matched)-30)
			break
		}
		t.Logf("    [%s] %q → %q (ID: %s)", r.candidate.platform, r.candidate.title, r.matched.Title, r.matched.ExternalID)
	}

	if pct < 30 {
		t.Errorf("match rate %.1f%% is too low (expected at least 30%%)", pct)
	}
}
