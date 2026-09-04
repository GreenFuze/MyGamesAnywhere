package sourcescope

import "testing"

func TestReadIncludePathsReadsNestedExcludePaths(t *testing.T) {
	includes := ReadIncludePaths("game-source-google-drive", map[string]any{
		"include_paths": []any{
			map[string]any{
				"path":          `Games\Arcade`,
				"recursive":     true,
				"exclude_paths": []any{`Games\Arcade\mga_sync`, "", `Games/Arcade/mga_sync`},
			},
		},
	})
	if len(includes) != 1 {
		t.Fatalf("include count = %d, want 1", len(includes))
	}
	if got := includes[0].ExcludePaths; len(got) != 1 || got[0] != "Games/Arcade/mga_sync" {
		t.Fatalf("exclude paths = %#v", got)
	}
}

func TestReadIncludePathsAssignsLegacyExcludesToOwningInclude(t *testing.T) {
	includes := ReadIncludePaths("game-source-google-drive", map[string]any{
		"include_paths": []any{
			map[string]any{"path": "Games", "recursive": true},
			map[string]any{"path": "Games/Arcade", "recursive": true},
		},
		"exclude_paths": []any{"Games/Arcade/mga_sync"},
	})
	if len(includes[0].ExcludePaths) != 0 {
		t.Fatalf("root include excludes = %#v, want none", includes[0].ExcludePaths)
	}
	if got := includes[1].ExcludePaths; len(got) != 1 || got[0] != "Games/Arcade/mga_sync" {
		t.Fatalf("nested include excludes = %#v", got)
	}
}

func TestValidateConfigRejectsExcludeOutsideInclude(t *testing.T) {
	err := ValidateConfig("game-source-smb", map[string]any{
		"include_paths": []any{
			map[string]any{
				"path":          "Games",
				"recursive":     true,
				"exclude_paths": []any{"Other/mga_sync"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid nested exclude to fail")
	}
}

func TestValidateConfigRejectsLegacyExcludeOutsideAllIncludes(t *testing.T) {
	err := ValidateConfig("game-source-google-drive", map[string]any{
		"include_paths": []any{map[string]any{"path": "Games", "recursive": true}},
		"exclude_paths": []any{"Other/mga_sync"},
	})
	if err == nil {
		t.Fatal("expected invalid legacy exclude to fail")
	}
}

func TestGoogleDriveScopePreservesStableObjectID(t *testing.T) {
	config := NormalizeConfig("game-source-google-drive", map[string]any{
		"include_paths": []any{map[string]any{
			"path":      "Shared with me/Arcade",
			"recursive": true,
			"object_id": " shared-folder-id ",
		}},
	})
	includes := ReadIncludePaths("game-source-google-drive", config)
	if len(includes) != 1 || includes[0].ObjectID != "shared-folder-id" {
		t.Fatalf("includes = %#v, want preserved stable object id", includes)
	}
	serialized, ok := config["include_paths"].([]map[string]any)
	if !ok || serialized[0]["object_id"] != "shared-folder-id" {
		t.Fatalf("normalized config = %#v, want persisted stable object id", config)
	}
}

func TestSMBScopeDropsGoogleObjectID(t *testing.T) {
	config := NormalizeConfig("game-source-smb", map[string]any{
		"include_paths": []any{map[string]any{
			"path":      "Games",
			"recursive": true,
			"object_id": "must-not-cross-provider-boundary",
		}},
	})
	includes := ReadIncludePaths("game-source-smb", config)
	if len(includes) != 1 || includes[0].ObjectID != "" {
		t.Fatalf("includes = %#v, want no Google object id in SMB scope", includes)
	}
	serialized, ok := config["include_paths"].([]map[string]any)
	if !ok {
		t.Fatalf("normalized config = %#v", config)
	}
	if _, exists := serialized[0]["object_id"]; exists {
		t.Fatalf("normalized SMB config retained object_id: %#v", serialized[0])
	}
}

func TestLocalSourceIsFilesystemBacked(t *testing.T) {
	// This one entry switches on include-path normalization, scan-scope
	// reconciliation, duplicate detection, file validation and destructive
	// delete. Without it every one of them silently no-ops for local folders.
	if !IsFilesystemBackedPlugin("game-source-local") {
		t.Fatal("game-source-local must be filesystem-backed")
	}
}

func TestLocalScopeHasNoLegacyPathKey(t *testing.T) {
	// Deliberately empty. A local connection's base is base_path, and wiring it
	// in here would feed an absolute path through NormalizeLogicalPath, which
	// strips a leading slash: "/mnt/games" would silently become the relative
	// "mnt/games" while "C:/Games" survived. Windows would pass and Linux would
	// corrupt.
	if key := legacyPathKey("game-source-local"); key != "" {
		t.Fatalf("legacyPathKey(local) = %q, want empty", key)
	}
}

func TestLocalScopeKeepsBasePathAndStillDropsLegacyKeys(t *testing.T) {
	config := NormalizeConfig("game-source-local", map[string]any{
		"base_path":     `D:\Games`,
		"root_path":     "should-not-survive",
		"path":          "should-not-survive",
		"exclude_paths": []any{},
		"include_paths": []any{map[string]any{"path": "SNES", "recursive": true}},
	})

	// base_path has to survive normalization: the server normalizes before
	// validating against the manifest schema, so a stripped required field
	// would fail every create and update.
	if config["base_path"] != `D:\Games` {
		t.Fatalf("base_path did not survive normalization: %#v", config)
	}
	for _, legacyKey := range []string{"root_path", "path", "exclude_paths"} {
		if _, exists := config[legacyKey]; exists {
			t.Fatalf("normalized config retained legacy key %q: %#v", legacyKey, config)
		}
	}
	includes := ReadIncludePaths("game-source-local", config)
	if len(includes) != 1 || includes[0].Path != "SNES" || !includes[0].Recursive {
		t.Fatalf("includes = %#v", includes)
	}
}

func TestLocalScopeDropsGoogleObjectID(t *testing.T) {
	config := NormalizeConfig("game-source-local", map[string]any{
		"include_paths": []any{map[string]any{
			"path":      "Games",
			"recursive": true,
			"object_id": "must-not-cross-provider-boundary",
		}},
	})
	includes := ReadIncludePaths("game-source-local", config)
	if len(includes) != 1 || includes[0].ObjectID != "" {
		t.Fatalf("includes = %#v, want no Google object id in local scope", includes)
	}
}
