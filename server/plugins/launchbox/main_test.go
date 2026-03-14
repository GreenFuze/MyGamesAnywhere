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

func TestStreamParseFiles(t *testing.T) {
	const sample = `<?xml version="1.0" standalone="yes"?>
<LaunchBox>
  <File>
    <Platform>Sony Playstation</Platform>
    <FileName>Crash Bandicoot (USA)</FileName>
    <GameName>Crash Bandicoot</GameName>
  </File>
  <File>
    <Platform>Nintendo Game Boy Advance</Platform>
    <FileName>Pokemon - Fire Red Version (USA, Europe)</FileName>
    <GameName>Pokemon FireRed Version</GameName>
  </File>
</LaunchBox>`

	entries, err := streamParseFiles(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("streamParseFiles: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Platform != "Sony Playstation" || entries[0].GameName != "Crash Bandicoot" {
		t.Errorf("entry 0: got %+v", entries[0])
	}
	if entries[1].FileName != "Pokemon - Fire Red Version (USA, Europe)" {
		t.Errorf("entry 1 filename: got %q", entries[1].FileName)
	}
}

func TestStreamParseMame(t *testing.T) {
	const sample = `<?xml version="1.0" standalone="yes"?>
<LaunchBox>
  <MameFile>
    <FileName>dkong</FileName>
    <Name>Donkey Kong</Name>
    <Status>good</Status>
    <Publisher>Nintendo</Publisher>
    <Year>1981</Year>
    <IsMechanical>false</IsMechanical>
  </MameFile>
  <MameFile>
    <FileName>sf2</FileName>
    <Name>Street Fighter II: The World Warrior</Name>
    <Status>good</Status>
    <Publisher>Capcom</Publisher>
    <Year>1991</Year>
    <IsMechanical>false</IsMechanical>
  </MameFile>
</LaunchBox>`

	entries, err := streamParseMame(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("streamParseMame: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Platform != "Arcade" || entries[0].FileName != "dkong" || entries[0].GameName != "Donkey Kong" {
		t.Errorf("entry 0: got %+v", entries[0])
	}
}

func TestStreamParseMetadata(t *testing.T) {
	const sample = `<?xml version="1.0" standalone="yes"?>
<LaunchBox>
  <Game>
    <Name>Duke Nukem 3D</Name>
    <ReleaseYear>1996</ReleaseYear>
    <DatabaseID>3791</DatabaseID>
    <Platform>MS-DOS</Platform>
    <Genres>Shooter</Genres>
    <Developer>3D Realms Entertainment</Developer>
    <Publisher>GT Interactive</Publisher>
  </Game>
  <Game>
    <Name>Donkey Kong</Name>
    <ReleaseYear>1981</ReleaseYear>
    <DatabaseID>88</DatabaseID>
    <Platform>Arcade</Platform>
    <Genres>Platform</Genres>
    <Developer>Nintendo</Developer>
    <Publisher>Nintendo</Publisher>
  </Game>
</LaunchBox>`

	entries, err := streamParseMetadata(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("streamParseMetadata: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].DatabaseID != 3791 || entries[0].Name != "Duke Nukem 3D" || entries[0].Platform != "MS-DOS" {
		t.Errorf("entry 0: got %+v", entries[0])
	}
	if entries[1].DatabaseID != 88 || entries[1].Name != "Donkey Kong" {
		t.Errorf("entry 1: got %+v", entries[1])
	}
}

func TestMatchGame_FileBasedLookup(t *testing.T) {
	idx := &launchBoxIndex{
		files: map[string]*fileEntry{
			"arcade\tdkong": {Platform: "Arcade", FileName: "dkong", GameName: "Donkey Kong"},
		},
		games: map[string]*gameEntry{
			"arcade\tdonkey kong": {DatabaseID: 88, Name: "Donkey Kong", Platform: "Arcade", Year: "1981"},
		},
	}

	q := gameQuery{Index: 0, Title: "dkong", Platform: "arcade", RootPath: "Mame/dkong.zip"}
	r := matchGame(idx, q)
	if r == nil {
		t.Fatal("expected match")
	}
	if r.Title != "Donkey Kong" {
		t.Errorf("title: got %q, want %q", r.Title, "Donkey Kong")
	}
	if r.ExternalID != "88" {
		t.Errorf("external_id: got %q, want %q", r.ExternalID, "88")
	}
}

func TestMatchGame_TitleBasedLookup(t *testing.T) {
	idx := &launchBoxIndex{
		files: map[string]*fileEntry{},
		games: map[string]*gameEntry{
			"ms-dos\tduke nukem 3d": {DatabaseID: 3791, Name: "Duke Nukem 3D", Platform: "MS-DOS", Year: "1996"},
		},
	}

	q := gameQuery{Index: 0, Title: "Duke Nukem 3D", Platform: "ms_dos"}
	r := matchGame(idx, q)
	if r == nil {
		t.Fatal("expected match")
	}
	if r.ExternalID != "3791" {
		t.Errorf("external_id: got %q, want %q", r.ExternalID, "3791")
	}
	if r.URL != "https://gamesdb.launchbox-app.com/games/details/3791" {
		t.Errorf("url: got %q", r.URL)
	}
}

func TestMatchGame_NoMatch(t *testing.T) {
	idx := &launchBoxIndex{
		files: map[string]*fileEntry{},
		games: map[string]*gameEntry{},
	}

	q := gameQuery{Index: 0, Title: "Unknown Game", Platform: "windows_pc"}
	r := matchGame(idx, q)
	if r != nil {
		t.Errorf("expected no match, got %+v", r)
	}
}

func TestTitleVariations(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"inca 2", []string{"inca ii"}},
		{"inca ii", []string{"inca 2"}},
		{"discworld 1", []string{"discworld i", "discworld"}},
		{"final fantasy vii", []string{"final fantasy 7"}},
		{"final fantasy 7", []string{"final fantasy vii"}},
		{"half life", nil},
		{"doom", nil},
	}
	for _, tc := range tests {
		got := titleVariations(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("titleVariations(%q): got %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("titleVariations(%q)[%d]: got %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestMatchGame_NumeralVariation(t *testing.T) {
	idx := &launchBoxIndex{
		files:      map[string]*fileEntry{},
		games:      map[string]*gameEntry{},
		normalized: map[string]*gameEntry{
			"scummvm\tinca ii": {DatabaseID: 999, Name: "Inca II", Platform: "ScummVM"},
			"ms-dos\tdiscworld": {DatabaseID: 888, Name: "Discworld", Platform: "MS-DOS"},
		},
		byPlatform: map[string][]tokenedGame{},
	}

	q1 := gameQuery{Index: 0, Title: "Inca 2", Platform: "scummvm"}
	r1 := matchGame(idx, q1)
	if r1 == nil {
		t.Fatal("expected match for 'Inca 2' → 'Inca II'")
	}
	if r1.Title != "Inca II" {
		t.Errorf("got title %q, want %q", r1.Title, "Inca II")
	}

	q2 := gameQuery{Index: 0, Title: "Discworld 1", Platform: "scummvm"}
	r2 := matchGame(idx, q2)
	if r2 == nil {
		t.Fatal("expected match for 'Discworld 1' → 'Discworld'")
	}
	if r2.Title != "Discworld" {
		t.Errorf("got title %q, want %q", r2.Title, "Discworld")
	}
}

func TestMatchGame_ScummVMFallback(t *testing.T) {
	idx := &launchBoxIndex{
		files: map[string]*fileEntry{},
		games: map[string]*gameEntry{
			"ms-dos\tday of the tentacle": {DatabaseID: 500, Name: "Day of the Tentacle", Platform: "MS-DOS"},
		},
	}

	q := gameQuery{Index: 0, Title: "Day of the Tentacle", Platform: "scummvm"}
	r := matchGame(idx, q)
	if r == nil {
		t.Fatal("expected match via ScummVM→MS-DOS fallback")
	}
	if r.ExternalID != "500" {
		t.Errorf("external_id: got %q", r.ExternalID)
	}
}

// --- Coverage test: match tv2_games.json against the real index ---

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
	if os.Getenv("LAUNCHBOX_INTEGRATION") == "" {
		t.Skip("set LAUNCHBOX_INTEGRATION=1 to run")
	}

	tmpDir, err := os.MkdirTemp("", "launchbox-coverage-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	cache = nil

	if _, errObj := handleInit(); errObj != nil {
		t.Fatalf("handleInit: %s: %s", errObj.Code, errObj.Message)
	}
	idx, err := loadIndex()
	if err != nil {
		t.Fatalf("loadIndex: %v", err)
	}

	tv2Path := filepath.Join(origDir, "..", "..", "internal", "scan", "scanner", "testdata", "tv2_games.json")
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
		"MS DOS":                "ms_dos",
		"Nintendo Game Boy Advanced": "gba",
		"Playstation":           "ps1",
		"Playstation 2":         "ps2",
		"Playstation 3":         "ps3",
		"Playstation Portable":  "psp",
		"XBox 360":              "xbox_360",
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
			raw := parts[1]
			title := cleanTitle(raw)
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

	for _, c := range candidates {
		q := gameQuery{
			Index:    0,
			Title:    c.title,
			Platform: c.platform,
			RootPath: c.rootPath,
		}
		r := matchGame(idx, q)
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

	t.Logf("\n=== TV2 Games Coverage Report ===")
	t.Logf("Total candidates: %d", total)
	t.Logf("Matched:          %d (%.1f%%)", matchCount, pct)
	t.Logf("Unmatched:        %d (%.1f%%)", missCount, 100-pct)

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
		if i >= 20 {
			t.Logf("    ... and %d more", len(matched)-20)
			break
		}
		t.Logf("    [%s] %q → %q (ID: %s)", r.candidate.platform, r.candidate.title, r.matched.Title, r.matched.ExternalID)
	}

	if pct < 30 {
		t.Errorf("match rate %.1f%% is too low (expected at least 30%%)", pct)
	}
}

// --- Integration test: full download + parse + index ---

func TestFullInit(t *testing.T) {
	if os.Getenv("LAUNCHBOX_INTEGRATION") == "" {
		t.Skip("set LAUNCHBOX_INTEGRATION=1 to run integration tests")
	}

	tmpDir, err := os.MkdirTemp("", "launchbox-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	cache = nil

	result, errObj := handleInit()
	if errObj != nil {
		t.Fatalf("handleInit error: %s: %s", errObj.Code, errObj.Message)
	}
	t.Logf("init result: %v", result)

	if _, err := os.Stat("data/files-index.json"); err != nil {
		t.Fatalf("files-index.json not found: %v", err)
	}
	if _, err := os.Stat("data/games-index.json"); err != nil {
		t.Fatalf("games-index.json not found: %v", err)
	}

	fi, _ := os.Stat("data/files-index.json")
	gi, _ := os.Stat("data/games-index.json")
	t.Logf("files-index.json: %d bytes", fi.Size())
	t.Logf("games-index.json: %d bytes", gi.Size())

	idx, err := loadIndex()
	if err != nil {
		t.Fatalf("loadIndex: %v", err)
	}
	t.Logf("loaded %d file mappings, %d game entries", len(idx.files), len(idx.games))

	if len(idx.files) < 50000 {
		t.Errorf("expected at least 50K file mappings, got %d", len(idx.files))
	}
	if len(idx.games) < 100000 {
		t.Errorf("expected at least 100K game entries, got %d", len(idx.games))
	}

	// Spot-check well-known games.
	checks := []struct {
		ourPlatform string
		title       string
		rootPath    string
		wantTitle   string
	}{
		{"arcade", "dkong", "Mame/dkong.zip", "Donkey Kong"},
		{"ms_dos", "Duke Nukem 3D", "", "Duke Nukem 3D"},
		{"arcade", "sf2", "Mame/sf2.zip", "Street Fighter II: The World Warrior"},
	}

	for _, c := range checks {
		q := gameQuery{Index: 0, Title: c.title, Platform: c.ourPlatform, RootPath: c.rootPath}
		r := matchGame(idx, q)
		if r == nil {
			t.Errorf("no match for %q (platform %s)", c.title, c.ourPlatform)
			continue
		}
		if r.Title != c.wantTitle {
			t.Errorf("%q: got title %q, want %q", c.title, r.Title, c.wantTitle)
		}
		t.Logf("matched %q → %q (ID: %s)", c.title, r.Title, r.ExternalID)
	}
}
