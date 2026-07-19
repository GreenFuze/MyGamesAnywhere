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
