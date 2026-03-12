package scan

import (
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestPlatformDetect_TV2(t *testing.T) {
	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)
	groups := NewFileGrouper().Group(annotated)
	NewPlatformDetector().DetectAll(groups)

	type expectation struct {
		rootDir  string
		name     string
		platform core.Platform
	}
	expectations := []expectation{
		// Mame → arcade
		{"Mame", "dkong3", core.PlatformArcade},
		// Roms/MS DOS → ms_dos
		{"Roms/MS DOS/bonus", "bonus", core.PlatformMSDOS},
		// Roms/Nintendo Game Boy Advanced → gba
		{"Roms/Nintendo Game Boy Advanced", "", core.PlatformGBA},
		// Roms/Playstation → ps1
		{"Roms/Playstation", "castlevania - symphony of the night (usa)", core.PlatformPS1},
		// Roms/Playstation 2 → ps2
		{"Roms/Playstation 2", "god of war (usa)", core.PlatformPS2},
		// Roms/Playstation 3 → ps3
		{"Roms/Playstation 3/Devil May Cry - HD Collection (USA) (En,Ja,Fr,De,Es,It)", "", core.PlatformPS3},
		// Roms/Playstation Portable → psp
		{"Roms/Playstation Portable", "", core.PlatformPSP},
		// Roms/XBox 360 → xbox_360
		{"Roms/XBox 360", "", core.PlatformXbox360},
		// ScummVM → scummvm
		{"ScummVM/Castle of Dr. Brain (CD DOS)", "Castle of Dr. Brain (CD DOS)", core.PlatformScummVM},
		// Hangman has .exe → windows_pc (no path hint)
		{"Hangman", "Hangman", core.PlatformWindowsPC},
	}

	byKey := map[string]GameGroup{}
	for _, g := range groups {
		byKey[g.RootDir+"|"+g.Name] = g
	}

	for _, exp := range expectations {
		found := false
		for _, g := range groups {
			matchDir := g.RootDir == exp.rootDir
			matchName := exp.name == "" || g.Name == exp.name
			if matchDir && matchName {
				found = true
				if g.Platform != exp.platform {
					t.Errorf("[%s/%s] platform = %q, want %q",
						g.RootDir, g.Name, g.Platform, exp.platform)
				}
				break
			}
		}
		if !found {
			t.Errorf("no group matching rootDir=%q name=%q", exp.rootDir, exp.name)
		}
	}

	// Log all platforms for inspection
	platCounts := map[core.Platform]int{}
	for _, g := range groups {
		platCounts[g.Platform]++
	}
	t.Log("Platform distribution:")
	for p, n := range platCounts {
		t.Logf("  %-20s %d groups", p, n)
	}
}

func TestPlatformDetect_Installers(t *testing.T) {
	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)
	groups := NewFileGrouper().Group(annotated)
	NewPlatformDetector().DetectAll(groups)

	for _, g := range groups {
		if g.RootDir != "Installers" {
			continue
		}
		hasExe := false
		for _, f := range g.Files {
			if f.Kind == FileKindExecutable {
				hasExe = true
				break
			}
		}
		if hasExe && g.Platform != core.PlatformWindowsPC {
			t.Errorf("Installers/%s has .exe but platform = %q, want windows_pc",
				g.Name, g.Platform)
		}
		if !hasExe && g.Platform != core.PlatformUnknown {
			t.Errorf("Installers/%s has no .exe but platform = %q, want unknown",
				g.Name, g.Platform)
		}
	}
}

func TestPlatformDetect_PathRulePrecedence(t *testing.T) {
	detector := NewPlatformDetector()

	tests := []struct {
		rootDir  string
		platform core.Platform
	}{
		{"Roms/Playstation Portable/SomeGame", core.PlatformPSP},
		{"Roms/Playstation 3/SomeGame", core.PlatformPS3},
		{"Roms/Playstation 2/SomeGame", core.PlatformPS2},
		{"Roms/Playstation/SomeGame", core.PlatformPS1},
		{"Mame", core.PlatformArcade},
		{"ScummVM/Some Game (CD DOS)", core.PlatformScummVM},
		{"Games/MS DOS/mygame", core.PlatformMSDOS},
		{"", core.PlatformUnknown},
	}

	for _, tt := range tests {
		got := detector.detectFromPath(tt.rootDir)
		if got != tt.platform {
			t.Errorf("detectFromPath(%q) = %q, want %q", tt.rootDir, got, tt.platform)
		}
	}
}

func TestPlatformDetect_FileSignals(t *testing.T) {
	tests := []struct {
		name     string
		files    []AnnotatedFile
		platform core.Platform
	}{
		{
			name: "PS3 disc structure",
			files: []AnnotatedFile{
				{FileEntry: core.FileEntry{Name: "PS3_DISC.SFB", Path: "game/PS3_DISC.SFB"}},
				{FileEntry: core.FileEntry{Name: "ICON0.PNG", Path: "game/PS3_GAME/ICON0.PNG"}},
			},
			platform: core.PlatformPS3,
		},
		{
			name: "PS3 files under PS3_GAME",
			files: []AnnotatedFile{
				{FileEntry: core.FileEntry{Name: "EBOOT.BIN", Path: "game/PS3_GAME/USRDIR/EBOOT.BIN"}},
			},
			platform: core.PlatformPS3,
		},
		{
			name: "DOS game with .com",
			files: []AnnotatedFile{
				{FileEntry: core.FileEntry{Name: "GAME.COM", Path: "dos/GAME.COM"}, Kind: FileKindDOSExecutable, Extension: ".com"},
			},
			platform: core.PlatformMSDOS,
		},
		{
			name: "DOS game with conf+bat",
			files: []AnnotatedFile{
				{FileEntry: core.FileEntry{Name: "dosbox.conf", Path: "dos/dosbox.conf"}, Extension: ".conf"},
				{FileEntry: core.FileEntry{Name: "RUN.BAT", Path: "dos/RUN.BAT"}, Kind: FileKindScript, Extension: ".bat"},
			},
			platform: core.PlatformMSDOS,
		},
		{
			name: "Windows exe",
			files: []AnnotatedFile{
				{FileEntry: core.FileEntry{Name: "setup.exe", Path: "game/setup.exe"}, Kind: FileKindExecutable, Extension: ".exe"},
			},
			platform: core.PlatformWindowsPC,
		},
		{
			name: "Archive only - unknown",
			files: []AnnotatedFile{
				{FileEntry: core.FileEntry{Name: "game.zip", Path: "stuff/game.zip"}, Kind: FileKindArchive, Extension: ".zip"},
			},
			platform: core.PlatformUnknown,
		},
	}

	for _, tt := range tests {
		got := detectFromFiles(tt.files)
		if got != tt.platform {
			t.Errorf("%s: detectFromFiles = %q, want %q", tt.name, got, tt.platform)
		}
	}
}
