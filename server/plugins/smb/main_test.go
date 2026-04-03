package main

import (
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
)

func TestNormalizedIncludePathsFallsBackToLegacyPath(t *testing.T) {
	includes := normalizedIncludePaths(SMBConfig{Path: `Games\Arcade`})
	if len(includes) != 1 {
		t.Fatalf("include count = %d, want 1", len(includes))
	}
	if includes[0].Path != "Games/Arcade" {
		t.Fatalf("path = %q, want Games/Arcade", includes[0].Path)
	}
	if !includes[0].Recursive {
		t.Fatal("legacy path should default to recursive")
	}
}

func TestSourceIdentityIgnoresIncludePaths(t *testing.T) {
	config := SMBConfig{
		Host:  "TV2",
		Share: "Games",
		IncludePaths: []sourcescope.IncludePath{
			{Path: "Arcade", Recursive: true},
		},
	}

	if got := sourceIdentity(config); got != "smb://tv2/games" {
		t.Fatalf("source identity = %q, want smb://tv2/games", got)
	}
}
