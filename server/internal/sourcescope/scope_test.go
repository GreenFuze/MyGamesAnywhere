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
