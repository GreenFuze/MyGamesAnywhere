package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
)

type fakeSMBDeleteShare struct {
	failures       map[string]error
	lstatFailures  map[string]error
	readFailures   map[string]error
	directories    map[string][]os.FileInfo
	symlinks       map[string]bool
	removed        []string
	directoryReads []string
}

func (s *fakeSMBDeleteShare) Lstat(name string) (os.FileInfo, error) {
	if err := s.lstatFailures[name]; err != nil {
		return nil, err
	}
	if s.symlinks[name] {
		return fakeSMBFileInfo{name: name, mode: os.ModeSymlink}, nil
	}
	if _, isDir := s.directories[name]; isDir {
		return fakeSMBFileInfo{name: name, mode: os.ModeDir}, nil
	}
	return fakeSMBFileInfo{name: name}, nil
}

func (s *fakeSMBDeleteShare) ReadDir(dirname string) ([]os.FileInfo, error) {
	s.directoryReads = append(s.directoryReads, dirname)
	if err := s.readFailures[dirname]; err != nil {
		return nil, err
	}
	return s.directories[dirname], nil
}

func (s *fakeSMBDeleteShare) Remove(name string) error {
	s.removed = append(s.removed, name)
	if err := s.failures[name]; err != nil {
		return err
	}
	return nil
}

type fakeSMBFileInfo struct {
	name string
	mode fs.FileMode
}

func (i fakeSMBFileInfo) Name() string       { return i.name }
func (i fakeSMBFileInfo) Size() int64        { return 0 }
func (i fakeSMBFileInfo) Mode() fs.FileMode  { return i.mode }
func (i fakeSMBFileInfo) ModTime() time.Time { return time.Time{} }
func (i fakeSMBFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fakeSMBFileInfo) Sys() any           { return nil }

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
	if len(resp.Items) != 1 || resp.Items[0].Path != "Games/Platforms/SNES" || !resp.Items[0].IsDir || resp.Items[0].Action != "prune_empty" {
		t.Fatalf("items = %+v, want empty-directory prune item", resp.Items)
	}
}

