package scan

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func loadTV2Fixture(t *testing.T) []core.FileEntry {
	t.Helper()
	data, err := os.ReadFile("testdata/tv2_games.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var raw []struct {
		Path  string `json:"path"`
		Name  string `json:"name"`
		Size  int64  `json:"size"`
		IsDir bool   `json:"is_dir"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	entries := make([]core.FileEntry, len(raw))
	for i, r := range raw {
		entries[i] = core.FileEntry{
			Path:  r.Path,
			Name:  r.Name,
			Size:  r.Size,
			IsDir: r.IsDir,
		}
	}
	return entries
}

func TestAnnotateFiles_KindCounts(t *testing.T) {
	entries := loadTV2Fixture(t)
	if len(entries) == 0 {
		t.Fatal("fixture is empty")
	}

	annotated := annotateFiles(entries)
	if len(annotated) != len(entries) {
		t.Fatalf("annotated count %d != entry count %d", len(annotated), len(entries))
	}

	kindCounts := map[FileKind]int{}
	for _, a := range annotated {
		kindCounts[a.Kind]++
	}

	t.Logf("Total entries: %d", len(annotated))
	for kind, count := range kindCounts {
		t.Logf("  %-20s %d", kind, count)
	}

	if kindCounts[FileKindExecutable] == 0 {
		t.Error("expected at least one executable")
	}
	if kindCounts[FileKindArchive] == 0 {
		t.Error("expected at least one archive")
	}
	if kindCounts[FileKindDiscImage] == 0 {
		t.Error("expected at least one disc image")
	}

	dirs := 0
	for _, a := range annotated {
		if a.FileEntry.IsDir {
			dirs++
		}
	}
	if dirs == 0 {
		t.Error("expected at least one directory entry")
	}
}

func TestAnnotateFiles_Depth(t *testing.T) {
	entries := loadTV2Fixture(t)
	annotated := annotateFiles(entries)

	maxDepth := 0
	for _, a := range annotated {
		if a.Depth > maxDepth {
			maxDepth = a.Depth
		}
	}

	if maxDepth < 2 {
		t.Errorf("expected max depth >= 2, got %d", maxDepth)
	}
	t.Logf("max depth: %d", maxDepth)
}

func TestAnnotateFiles_ExtensionLowercase(t *testing.T) {
	entries := []core.FileEntry{
		{Path: "Game/Setup.EXE", Name: "Setup.EXE", IsDir: false, Size: 1000},
		{Path: "Game/readme.TXT", Name: "readme.TXT", IsDir: false, Size: 100},
		{Path: "Game", Name: "Game", IsDir: true},
	}
	annotated := annotateFiles(entries)
	if annotated[0].Extension != ".exe" {
		t.Errorf("expected .exe, got %q", annotated[0].Extension)
	}
	if annotated[0].Kind != FileKindExecutable {
		t.Errorf("expected executable, got %s", annotated[0].Kind)
	}
	if annotated[1].Extension != ".txt" {
		t.Errorf("expected .txt, got %q", annotated[1].Extension)
	}
	if annotated[2].Extension != "" {
		t.Errorf("expected empty extension for dir, got %q", annotated[2].Extension)
	}
}
