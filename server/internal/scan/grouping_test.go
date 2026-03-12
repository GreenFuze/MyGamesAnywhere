package scan

import (
	"sort"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestGroupFiles_TV2(t *testing.T) {
	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)
	grouper := NewFileGrouper()
	groups := grouper.Group(annotated)

	if len(groups) == 0 {
		t.Fatal("expected at least one group")
	}

	byName := map[string]GameGroup{}
	for _, g := range groups {
		byName[g.RootDir+"/"+g.Name] = g
	}

	t.Logf("Total groups: %d", len(groups))
	for _, g := range groups {
		t.Logf("  [%s] %-60s  (%d files)", g.RootDir, g.Name, len(g.Files))
	}

	// ── Hangman: single game directory ──
	hg, ok := byName["Hangman/Hangman"]
	if !ok {
		t.Error("missing group for Hangman")
	} else if len(hg.Files) != 7 {
		t.Errorf("Hangman: expected 7 files, got %d", len(hg.Files))
	}

	// ── Mame: each zip is a separate group ──
	mameCount := 0
	for _, g := range groups {
		if g.RootDir == "Mame" {
			mameCount++
			if len(g.Files) != 1 {
				t.Errorf("Mame group %q: expected 1 file, got %d", g.Name, len(g.Files))
			}
		}
	}
	if mameCount < 10 {
		t.Errorf("expected at least 10 Mame groups, got %d", mameCount)
	}

	// ── Installers: multi-file clustering ──
	legoBatman, ok := byName["Installers/setup_lego_batman_1.0_(18156)"] // already lowercase
	if !ok {
		t.Error("missing group for Lego Batman installer")
	} else if len(legoBatman.Files) != 2 {
		t.Errorf("Lego Batman installer: expected 2 files (exe + bin), got %d", len(legoBatman.Files))
	}

	legoDCSV, ok := byName["Installers/setup_lego_dc_super-villains_1.0_(64bit)_(57222)"]
	if !ok {
		t.Error("missing group for Lego DC Super-Villains installer")
	} else if len(legoDCSV.Files) != 6 {
		t.Errorf("Lego DC Super-Villains: expected 6 files (exe + 5 bin), got %d", len(legoDCSV.Files))
	}

	beamng, ok := byName["Installers/beamng.drive.v0.29.0"]
	if !ok {
		t.Error("missing group for BeamNG.Drive")
	} else if len(beamng.Files) != 1 {
		t.Errorf("BeamNG.Drive: expected 1 file, got %d", len(beamng.Files))
	}

	// ── Roms/Playstation: disc image grouping ──
	castlevania, ok := byName["Roms/Playstation/castlevania - symphony of the night (usa)"]
	if !ok {
		t.Error("missing group for Castlevania PS1")
	} else if len(castlevania.Files) != 3 {
		t.Errorf("Castlevania PS1: expected 3 files (cue + 2 bin), got %d", len(castlevania.Files))
	}

	megaman, ok := byName["Roms/Playstation/mega man x4 (usa)"]
	if !ok {
		t.Error("missing group for Mega Man X4")
	} else if len(megaman.Files) != 2 {
		t.Errorf("Mega Man X4: expected 2 files (cue + bin), got %d", len(megaman.Files))
	}

	// ── Roms/Playstation 2: single ISO ──
	godOfWar, ok := byName["Roms/Playstation 2/god of war (usa)"]
	if !ok {
		t.Error("missing group for God of War PS2")
	} else if len(godOfWar.Files) != 1 {
		t.Errorf("God of War: expected 1 file, got %d", len(godOfWar.Files))
	}

	// ── Roms/Playstation 3: extracted directory is one game ──
	ps3Game := false
	for _, g := range groups {
		if g.RootDir == "Roms/Playstation 3/Devil May Cry - HD Collection (USA) (En,Ja,Fr,De,Es,It)" {
			ps3Game = true
			if len(g.Files) < 5 {
				t.Errorf("Devil May Cry PS3: expected at least 5 files, got %d", len(g.Files))
			}
		}
	}
	if !ps3Game {
		t.Error("missing group for Devil May Cry PS3")
	}

	// ── ScummVM: each game dir is one group ──
	castleBrain := false
	for _, g := range groups {
		if g.Name == "Castle of Dr. Brain (CD DOS)" {
			castleBrain = true
			if len(g.Files) < 10 {
				t.Errorf("Castle of Dr. Brain: expected at least 10 files, got %d", len(g.Files))
			}
		}
	}
	if !castleBrain {
		t.Error("missing group for Castle of Dr. Brain")
	}

	// ── Every non-dir file should appear in exactly one group ──
	fileCount := map[string]int{}
	for _, g := range groups {
		for _, f := range g.Files {
			fileCount[f.Path]++
		}
	}
	totalNonDir := 0
	for _, f := range annotated {
		if !f.IsDir {
			totalNonDir++
		}
	}
	duplicates := 0
	for path, count := range fileCount {
		if count > 1 {
			t.Errorf("file in multiple groups: %s (count %d)", path, count)
			duplicates++
		}
	}
	if duplicates > 0 {
		t.Errorf("%d files appear in multiple groups", duplicates)
	}
	if len(fileCount) != totalNonDir {
		t.Errorf("grouped %d files but have %d non-dir entries (missing %d)",
			len(fileCount), totalNonDir, totalNonDir-len(fileCount))
	}
}