func TestExecuteSourceDeletePlanKeepsNonEmptyRootAfterFiles(t *testing.T) {
	root := "Games/ScummVM/Gobliins 2"
	share := &fakeSMBDeleteShare{directories: map[string][]os.FileInfo{
		root: {fakeSMBFileInfo{name: "user-notes.txt"}},
	}}
	items := []sourceDeletePlanItem{
		{Path: root, IsDir: true, Action: "prune_empty"},
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
	if len(warnings) != 1 || !strings.Contains(warnings[0], "contains files MGA did not authorize") {
		t.Fatalf("warnings = %#v, want non-empty directory warning", warnings)
	}
	wantOrder := []string{
		"Games/ScummVM/Gobliins 2/INTRO.STK",
		"Games/ScummVM/Gobliins 2/GOB2.EXE",
	}
	if !reflect.DeepEqual(share.removed, wantOrder) {
		t.Fatalf("remove order = %#v, want %#v", share.removed, wantOrder)
	}
}

func TestExecuteSourceDeletePlanFailsWhenFileCannotBeRemoved(t *testing.T) {
	share := &fakeSMBDeleteShare{failures: map[string]error{
		"Games/ScummVM/Gobliins 2/GOB2.EXE": errors.New("locked"),
	}, directories: map[string][]os.FileInfo{"Games/ScummVM/Gobliins 2": {}}}
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
	share := &fakeSMBDeleteShare{lstatFailures: map[string]error{
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

func TestExecuteSourceDeletePlanPrunesEmptyDirectoriesLeavesFirst(t *testing.T) {
	root := "Games/Arcade/Alpha"
	nested := root + "/data"
	share := &fakeSMBDeleteShare{directories: map[string][]os.FileInfo{
		root:   {},
		nested: {},
	}}
	items := []sourceDeletePlanItem{
		{Path: root, IsDir: true, Action: "prune_empty"},
		{Path: nested, IsDir: true, Action: "prune_empty"},
		{Path: nested + "/game.bin", Action: "delete"},
	}

	deletedCount, warnings, errObj := executeSourceDeletePlan(share, items)
	if errObj != nil {
		t.Fatalf("executeSourceDeletePlan error = %s: %s", errObj.Code, errObj.Message)
	}
	if deletedCount != 3 || len(warnings) != 0 {
		t.Fatalf("deletedCount = %d warnings = %#v, want three clean removals", deletedCount, warnings)
	}
	wantOrder := []string{nested + "/game.bin", nested, root}
	if !reflect.DeepEqual(share.removed, wantOrder) {
		t.Fatalf("remove order = %#v, want %#v", share.removed, wantOrder)
	}
}

func TestExecuteSourceDeletePlanRejectsLinkBeforeDeletingAnything(t *testing.T) {
	root := "Games/Arcade/Alpha"
	share := &fakeSMBDeleteShare{
		directories: map[string][]os.FileInfo{root: {}},
		symlinks:    map[string]bool{root + "/game.bin": true},
	}
	items := []sourceDeletePlanItem{
		{Path: root + "/game.bin", Action: "delete"},
		{Path: root, IsDir: true, Action: "prune_empty"},
	}

	_, _, errObj := executeSourceDeletePlan(share, items)
	if errObj == nil || errObj.Code != "NOT_ALLOWED" {
		t.Fatalf("error = %+v, want NOT_ALLOWED", errObj)
	}
	if len(share.removed) != 0 {
		t.Fatalf("removed = %#v, want no mutation after unsafe-link preflight", share.removed)
	}
}

func TestExecuteSourceDeletePlanFailsClosedWhenDirectoryCannotBeChecked(t *testing.T) {
	root := "Games/Arcade/Alpha"
	share := &fakeSMBDeleteShare{
		directories:  map[string][]os.FileInfo{root: {}},
		readFailures: map[string]error{root: errors.New("access denied")},
	}
	items := []sourceDeletePlanItem{
		{Path: root + "/game.bin", Action: "delete"},
		{Path: root, IsDir: true, Action: "prune_empty"},
	}

	deletedCount, _, errObj := executeSourceDeletePlan(share, items)
	if errObj == nil || errObj.Code != "DELETE_FAILED" {
		t.Fatalf("error = %+v, want DELETE_FAILED", errObj)
	}
	if deletedCount != 1 || !reflect.DeepEqual(share.removed, []string{root + "/game.bin"}) {
		t.Fatalf("deletedCount = %d removed = %#v, want only authorized file removed", deletedCount, share.removed)
	}
}

func TestTransferManifestHashIsStableAndBindsContent(t *testing.T) {
	first := []transferFile{
		{RelativePath: "disc/game.bin", Size: 7, SHA256: strings.Repeat("a", 64)},
		{RelativePath: "readme.txt", Size: 3, SHA256: strings.Repeat("b", 64)},
	}
	reordered := []transferFile{first[1], first[0]}
	if transferManifestHash(first) != transferManifestHash(reordered) {
		t.Fatal("manifest hash changed when file order changed")
	}
	changed := append([]transferFile(nil), first...)
	changed[0].SHA256 = strings.Repeat("c", 64)
	if transferManifestHash(first) == transferManifestHash(changed) {
		t.Fatal("manifest hash did not bind the file checksum")
	}
}

func TestTransferPathsStayInsideOwnedStage(t *testing.T) {
	if got := transferStagePath("Games/Game", "transfer-1"); got != "Games/.mga-transfer-transfer-1" {
		t.Fatalf("stage path = %q", got)
	}
	for _, value := range []string{"../outside", ".mga/transfer.json", "", "."} {
		if _, err := safeTransferRelativePath(value); err == nil {
			t.Fatalf("safeTransferRelativePath(%q) succeeded", value)
		}
	}
	if got, err := safeTransferRelativePath("disc/game.bin"); err != nil || got != "disc/game.bin" {
		t.Fatalf("safe relative path = %q, %v", got, err)
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
