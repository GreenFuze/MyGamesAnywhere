package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
)

type fakeSMBDeleteShare struct {
	failures map[string]error
	removed  []string
}

func (s *fakeSMBDeleteShare) Remove(name string) error {
	s.removed = append(s.removed, name)
	if err := s.failures[name]; err != nil {
		return err
	}
	return nil
}

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

func TestDecodeSMBConfigReadsNestedExcludePaths(t *testing.T) {
	config, err := decodeSMBConfig(mustJSON(t, map[string]any{
		"host":  "TV2",
		"share": "Games",
		"include_paths": []map[string]any{{
			"path":          `Games\Arcade`,
			"recursive":     true,
			"exclude_paths": []string{`Games\Arcade\mga_sync`},
		}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	includes := normalizedIncludePaths(config)
	if len(includes) != 1 {
		t.Fatalf("include count = %d, want 1", len(includes))
	}
	if got := includes[0].ExcludePaths; len(got) != 1 || got[0] != "Games/Arcade/mga_sync" {
		t.Fatalf("exclude paths = %#v", got)
	}
}

func TestDecodeSMBConfigRejectsExcludeOutsideInclude(t *testing.T) {
	_, err := decodeSMBConfig(mustJSON(t, map[string]any{
		"host":  "TV2",
		"share": "Games",
		"include_paths": []map[string]any{{
			"path":          "Games",
			"recursive":     true,
			"exclude_paths": []string{"Other/mga_sync"},
		}},
	}))
	if err == nil {
		t.Fatal("expected invalid exclude to fail")
	}
}

func TestSMBPathExcludedMatchesDescendantsOnly(t *testing.T) {
	excludes := []string{"Games/Arcade/Skip"}
	if !smbPathExcluded("Games/Arcade/Skip", excludes) {
		t.Fatal("expected exact excluded path to match")
	}
	if !smbPathExcluded("Games/Arcade/Skip/Nested/Game.zip", excludes) {
		t.Fatal("expected descendant path to match")
	}
	if smbPathExcluded("Games/Arcade/SkipButDifferent/Game.zip", excludes) {
		t.Fatal("did not expect sibling prefix to match")
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

func TestHandleSourceDeleteDryRunAcceptsDirectoryEntry(t *testing.T) {
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
			"path":   "Games/Platforms/SNES",
			"is_dir": true,
		}},
	}))
	if errObj != nil {
		t.Fatalf("handleSourceDelete dry run error = %s: %s", errObj.Code, errObj.Message)
	}
	encoded, _ := json.Marshal(result)
	var resp struct {
		Items []struct {
			Path   string `json:"path"`
			IsDir  bool   `json:"is_dir"`
			Action string `json:"action"`
		} `json:"items"`
	}
	if err := json.Unmarshal(encoded, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Path != "Games/Platforms/SNES" || !resp.Items[0].IsDir || resp.Items[0].Action != "delete" {
		t.Fatalf("items = %+v, want directory delete item", resp.Items)
	}
}

func TestExecuteSourceDeletePlanWarnsWhenRootDirectoryCannotBeRemovedAfterFiles(t *testing.T) {
	share := &fakeSMBDeleteShare{failures: map[string]error{
		"Games/ScummVM/Gobliins 2": errors.New("response error: A file cannot be opened because the share access flags are incompatible"),
	}}
	items := []sourceDeletePlanItem{
		{Path: "Games/ScummVM/Gobliins 2", IsDir: true, Action: "delete"},
		{Path: "Games/ScummVM/Gobliins 2/INTRO.STK", Size: 1024, Action: "delete"},
		{Path: "Games/ScummVM/Gobliins 2/GOB2.EXE", Size: 2048, Action: "delete"},
	}

	deletedCount, warnings, errObj := executeSourceDeletePlan(share, items)
	if errObj != nil {
		t.Fatalf("executeSourceDeletePlan error = %s: %s", errObj.Code, errObj.Message)
	}
	if deletedCount != 2 {
		t.Fatalf("deletedCount = %d, want 2 files deleted", deletedCount)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "could not be removed") {
		t.Fatalf("warnings = %#v, want directory cleanup warning", warnings)
	}
	wantOrder := []string{
		"Games/ScummVM/Gobliins 2/INTRO.STK",
		"Games/ScummVM/Gobliins 2/GOB2.EXE",
		"Games/ScummVM/Gobliins 2",
	}
	if !reflect.DeepEqual(share.removed, wantOrder) {
		t.Fatalf("remove order = %#v, want %#v", share.removed, wantOrder)
	}
}

func TestExecuteSourceDeletePlanFailsWhenFileCannotBeRemoved(t *testing.T) {
	share := &fakeSMBDeleteShare{failures: map[string]error{
		"Games/ScummVM/Gobliins 2/GOB2.EXE": errors.New("locked"),
	}}
	items := []sourceDeletePlanItem{
		{Path: "Games/ScummVM/Gobliins 2/GOB2.EXE", Size: 2048, Action: "delete"},
		{Path: "Games/ScummVM/Gobliins 2", IsDir: true, Action: "delete"},
	}

	_, warnings, errObj := executeSourceDeletePlan(share, items)
	if errObj == nil || errObj.Code != "DELETE_FAILED" {
		t.Fatalf("error = %+v, want DELETE_FAILED", errObj)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none for file delete failure", warnings)
	}
}

func TestExecuteSourceDeletePlanTreatsMissingFileAsAlreadyDeleted(t *testing.T) {
	share := &fakeSMBDeleteShare{failures: map[string]error{
		"Games/ScummVM/Gobliins 2/GOBNEW.LIC": errors.New("remove ScummVM\\Gobliins 2\\GOBNEW.LIC: file does not exist"),
	}}
	items := []sourceDeletePlanItem{
		{Path: "Games/ScummVM/Gobliins 2/GOBNEW.LIC", Size: 1024, Action: "delete"},
		{Path: "Games/ScummVM/Gobliins 2/GOB2.EXE", Size: 2048, Action: "delete"},
	}

	deletedCount, warnings, errObj := executeSourceDeletePlan(share, items)
	if errObj != nil {
		t.Fatalf("executeSourceDeletePlan error = %s: %s", errObj.Code, errObj.Message)
	}
	if deletedCount != 2 {
		t.Fatalf("deletedCount = %d, want 2 including already-missing file", deletedCount)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "already deleted") {
		t.Fatalf("warnings = %#v, want already-deleted warning", warnings)
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
