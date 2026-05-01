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

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	os.WriteFile(cfgPath, []byte(`{"api_key":"test_key_12345"}`), 0o644)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.APIKey != "test_key_12345" {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	_, err := loadConfig()
	if err == nil {
		t.Error("expected error for missing config.json")
	}
}

func TestBuildSearchQueries(t *testing.T) {
	// Platform with a RAWG mapping should produce 3 passes.
	passes := buildSearchQueries("windows_pc")
	if len(passes) != 3 {
		t.Fatalf("expected 3 passes for windows_pc, got %d", len(passes))
	}
	if passes[0].platformID != 4 {
		t.Errorf("pass 1 should have platformID=4, got %d", passes[0].platformID)
	}
	if passes[0].exact {
		t.Error("pass 1 should not be exact")
	}
	if passes[1].platformID != 0 {
		t.Errorf("pass 2 should have platformID=0, got %d", passes[1].platformID)
	}
	if passes[2].platformID != 0 || !passes[2].exact {
		t.Errorf("pass 3 should be exact without platform filter: %+v", passes[2])
	}

	n64Passes := buildSearchQueries("n64")
	if len(n64Passes) != 3 || n64Passes[0].platformID != 83 {
		t.Fatalf("n64 passes = %+v, want first platformID=83", n64Passes)
	}

	// Arcade has no RAWG platform → only 2 passes (no platform-filtered pass).
	arcadePasses := buildSearchQueries("arcade")
	if len(arcadePasses) != 2 {
		t.Fatalf("expected 2 passes for arcade, got %d", len(arcadePasses))
	}
	if arcadePasses[0].platformID != 0 {
		t.Errorf("arcade pass 1 should have platformID=0, got %d", arcadePasses[0].platformID)
	}
}

// --- Integration tests (require real RAWG API key) ---

func loadTestConfig(t *testing.T) {
	t.Helper()
	if os.Getenv("RAWG_INTEGRATION") == "" {
		t.Skip("set RAWG_INTEGRATION=1 to run integration tests")
	}

	origDir, _ := os.Getwd()

	srcConfigPath := filepath.Join(origDir, "config.json")
	if _, err := os.Stat(srcConfigPath); err != nil {
		t.Skipf("no config.json found at %s", srcConfigPath)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	apiKey = cfg.APIKey
}

func TestMatchGame(t *testing.T) {
	loadTestConfig(t)

	tests := []struct {
		title    string
		platform string
		wantAny  []string
	}{
		{"Donkey Kong", "arcade", []string{"Donkey Kong"}},
		{"Duke Nukem 3D", "ms_dos", []string{"Duke Nukem 3D"}},
		{"Half-Life 2", "windows_pc", []string{"Half-Life 2"}},
		{"Sonic the Hedgehog 2", "arcade", []string{"Sonic the Hedgehog 2"}},
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
	if os.Getenv("RAWG_INTEGRATION") == "" {
		t.Skip("set RAWG_INTEGRATION=1 to run")
	}
	loadTestConfig(t)

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

	t.Logf("\n=== RAWG TV2 Games Coverage Report ===")
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
