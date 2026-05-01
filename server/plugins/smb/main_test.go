package main

import (
	"encoding/json"
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

func TestSourceDeletePathWithinRoot(t *testing.T) {
	tests := []struct {
		name     string
		rootPath string
		filePath string
		want     bool
	}{
		{name: "child file", rootPath: `Games\Platforms\SNES`, filePath: "Games/Platforms/SNES/Game.sfc", want: true},
		{name: "same file root", rootPath: "Games/Platforms/SNES/Game.sfc", filePath: "Games/Platforms/SNES/Game.sfc", want: true},
		{name: "sibling prefix rejected", rootPath: "Games/Platforms/SNES", filePath: "Games/Platforms/SNES Extras/Game.sfc", want: false},
		{name: "outside root rejected", rootPath: "Games/Platforms/SNES", filePath: "Games/Platforms/N64/Game.z64", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceDeletePathWithinRoot(tt.rootPath, tt.filePath); got != tt.want {
				t.Fatalf("sourceDeletePathWithinRoot(%q, %q) = %t, want %t", tt.rootPath, tt.filePath, got, tt.want)
			}
		})
	}
}

func TestHandleSourceDeleteDryRunReturnsDeletePlan(t *testing.T) {
	result, errObj := handleSourceDelete(mustJSON(t, map[string]any{
		"dry_run":        true,
		"source_game_id": "scan:smb-game",
		"root_path":      "Games/Platforms/SNES",
		"config": map[string]any{
			"host":  "nas",
			"share": "games",
			"include_paths": []map[string]any{{
				"path":      "Games",
				"recursive": true,
			}},
		},
		"files": []map[string]any{{
			"path": "Games/Platforms/SNES/Game.sfc",
			"size": 1024,
		}},
	}))
	if errObj != nil {
		t.Fatalf("handleSourceDelete dry run error = %s: %s", errObj.Code, errObj.Message)
	}
	encoded, _ := json.Marshal(result)
	var resp struct {
		SourceGameID string `json:"source_game_id"`
		PluginID     string `json:"plugin_id"`
		Action       string `json:"action"`
		Items        []struct {
			Path   string `json:"path"`
			Action string `json:"action"`
		} `json:"items"`
		DeletedCount int `json:"deleted_count"`
	}
	if err := json.Unmarshal(encoded, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.SourceGameID != "scan:smb-game" || resp.PluginID != "game-source-smb" || resp.Action != "delete" {
		t.Fatalf("response = %+v, want smb delete plan metadata", resp)
	}
	if len(resp.Items) != 1 || resp.Items[0].Path != "Games/Platforms/SNES/Game.sfc" || resp.Items[0].Action != "delete" {
		t.Fatalf("items = %+v, want exact delete item", resp.Items)
	}
	if resp.DeletedCount != 0 {
		t.Fatalf("deleted_count = %d, want 0 for dry run", resp.DeletedCount)
	}
}

func TestHandleSourceDeleteRejectsDirectoryEntry(t *testing.T) {
	_, errObj := handleSourceDelete(mustJSON(t, map[string]any{
		"dry_run":        true,
		"source_game_id": "scan:smb-game",
		"root_path":      "Games/Platforms/SNES",
		"config": map[string]any{
			"host":  "nas",
			"share": "games",
			"include_paths": []map[string]any{{
				"path":      "Games",
				"recursive": true,
			}},
		},
		"files": []map[string]any{{
			"path":   "Games/Platforms/SNES",
			"is_dir": true,
		}},
	}))
	if errObj == nil {
		t.Fatal("expected directory delete to be rejected")
	}
	if errObj.Code != "INVALID_PARAMS" {
		t.Fatalf("error code = %q, want INVALID_PARAMS", errObj.Code)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
