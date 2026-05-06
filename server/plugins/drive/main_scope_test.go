package main

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/oauth2"
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

func TestFilesystemIncludePathsFromConfigReadsNestedExcludePaths(t *testing.T) {
	includes := filesystemIncludePathsFromConfig(map[string]any{
		"include_paths": []any{
			map[string]any{
				"path":          "Games/Arcade",
				"recursive":     true,
				"exclude_paths": []any{`Games\Arcade\Bad`, "", "Games/Arcade/Skip"},
			},
		},
	})
	if len(includes) != 1 {
		t.Fatalf("include count = %d, want 1", len(includes))
	}
	excludes := includes[0].ExcludePaths
	if len(excludes) != 2 {
		t.Fatalf("exclude count = %d, want 2", len(excludes))
	}
	if excludes[0] != "Games/Arcade/Bad" || excludes[1] != "Games/Arcade/Skip" {
		t.Fatalf("excludes = %#v", excludes)
	}
}

func TestDrivePathExcludedMatchesDescendantsOnly(t *testing.T) {
	excludes := []string{"Games/Arcade/Skip"}
	if !drivePathExcluded("Games/Arcade/Skip", excludes) {
		t.Fatal("expected exact excluded path to match")
	}
	if !drivePathExcluded("Games/Arcade/Skip/Nested/Game.zip", excludes) {
		t.Fatal("expected descendant path to match")
	}
	if drivePathExcluded("Games/Arcade/SkipButDifferent/Game.zip", excludes) {
		t.Fatal("did not expect sibling prefix to match")
	}
}

func TestDrivePathNotFoundSentinelSurvivesWrapping(t *testing.T) {
	err := fmt.Errorf("resolve save sync path: %w", errDrivePathNotFound)
	if !errors.Is(err, errDrivePathNotFound) {
		t.Fatalf("expected wrapped save-sync lookup error to classify as not found")
	}
}

func TestDriveTokenConfigRoundTrip(t *testing.T) {
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	updates := tokenConfigUpdates(&oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Expiry:       expiry,
	})
	tok := tokenFromConfig(updates)
	if tok == nil {
		t.Fatal("tokenFromConfig returned nil")
	}
	if tok.AccessToken != "access" || tok.RefreshToken != "refresh" || tok.TokenType != "Bearer" || !tok.Expiry.Equal(expiry) {
		t.Fatalf("token = %+v, want round-tripped token expiring %s", tok, expiry)
	}
}
