package scan

import (
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestClassify_TV2(t *testing.T) {
	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)
	groups := NewFileGrouper().Group(annotated)
	NewPlatformDetector().DetectAll(groups)
	NewGroupClassifier().ClassifyAll(groups)

	type expectation struct {
		rootDir   string
		name      string
		groupKind core.GroupKind
	}
	expectations := []expectation{
		// Emulated platforms → self_contained
		{"Mame", "dkong3", core.GroupKindSelfContained},
		{"Roms/MS DOS/bonus", "bonus", core.GroupKindSelfContained},
		{"Roms/Playstation", "castlevania - symphony of the night (usa)", core.GroupKindSelfContained},
		{"Roms/Playstation 2", "god of war (usa)", core.GroupKindSelfContained},
		{"Roms/Playstation 3/Devil May Cry - HD Collection (USA) (En,Ja,Fr,De,Es,It)", "", core.GroupKindSelfContained},
		{"ScummVM/Castle of Dr. Brain (CD DOS)", "Castle of Dr. Brain (CD DOS)", core.GroupKindSelfContained},

		// Windows extracted game → self_contained
		{"Hangman", "Hangman", core.GroupKindSelfContained},

		// GOG installers (exe + bin) → packed
		{"Installers", "setup_lego_batman_1.0_(18156)", core.GroupKindPacked},
		{"Installers", "setup_alone_in_the_dark_1.0_cs_(28043)", core.GroupKindPacked},

		// Standalone archives (unknown platform) → packed
		{"Installers", "beamng.drive.v0.29.0", core.GroupKindPacked},
		{"Installers", "sonic mania", core.GroupKindPacked},
	}

	for _, exp := range expectations {
		found := false
		for _, g := range groups {
			matchDir := g.RootDir == exp.rootDir
			matchName := exp.name == "" || g.Name == exp.name
			if matchDir && matchName {
				found = true
				if g.GroupKind != exp.groupKind {
					t.Errorf("[%s/%s] GroupKind = %q, want %q",
						g.RootDir, g.Name, g.GroupKind, exp.groupKind)
				}
				break
			}
		}
		if !found {
			t.Errorf("no group matching rootDir=%q name=%q", exp.rootDir, exp.name)
		}
	}

	// Log distribution
	kindCounts := map[core.GroupKind]int{}
	for _, g := range groups {
		kindCounts[g.GroupKind]++
	}
	t.Log("GroupKind distribution:")
	for k, n := range kindCounts {
		t.Logf("  %-20s %d groups", k, n)
	}
}

func TestClassify_GBA_MP3_IsExtras(t *testing.T) {
	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)
	groups := NewFileGrouper().Group(annotated)
	NewPlatformDetector().DetectAll(groups)
	NewGroupClassifier().ClassifyAll(groups)

	for _, g := range groups {
		if g.RootDir != "Roms/Nintendo Game Boy Advanced" {
			continue
		}
		allAudio := true
		for _, f := range g.Files {
			if f.Kind != FileKindAudio {
				allAudio = false
				break
			}
		}
		if allAudio && g.GroupKind != core.GroupKindExtras {
			t.Errorf("GBA audio-only group %q: GroupKind = %q, want extras",
				g.Name, g.GroupKind)
		}
		if !allAudio && g.GroupKind != core.GroupKindSelfContained {
			t.Errorf("GBA game group %q: GroupKind = %q, want self_contained",
				g.Name, g.GroupKind)
		}
	}
}

func TestClassify_Synthetic(t *testing.T) {
	tests := []struct {
		name      string
		platform  core.Platform
		files     []AnnotatedFile
		groupKind core.GroupKind
	}{
		{
			name:     "emulated single rom",
			platform: core.PlatformArcade,
			files: []AnnotatedFile{
				{Kind: FileKindArchive},
			},
			groupKind: core.GroupKindSelfContained,
		},
		{
			name:     "emulated disc image",
			platform: core.PlatformPS1,
			files: []AnnotatedFile{
				{Kind: FileKindDiscMeta},
				{Kind: FileKindUnknown}, // .bin
			},
			groupKind: core.GroupKindSelfContained,
		},
		{
			name:     "emulated media only",
			platform: core.PlatformGBA,
			files: []AnnotatedFile{
				{Kind: FileKindAudio},
				{Kind: FileKindImage},
			},
			groupKind: core.GroupKindExtras,
		},
		{
			name:     "windows extracted game",
			platform: core.PlatformWindowsPC,
			files: []AnnotatedFile{
				{Kind: FileKindExecutable},
				{Kind: FileKindImage},
				{Kind: FileKindAudio},
				{Kind: FileKindDocument},
			},
			groupKind: core.GroupKindSelfContained,
		},
		{
			name:     "windows installer exe+bin",
			platform: core.PlatformWindowsPC,
			files: []AnnotatedFile{
				{Kind: FileKindExecutable},
				{Kind: FileKindUnknown}, // .bin
			},
			groupKind: core.GroupKindPacked,
		},
		{
			name:     "windows single exe",
			platform: core.PlatformWindowsPC,
			files: []AnnotatedFile{
				{Kind: FileKindExecutable},
			},
			groupKind: core.GroupKindPacked,
		},
		{
			name:     "unknown platform archive",
			platform: core.PlatformUnknown,
			files: []AnnotatedFile{
				{Kind: FileKindArchive},
			},
			groupKind: core.GroupKindPacked,
		},
		{
			name:     "unknown platform media only",
			platform: core.PlatformUnknown,
			files: []AnnotatedFile{
				{Kind: FileKindAudio},
				{Kind: FileKindDocument},
			},
			groupKind: core.GroupKindExtras,
		},
		{
			name:     "unknown platform mixed",
			platform: core.PlatformUnknown,
			files: []AnnotatedFile{
				{Kind: FileKindUnknown},
				{Kind: FileKindDocument},
			},
			groupKind: core.GroupKindUnknown,
		},
	}

	classifier := NewGroupClassifier()
	for _, tt := range tests {
		g := GameGroup{
			Name:     tt.name,
			Platform: tt.platform,
			Files:    tt.files,
		}
		classifier.ClassifyAll([]GameGroup{g})
		// ClassifyAll modifies in-place, but we passed a copy; re-classify
		g.GroupKind = "" // reset
		groups := []GameGroup{g}
		classifier.ClassifyAll(groups)
		if groups[0].GroupKind != tt.groupKind {
			t.Errorf("%s: GroupKind = %q, want %q",
				tt.name, groups[0].GroupKind, tt.groupKind)
		}
	}
}
