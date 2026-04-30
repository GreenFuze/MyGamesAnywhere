package main

import (
	"errors"
	"fmt"
	"testing"
)

func TestFilesystemIncludePathsFromConfigSupportsLegacyRootPath(t *testing.T) {
	includes := filesystemIncludePathsFromConfig(map[string]any{
		"root_path": "Games/Arcade",
	})
	if len(includes) != 1 {
		t.Fatalf("include count = %d, want 1", len(includes))
	}
	if includes[0].Path != "Games/Arcade" {
		t.Fatalf("path = %q, want Games/Arcade", includes[0].Path)
	}
	if !includes[0].Recursive {
		t.Fatal("legacy root path should default to recursive")
	}
}

func TestFilesystemIncludePathsFromConfigReadsNormalizedIncludePaths(t *testing.T) {
	includes := filesystemIncludePathsFromConfig(map[string]any{
		"include_paths": []any{
			map[string]any{"path": `Games\Arcade`, "recursive": false},
		},
	})
	if len(includes) != 1 {
		t.Fatalf("include count = %d, want 1", len(includes))
	}
	if includes[0].Path != "Games/Arcade" {
		t.Fatalf("path = %q, want Games/Arcade", includes[0].Path)
	}
	if includes[0].Recursive {
		t.Fatal("recursive = true, want false")
	}
}

func TestDrivePathNotFoundSentinelSurvivesWrapping(t *testing.T) {
	err := fmt.Errorf("resolve save sync path: %w", errDrivePathNotFound)
	if !errors.Is(err, errDrivePathNotFound) {
		t.Fatalf("expected wrapped save-sync lookup error to classify as not found")
	}
}
