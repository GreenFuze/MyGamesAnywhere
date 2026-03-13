package main

import (
	"os"
	"strings"
	"testing"
)

func TestStreamParseMAME(t *testing.T) {
	const sample = `<?xml version="1.0"?>
<mame build="0.286 (mame0286)" debug="no" mameconfig="10">
	<machine name="005" sourcefile="sega/segag80r.cpp" sampleof="005">
		<description>005</description>
		<year>1981</year>
		<manufacturer>Sega</manufacturer>
		<rom name="1346b.cpu-u25" size="2048" crc="8e68533e" region="maincpu" offset="0"/>
		<chip type="cpu" tag="maincpu" name="Zilog Z80" clock="3867120"/>
	</machine>
	<machine name="005a" sourcefile="sega/segag80r.cpp" cloneof="005" romof="005" sampleof="005">
		<description>005 (earlier version?)</description>
		<year>1981</year>
		<manufacturer>Sega</manufacturer>
		<rom name="1346b.cpu-u25" size="2048" crc="8e68533e" region="maincpu" offset="0"/>
	</machine>
	<machine name="puckman" sourcefile="namco/pacman.cpp">
		<description>Puck Man (Japan set 1)</description>
		<year>1980</year>
		<manufacturer>Namco</manufacturer>
	</machine>
</mame>`

	entries, err := streamParseMAME(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("streamParseMAME: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	tests := []struct {
		name, cloneOf, desc, year, mfr string
	}{
		{"005", "", "005", "1981", "Sega"},
		{"005a", "005", "005 (earlier version?)", "1981", "Sega"},
		{"puckman", "", "Puck Man (Japan set 1)", "1980", "Namco"},
	}

	for i, tt := range tests {
		e := entries[i]
		if e.Name != tt.name {
			t.Errorf("[%d] name: got %q, want %q", i, e.Name, tt.name)
		}
		if e.CloneOf != tt.cloneOf {
			t.Errorf("[%d] clone_of: got %q, want %q", i, e.CloneOf, tt.cloneOf)
		}
		if e.Description != tt.desc {
			t.Errorf("[%d] description: got %q, want %q", i, e.Description, tt.desc)
		}
		if e.Year != tt.year {
			t.Errorf("[%d] year: got %q, want %q", i, e.Year, tt.year)
		}
		if e.Manufacturer != tt.mfr {
			t.Errorf("[%d] manufacturer: got %q, want %q", i, e.Manufacturer, tt.mfr)
		}
	}
}

func TestStreamParseMAME_DatafileRoot(t *testing.T) {
	const sample = `<?xml version="1.0"?>
<datafile>
	<game name="dkong" cloneof="">
		<description>Donkey Kong (US set 1)</description>
		<year>1981</year>
		<manufacturer>Nintendo</manufacturer>
	</game>
</datafile>`

	entries, err := streamParseMAME(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("streamParseMAME: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "dkong" {
		t.Errorf("name: got %q, want %q", entries[0].Name, "dkong")
	}
}

func TestLookup(t *testing.T) {
	entries := []indexEntry{
		{Name: "005", Description: "005", Year: "1981", Manufacturer: "Sega"},
		{Name: "005a", CloneOf: "005", Description: "005 (earlier version?)", Year: "1981", Manufacturer: "Sega"},
		{Name: "puckman", Description: "Puck Man (Japan set 1)", Year: "1980", Manufacturer: "Namco"},
	}

	idx := make(map[string]*indexEntry, len(entries))
	for i := range entries {
		idx[strings.ToLower(entries[i].Name)] = &entries[i]
	}
	lk := &mameLookup{index: idx}

	if m := lk.lookup("PUCKMAN"); m == nil || m.Description != "Puck Man (Japan set 1)" {
		t.Errorf("lookup PUCKMAN: got %v", m)
	}
	if m := lk.lookup("005a"); m == nil || m.CloneOf != "005" {
		t.Errorf("lookup 005a: got %v", m)
	}
	if m := lk.lookup("nonexistent"); m != nil {
		t.Errorf("lookup nonexistent: expected nil, got %v", m)
	}
}

func TestFetchLatestRelease(t *testing.T) {
	if os.Getenv("MAME_DAT_INTEGRATION") == "" {
		t.Skip("set MAME_DAT_INTEGRATION=1 to run integration tests")
	}
	tag, url, err := fetchLatestRelease()
	if err != nil {
		t.Fatalf("fetchLatestRelease: %v", err)
	}
	if !strings.HasPrefix(tag, "mame") {
		t.Errorf("tag: got %q, expected mame* prefix", tag)
	}
	if !strings.HasSuffix(url, "lx.zip") {
		t.Errorf("url: got %q, expected *lx.zip suffix", url)
	}
	t.Logf("latest: tag=%s url=%s", tag, url)
}

func TestFullInit(t *testing.T) {
	if os.Getenv("MAME_DAT_INTEGRATION") == "" {
		t.Skip("set MAME_DAT_INTEGRATION=1 to run integration tests")
	}

	tmpDir, err := os.MkdirTemp("", "mame-dat-test-*")
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

	if _, err := os.Stat("data/index.json"); err != nil {
		t.Fatalf("index.json not found: %v", err)
	}
	if _, err := os.Stat("data/.version"); err != nil {
		t.Fatalf(".version not found: %v", err)
	}

	info, _ := os.Stat("data/index.json")
	t.Logf("index.json size: %d bytes", info.Size())

	lk, err := loadIndex()
	if err != nil {
		t.Fatalf("loadIndex: %v", err)
	}
	t.Logf("loaded %d machines", len(lk.index))

	if len(lk.index) < 40000 {
		t.Errorf("expected at least 40000 machines, got %d", len(lk.index))
	}

	// Spot check some well-known games.
	checks := map[string]string{
		"puckman": "Puck Man",
		"dkong":   "Donkey Kong",
		"sf2":     "Street Fighter II",
	}
	for name, wantPrefix := range checks {
		m := lk.lookup(name)
		if m == nil {
			t.Errorf("expected to find %q", name)
			continue
		}
		if !strings.HasPrefix(m.Description, wantPrefix) {
			t.Errorf("%s: desc=%q, want prefix %q", name, m.Description, wantPrefix)
		}
	}
}
