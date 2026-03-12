package scanner

import (
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestRoleAssign_TV2(t *testing.T) {
	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)
	groups := NewFileGrouper().Group(annotated)
	NewPlatformDetector().DetectAll(groups)
	NewGroupClassifier().ClassifyAll(groups)
	NewRoleAssigner().AssignAll(groups)

	for _, g := range groups {
		rootCount := 0
		for _, f := range g.Files {
			if f.Role == "" {
				t.Errorf("[%s/%s] file %q has no role assigned", g.RootDir, g.Name, f.Name)
			}
			if f.Role == core.GameFileRoleRoot {
				rootCount++
			}
		}
		if rootCount > 1 {
			t.Errorf("[%s/%s] has %d root files, want at most 1", g.RootDir, g.Name, rootCount)
		}
	}
}

func TestRoleAssign_MameRoot(t *testing.T) {
	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)
	groups := NewFileGrouper().Group(annotated)
	NewPlatformDetector().DetectAll(groups)
	NewGroupClassifier().ClassifyAll(groups)
	NewRoleAssigner().AssignAll(groups)

	for _, g := range groups {
		if g.RootDir != "Mame" {
			continue
		}
		if len(g.Files) != 1 {
			t.Errorf("Mame/%s: expected 1 file, got %d", g.Name, len(g.Files))
			continue
		}
		if g.Files[0].Role != core.GameFileRoleRoot {
			t.Errorf("Mame/%s: expected root role, got %q", g.Name, g.Files[0].Role)
		}
	}
}

func TestRoleAssign_PS1DiscMeta(t *testing.T) {
	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)
	groups := NewFileGrouper().Group(annotated)
	NewPlatformDetector().DetectAll(groups)
	NewGroupClassifier().ClassifyAll(groups)
	NewRoleAssigner().AssignAll(groups)

	for _, g := range groups {
		if g.RootDir != "Roms/Playstation" {
			continue
		}
		for _, f := range g.Files {
			if f.Kind == FileKindDiscMeta {
				if f.Role != core.GameFileRoleRoot {
					t.Errorf("PS1 %s: .cue file %q should be root, got %q",
						g.Name, f.Name, f.Role)
				}
			}
			if f.Kind == FileKindUnknown && f.Extension == ".bin" {
				if f.Role != core.GameFileRoleRequired {
					t.Errorf("PS1 %s: .bin file %q should be required, got %q",
						g.Name, f.Name, f.Role)
				}
			}
		}
	}
}

func TestRoleAssign_GOGInstaller(t *testing.T) {
	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)
	groups := NewFileGrouper().Group(annotated)
	NewPlatformDetector().DetectAll(groups)
	NewGroupClassifier().ClassifyAll(groups)
	NewRoleAssigner().AssignAll(groups)

	for _, g := range groups {
		if g.Name != "setup_lego_batman_1.0_(18156)" {
			continue
		}
		for _, f := range g.Files {
			if f.Kind == FileKindExecutable {
				if f.Role != core.GameFileRoleRoot {
					t.Errorf("Lego Batman: exe %q should be root, got %q", f.Name, f.Role)
				}
			} else {
				if f.Role != core.GameFileRoleRequired {
					t.Errorf("Lego Batman: companion %q should be required, got %q",
						f.Name, f.Role)
				}
			}
		}
		return
	}
	t.Error("missing Lego Batman installer group")
}

func TestRoleAssign_ExtractedGame(t *testing.T) {
	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)
	groups := NewFileGrouper().Group(annotated)
	NewPlatformDetector().DetectAll(groups)
	NewGroupClassifier().ClassifyAll(groups)
	NewRoleAssigner().AssignAll(groups)

	for _, g := range groups {
		if g.Name != "Hangman" || g.RootDir != "Hangman" {
			continue
		}
		rootFound := false
		for _, f := range g.Files {
			if f.Kind == FileKindExecutable {
				if f.Role != core.GameFileRoleRoot {
					t.Errorf("Hangman: exe %q should be root, got %q", f.Name, f.Role)
				}
				rootFound = true
			}
			if f.Kind == FileKindImage {
				if f.Role != core.GameFileRoleRequired {
					t.Errorf("Hangman: image %q should be required (game asset), got %q",
						f.Name, f.Role)
				}
			}
		}
		if !rootFound {
			t.Error("Hangman: no root file found")
		}
		return
	}
	t.Error("missing Hangman group")
}

func TestRoleAssign_Synthetic(t *testing.T) {
	tests := []struct {
		name      string
		groupKind core.GroupKind
		files     []AnnotatedFile
		wantRoots int
		rootKind  FileKind
	}{
		{
			name:      "disc meta is root over disc image",
			groupKind: core.GroupKindSelfContained,
			files: []AnnotatedFile{
				{Kind: FileKindDiscMeta, FileEntry: core.FileEntry{Name: "game.cue", Size: 100}},
				{Kind: FileKindUnknown, FileEntry: core.FileEntry{Name: "game.bin", Size: 500000}},
			},
			wantRoots: 1,
			rootKind:  FileKindDiscMeta,
		},
		{
			name:      "iso is root when no cue",
			groupKind: core.GroupKindSelfContained,
			files: []AnnotatedFile{
				{Kind: FileKindDiscImage, FileEntry: core.FileEntry{Name: "game.iso", Size: 700000}},
				{Kind: FileKindDocument, FileEntry: core.FileEntry{Name: "readme.txt", Size: 100}},
			},
			wantRoots: 1,
			rootKind:  FileKindDiscImage,
		},
		{
			name:      "no root candidates - all data files",
			groupKind: core.GroupKindSelfContained,
			files: []AnnotatedFile{
				{Kind: FileKindUnknown, FileEntry: core.FileEntry{Name: "data.res", Size: 1000}},
				{Kind: FileKindUnknown, FileEntry: core.FileEntry{Name: "sound.vga", Size: 2000}},
			},
			wantRoots: 0,
		},
		{
			name:      "extras group - all optional",
			groupKind: core.GroupKindExtras,
			files: []AnnotatedFile{
				{Kind: FileKindDocument, FileEntry: core.FileEntry{Name: "manual.pdf", Size: 500}},
			},
			wantRoots: 0,
		},
	}

	assigner := NewRoleAssigner()
	for _, tt := range tests {
		groups := []GameGroup{{
			Name:      tt.name,
			GroupKind: tt.groupKind,
			Files:     tt.files,
		}}
		assigner.AssignAll(groups)

		rootCount := 0
		for _, f := range groups[0].Files {
			if f.Role == core.GameFileRoleRoot {
				rootCount++
				if tt.rootKind != "" && f.Kind != tt.rootKind {
					t.Errorf("%s: root is %s, want %s", tt.name, f.Kind, tt.rootKind)
				}
			}
		}
		if rootCount != tt.wantRoots {
			t.Errorf("%s: %d roots, want %d", tt.name, rootCount, tt.wantRoots)
		}
	}
}
