package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
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

func TestJaccardSimilarity(t *testing.T) {
	a := map[string]bool{"duke": true, "nukem": true, "3d": true}
	b := map[string]bool{"duke": true, "nukem": true, "3d": true}
	if s := jaccardSimilarity(a, b); s != 1.0 {
		t.Errorf("identical sets: got %f, want 1.0", s)
	}

	c := map[string]bool{"duke": true, "nukem": true, "3d": true, "atomic": true, "edition": true}
	s := jaccardSimilarity(a, c)
	if s < 0.5 || s > 0.7 {
		t.Errorf("overlapping sets: got %f, want ~0.6", s)
	}

	d := map[string]bool{"zelda": true, "breath": true}
	if s := jaccardSimilarity(a, d); s != 0.0 {
		t.Errorf("disjoint sets: got %f, want 0.0", s)
	}
}

func TestSupportedPlatforms(t *testing.T) {
	if !supportedPlatforms["windows_pc"] {
		t.Error("windows_pc should be supported")
	}
	if !supportedPlatforms["ms_dos"] {
		t.Error("ms_dos should be supported")
	}
	if supportedPlatforms["ps2"] {
		t.Error("ps2 should NOT be supported by Steam resolver")
	}
	if supportedPlatforms["arcade"] {
		t.Error("arcade should NOT be supported by Steam resolver")
	}
}

func TestScoreCandidate(t *testing.T) {
	tests := []struct {
		query     string
		candidate string
		wantMin   float64
		wantMax   float64
	}{
		{"half life 2", "Half-Life 2", 0.99, 1.01},
		{"duke nukem 3d", "Duke Nukem 3D", 0.99, 1.01},
		{"doom", "Doom Eternal", 0.4, 0.6},
		{"totally different", "Unrelated Game", -0.1, 0.1},
		// Prefix-subtitle bonus: 3+ token query, non-sequel suffix.
		{"duke nukem 3d", "Duke Nukem 3D: 20th Anniversary World Tour", 0.76, 0.95},
		// Episode/expansion suffix (non-sequel) → gets prefix bonus.
		{"half life 2", "Half-Life 2: Episode One", 0.80, 0.90},
	}
	for _, tc := range tests {
		queryTokens := tokenize(tc.query)
		score := scoreCandidate(tc.query, queryTokens, tc.candidate)
		if score < tc.wantMin || score > tc.wantMax {
			t.Errorf("scoreCandidate(%q, %q) = %.2f, want [%.2f, %.2f]",
				tc.query, tc.candidate, score, tc.wantMin, tc.wantMax)
		}
	}
}

func TestIsSequelSuffix(t *testing.T) {
	tests := []struct {
		suffix string
		want   bool
	}{
		{"2 episode one", true},
		{"3", true},
		{"ii enhanced", true},
		{"iii", true},
		{"enhanced edition", false},
		{"complete collection", false},
		{"20th anniversary world tour", false},
		{"remastered", false},
		{"64", true},
		{"25th anniversary", false},
	}
	for _, tc := range tests {
		got := isSequelSuffix(tc.suffix)
		if got != tc.want {
			t.Errorf("isSequelSuffix(%q) = %v, want %v", tc.suffix, got, tc.want)
		}
	}
}

// --- Integration tests (require Steam API access) ---

func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("STEAM_INTEGRATION") == "" {
		t.Skip("set STEAM_INTEGRATION=1 to run integration tests")
	}
}

func TestSteamSearch(t *testing.T) {
	skipUnlessIntegration(t)

	tests := []struct {
		query   string
		wantAny []string
	}{
		{"Half-Life 2", []string{"Half-Life 2"}},
		{"Portal", []string{"Portal"}},
		{"Team Fortress 2", []string{"Team Fortress 2"}},
	}

	for _, tc := range tests {
		results, err := steamSearch(tc.query)
		if err != nil {
			t.Errorf("%q: error: %v", tc.query, err)
			continue
		}
		if len(results) == 0 {
			t.Errorf("%q: no results", tc.query)
			continue
		}
		t.Logf("%q: %d results", tc.query, len(results))
		for _, r := range results {
			t.Logf("  → %q (appid: %s)", r.Name, r.AppID)
		}
		found := false
		for _, r := range results {
			for _, want := range tc.wantAny {
				if normalizeTitle(r.Name) == normalizeTitle(want) {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%q: none of %v found in results", tc.query, tc.wantAny)
		}
	}
}

func TestMatchGame(t *testing.T) {
	skipUnlessIntegration(t)

	tests := []struct {
		title    string
		platform string
		wantAny  []string
	}{
		{"Half-Life 2", "windows_pc", []string{"Half-Life 2"}},
		{"Portal", "windows_pc", []string{"Portal"}},
		{"Team Fortress 2", "windows_pc", []string{"Team Fortress 2"}},
		{"Doom", "windows_pc", []string{"DOOM", "Doom"}},
	}

	for _, tc := range tests {
		q := gameQuery{Index: 0, Title: tc.title, Platform: tc.platform}
		r, err := matchGame(q)
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

func TestMatchGameSkipsConsoles(t *testing.T) {
	skipUnlessIntegration(t)

	consolePlatforms := []string{"arcade", "gba", "ps1", "ps2", "ps3", "psp", "xbox_360"}
	for _, p := range consolePlatforms {
		q := gameQuery{Index: 0, Title: "Test Game", Platform: p}
		r, err := matchGame(q)
		if err != nil {
			t.Errorf("platform %q: unexpected error: %v", p, err)
		}
		if r != nil {
			t.Errorf("platform %q: expected nil result for unsupported platform, got %+v", p, r)
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
	if os.Getenv("STEAM_INTEGRATION") == "" {
		t.Skip("set STEAM_INTEGRATION=1 to run")
	}

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
		"MS DOS": "ms_dos",
	}

	seenDirs := map[string]bool{}
	for _, e := range entries {
		parts := strings.Split(e.Path, "\\")

		switch {
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

	t.Logf("extracted %d PC game candidates from tv2_games.json", len(candidates))

	type result struct {
		candidate gameCandidate
		matched   *lookupResult
	}
	var matched, unmatched []result
	errors := 0

	for i, c := range candidates {
		if i > 0 && i%25 == 0 {
			pct := float64(len(matched)) / float64(i) * 100
			t.Logf("progress: %d/%d (%.0f%% matched so far, %d errors)", i, len(candidates), pct, errors)
		}

		q := gameQuery{
			Index:    0,
			Title:    c.title,
			Platform: c.platform,
			RootPath: c.rootPath,
		}
		r, err := matchGame(q)
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

	t.Logf("\n=== Steam TV2 Games Coverage Report ===")
	t.Logf("Total PC candidates: %d", total)
	t.Logf("Matched:             %d (%.1f%%)", matchCount, pct)
	t.Logf("Unmatched:           %d (%.1f%%)", missCount, 100-pct)
	t.Logf("Errors:              %d", errors)

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

	if pct < 15 {
		t.Errorf("match rate %.1f%% is too low (expected at least 15%%)", pct)
	}
}