func TestGroupFiles_ContainerDetection(t *testing.T) {
	tests := []struct {
		name        string
		isContainer bool
	}{
		{"Installers", true},
		{"Mame", true},
		{"Roms", true},
		{"ScummVM", true},
		{"Hangman", false},
	}

	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)
	grouper := NewFileGrouper()
	tree := buildTree(annotated)

	for _, tt := range tests {
		child, ok := tree.children[tt.name]
		if !ok {
			t.Errorf("missing tree node: %s", tt.name)
			continue
		}
		got := grouper.isContainer(child)
		if got != tt.isContainer {
			t.Errorf("%s: isContainer = %v, want %v", tt.name, got, tt.isContainer)
		}
	}
}

func TestClusterKey(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"setup_lego_batman_1.0_(18156).exe", "setup_lego_batman_1.0_(18156)"},
		{"setup_lego_batman_1.0_(18156)-1.bin", "setup_lego_batman_1.0_(18156)"},
		{"setup_lego_batman_1.0_(18156)-2.bin", "setup_lego_batman_1.0_(18156)"},
		{"Castlevania - Symphony of the Night (USA).cue", "castlevania - symphony of the night (usa)"},
		{"Castlevania - Symphony of the Night (USA) (Track 1).bin", "castlevania - symphony of the night (usa)"},
		{"Castlevania - Symphony of the Night (USA) (Track 2).bin", "castlevania - symphony of the night (usa)"},
		{"BeamNG.Drive.v0.29.0.zip", "beamng.drive.v0.29.0"},
		{"dkong3.zip", "dkong3"},
		{"God of War (USA).iso", "god of war (usa)"},
		{"game (Disc 1).iso", "game"},
		{"game (Disc 2).iso", "game"},
		// Mixed case should cluster together
		{"Setup_Game.EXE", "setup_game"},
		{"setup_game-1.BIN", "setup_game"},
	}
	for _, tt := range tests {
		got := clusterKey(tt.filename)
		if got != tt.want {
			t.Errorf("clusterKey(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestGroupFiles_Synthetic_MultiGameFlatDir(t *testing.T) {
	files := annotateFiles([]core.FileEntry{
		{Path: "Stuff/GameA.zip", Name: "GameA.zip", Size: 1000},
		{Path: "Stuff/GameB.zip", Name: "GameB.zip", Size: 2000},
		{Path: "Stuff/GameC.exe", Name: "GameC.exe", Size: 500},
	})
	groups := NewFileGrouper().Group(files)
	if len(groups) != 3 {
		t.Errorf("expected 3 groups, got %d", len(groups))
		for _, g := range groups {
			t.Logf("  %s (%d files)", g.Name, len(g.Files))
		}
	}
}

func TestGroupFiles_Synthetic_SingleGameDir(t *testing.T) {
	files := annotateFiles([]core.FileEntry{
		{Path: "MyGame/game.exe", Name: "game.exe", Size: 1000},
		{Path: "MyGame/data.txt", Name: "data.txt", Size: 200},
		{Path: "MyGame/music.mp3", Name: "music.mp3", Size: 5000},
	})
	groups := NewFileGrouper().Group(files)
	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "MyGame" {
		t.Errorf("expected name MyGame, got %q", groups[0].Name)
	}
	if len(groups[0].Files) != 3 {
		t.Errorf("expected 3 files, got %d", len(groups[0].Files))
	}
}

func TestGroupFiles_Synthetic_TwoExeGameDir(t *testing.T) {
	files := annotateFiles([]core.FileEntry{
		{Path: "DOSGame/GAME.EXE", Name: "GAME.EXE", Size: 1000},
		{Path: "DOSGame/SETUP.EXE", Name: "SETUP.EXE", Size: 500},
		{Path: "DOSGame/DATA.DAT", Name: "DATA.DAT", Size: 2000},
		{Path: "DOSGame/README.TXT", Name: "README.TXT", Size: 100},
	})
	groups := NewFileGrouper().Group(files)
	names := []string{}
	for _, g := range groups {
		names = append(names, g.Name)
	}
	sort.Strings(names)
	if len(groups) != 1 {
		t.Errorf("expected 1 group (DOS game with 2 exes), got %d: %v", len(groups), names)
	}
}
