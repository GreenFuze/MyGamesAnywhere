package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
)

// mustConfigJSON builds request payloads through the JSON encoder rather than
// string literals, because Windows paths carry backslashes that a literal would
// silently mangle.
func mustConfigJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return encoded
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent of %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

// --------------- Config ---------------

func TestDecodeLocalConfigAcceptsBareAndNestedShapes(t *testing.T) {
	// The server sends the bare config map when scanning and nests it under
	// "config" when checking or deleting. Both must decode identically.
	body := map[string]any{
		"base_path":     `D:\Games`,
		"include_paths": []any{map[string]any{"path": "SNES", "recursive": true}},
	}

	bare, err := decodeLocalConfig(mustConfigJSON(t, body))
	if err != nil {
		t.Fatalf("decode bare config: %v", err)
	}
	nested, err := decodeLocalConfig(mustConfigJSON(t, map[string]any{"config": body}))
	if err != nil {
		t.Fatalf("decode nested config: %v", err)
	}

	if bare.BasePath != `D:\Games` || nested.BasePath != `D:\Games` {
		t.Fatalf("base_path lost: bare=%q nested=%q", bare.BasePath, nested.BasePath)
	}
	if len(bare.IncludePaths) != 1 || bare.IncludePaths[0].Path != "SNES" {
		t.Fatalf("unexpected include paths: %+v", bare.IncludePaths)
	}
}

func TestDecodeLocalConfigRejectsExcludeOutsideInclude(t *testing.T) {
	_, err := decodeLocalConfig(mustConfigJSON(t, map[string]any{
		"base_path": `D:\Games`,
		"include_paths": []any{map[string]any{
			"path":          "SNES",
			"recursive":     true,
			"exclude_paths": []any{"Genesis/Extras"},
		}},
	}))
	if err == nil {
		t.Fatal("expected an exclude outside its include to be rejected")
	}
}

func TestNormalizedIncludePathsDefaultsToWholeBase(t *testing.T) {
	includes := normalizedIncludePaths(LocalConfig{BasePath: `D:\Games`})
	if len(includes) != 1 || includes[0].Path != "" || !includes[0].Recursive {
		t.Fatalf("expected one recursive whole-base include, got %+v", includes)
	}
}

// --------------- Base resolution ---------------

func TestResolveBaseRejectsRelativeAndEmptyPaths(t *testing.T) {
	for _, basePath := range []string{"", "   ", "games", filepath.Join("relative", "games")} {
		if _, err := resolveBase(LocalConfig{BasePath: basePath}); err == nil {
			t.Fatalf("expected %q to be rejected as a base path", basePath)
		}
	}
}

func TestResolveBaseRejectsMissingDirectoryAndFile(t *testing.T) {
	root := t.TempDir()

	if _, err := resolveBase(LocalConfig{BasePath: filepath.Join(root, "absent")}); err == nil {
		t.Fatal("expected a missing base path to be rejected")
	}

	filePath := filepath.Join(root, "game.iso")
	writeFile(t, filePath, "bytes")
	if _, err := resolveBase(LocalConfig{BasePath: filePath}); err == nil {
		t.Fatal("expected a file to be rejected as a base path")
	}
}

func TestResolveBaseAcceptsARealDirectory(t *testing.T) {
	root := t.TempDir()
	resolved, err := resolveBase(LocalConfig{BasePath: root})
	if err != nil {
		t.Fatalf("resolve base: %v", err)
	}
	if !filepath.IsAbs(resolved) {
		t.Fatalf("expected an absolute resolved base, got %q", resolved)
	}
}

// --------------- Containment ---------------

func TestResolveWithinBaseRejectsTraversal(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "Games", "game.sfc"), "rom")

	for _, logicalPath := range []string{"../outside.txt", "..", "Games/../../outside.txt"} {
		if _, err := resolveWithinBase(base, logicalPath, false); err == nil {
			t.Fatalf("expected %q to be refused", logicalPath)
		}
	}
}

func TestResolveWithinBaseAllowsContainedPaths(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "Games", "game.sfc"), "rom")

	resolved, err := resolveWithinBase(base, "Games/game.sfc", true)
	if err != nil {
		t.Fatalf("resolve contained path: %v", err)
	}
	if !strings.HasSuffix(resolved, filepath.Join("Games", "game.sfc")) {
		t.Fatalf("unexpected resolved path %q", resolved)
	}
}

func TestResolveWithinBaseTreatsEmptyPathAsTheBase(t *testing.T) {
	base := t.TempDir()
	resolved, err := resolveWithinBase(base, "", true)
	if err != nil || resolved != base {
		t.Fatalf("expected the base itself, got %q (%v)", resolved, err)
	}
}

func TestResolveWithinBaseRefusesMissingPathWhenItMustExist(t *testing.T) {
	base := t.TempDir()
	if _, err := resolveWithinBase(base, "Games/absent.sfc", true); err == nil {
		t.Fatal("expected a missing path to be refused when it must exist")
	}
	// A delete plan still has to describe a file that raced away.
	if _, err := resolveWithinBase(base, "Games/absent.sfc", false); err != nil {
		t.Fatalf("expected a tolerated absence, got %v", err)
	}
}

func TestResolveWithinBaseRefusesSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "not yours")

	if err := os.Symlink(outside, filepath.Join(base, "escape")); err != nil {
		t.Skipf("symlinks unavailable on this machine: %v", err)
	}
	if _, err := resolveWithinBase(base, "escape/secret.txt", true); err == nil {
		t.Fatal("expected a symlinked escape to be refused")
	}
}

func TestContainedInRefusesADifferentVolume(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("volume names are only distinct on Windows")
	}
	if err := containedIn(`C:\Games`, `D:\Games\game.sfc`); err == nil {
		t.Fatal("expected a cross-volume path to be refused")
	}
}

// --------------- Source identity ---------------

func TestSourceIdentityIsStableAndOpaque(t *testing.T) {
	base := t.TempDir()
	identity := sourceIdentity(base)

	if identity != sourceIdentity(base) {
		t.Fatal("expected a stable identity for the same folder")
	}
	if !strings.HasPrefix(identity, "localdir:") {
		t.Fatalf("unexpected identity prefix: %q", identity)
	}
	// The server's filesystem layout must not travel in an API response.
	if strings.Contains(identity, filepath.Base(base)) {
		t.Fatalf("identity leaked the folder path: %q", identity)
	}
}

func TestSourceIdentityCaseSensitivityFollowsThePlatform(t *testing.T) {
	same := sourceIdentity(`C:\Games`) == sourceIdentity(`C:\games`)
	if runtime.GOOS == "windows" && !same {
		t.Fatal("NTFS treats these as one folder; identities should match")
	}
	if runtime.GOOS != "windows" && same {
		t.Fatal("POSIX treats these as two folders; identities should differ")
	}
}

// --------------- Listing ---------------

func listFor(t *testing.T, config LocalConfig) map[string]map[string]any {
	t.Helper()
	files, err := listFiles(config, nil)
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	byPath := make(map[string]map[string]any, len(files))
	for _, file := range files {
		byPath[file["path"].(string)] = file
	}
	return byPath
}

func TestListFilesReturnsBaseRelativeSlashPaths(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "Games", "Chrono", "game.sfc"), "rom")

	files := listFor(t, LocalConfig{BasePath: base})

	if _, ok := files["Games/Chrono/game.sfc"]; !ok {
		t.Fatalf("expected a base-relative slash path, got %v", keysOf(files))
	}
	if _, ok := files["Games/Chrono"]; !ok {
		t.Fatal("expected directories to be listed too")
	}
	entry := files["Games/Chrono/game.sfc"]
	if entry["is_dir"].(bool) {
		t.Fatal("a file was reported as a directory")
	}
	if entry["size"].(int64) != int64(len("rom")) {
		t.Fatalf("unexpected size: %v", entry["size"])
	}
	if _, err := time.Parse(time.RFC3339, entry["mod_time"].(string)); err != nil {
		t.Fatalf("mod_time must parse as RFC3339, got %v: %v", entry["mod_time"], err)
	}
}

func TestListFilesSkipsMGAControlDirectories(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "game.sfc"), "rom")
	writeFile(t, filepath.Join(base, ".mga", "state.json"), "{}")
	writeFile(t, filepath.Join(base, ".mga-transfer-abc", "part"), "x")

	files := listFor(t, LocalConfig{BasePath: base})

	for path := range files {
		if strings.HasPrefix(path, ".mga") {
			t.Fatalf("MGA bookkeeping leaked into the listing: %q", path)
		}
	}
}

func TestListFilesSkipsSymlinksEntirely(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "not yours")
	writeFile(t, filepath.Join(base, "game.sfc"), "rom")

	if err := os.Symlink(outside, filepath.Join(base, "escape")); err != nil {
		t.Skipf("symlinks unavailable on this machine: %v", err)
	}

	files := listFor(t, LocalConfig{BasePath: base})

	// A link must be neither emitted nor descended: emitting one would leave a
	// row that delivery resolves elsewhere and deletion refuses to touch.
	for path := range files {
		if strings.HasPrefix(path, "escape") {
			t.Fatalf("link surfaced in the listing: %q", path)
		}
	}
	if _, ok := files["game.sfc"]; !ok {
		t.Fatal("the real file should still be listed")
	}
}

func TestListFilesDoesNotHangOnASymlinkLoop(t *testing.T) {
	base := t.TempDir()
	inner := filepath.Join(base, "a")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatalf("create %q: %v", inner, err)
	}
	writeFile(t, filepath.Join(inner, "game.sfc"), "rom")
	if err := os.Symlink(inner, filepath.Join(inner, "loop")); err != nil {
		t.Skipf("symlinks unavailable on this machine: %v", err)
	}

	files := listFor(t, LocalConfig{BasePath: base})

	if _, ok := files["a/game.sfc"]; !ok {
		t.Fatal("expected the real file despite the loop")
	}
	for path := range files {
		if strings.Contains(path, "loop") {
			t.Fatalf("loop was traversed: %q", path)
		}
	}
}

func TestListFilesHonoursNonRecursiveInclude(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "Games", "top.sfc"), "rom")
	writeFile(t, filepath.Join(base, "Games", "Deep", "nested.sfc"), "rom")

	files := listFor(t, LocalConfig{
		BasePath:     base,
		IncludePaths: []sourcescope.IncludePath{{Path: "Games", Recursive: false}},
	})

	if _, ok := files["Games/top.sfc"]; !ok {
		t.Fatal("expected the top-level file")
	}
	if _, ok := files["Games/Deep/nested.sfc"]; ok {
		t.Fatal("a non-recursive include must not descend")
	}
}

func TestListFilesExcludesDescendantsOnly(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "Games", "SNES", "game.sfc"), "rom")
	writeFile(t, filepath.Join(base, "Games", "SNES Extras", "manual.pdf"), "doc")

	files := listFor(t, LocalConfig{
		BasePath: base,
		IncludePaths: []sourcescope.IncludePath{{
			Path:         "Games",
			Recursive:    true,
			ExcludePaths: []string{"Games/SNES"},
		}},
	})

	if _, ok := files["Games/SNES/game.sfc"]; ok {
		t.Fatal("excluded folder was scanned")
	}
	// A sibling that merely shares a name prefix is a different folder.
	if _, ok := files["Games/SNES Extras/manual.pdf"]; !ok {
		t.Fatal("a name-prefix sibling was wrongly excluded")
	}
}

func TestListFilesDeduplicatesOverlappingIncludes(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "Games", "Chrono", "game.sfc"), "rom")

	raw, err := listFiles(LocalConfig{
		BasePath: base,
		IncludePaths: []sourcescope.IncludePath{
			{Path: "Games", Recursive: true},
			{Path: "Games/Chrono", Recursive: true},
		},
	}, nil)
	if err != nil {
		t.Fatalf("list files: %v", err)
	}

	seen := map[string]int{}
	for _, file := range raw {
		seen[file["path"].(string)]++
	}
	if seen["Games/Chrono/game.sfc"] != 1 {
		t.Fatalf("overlapping includes reported the file %d times", seen["Games/Chrono/game.sfc"])
	}
}

func TestListFilesContinuesWhenOneIncludeIsMissing(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "Games", "game.sfc"), "rom")

	files := listFor(t, LocalConfig{
		BasePath: base,
		IncludePaths: []sourcescope.IncludePath{
			{Path: "Unplugged", Recursive: true},
			{Path: "Games", Recursive: true},
		},
	})

	// Losing one folder must not cost the operator the rest of the library.
	if _, ok := files["Games/game.sfc"]; !ok {
		t.Fatal("a missing include path aborted the whole scan")
	}
}

func TestListFilesRejectsAnUnusableBase(t *testing.T) {
	if _, err := listFiles(LocalConfig{BasePath: "relative/path"}, nil); err == nil {
		t.Fatal("expected a relative base to fail the scan")
	}
}

// --------------- Browse ---------------

func TestBrowseListsRootsWhenNothingIsChosen(t *testing.T) {
	result, errObj := handleBrowse(mustConfigJSON(t, map[string]any{"path": "", "config": map[string]any{}}))
	if errObj != nil {
		t.Fatalf("browse roots: %+v", errObj)
	}
	folders := result.(map[string]any)["folders"].([]browseFolder)
	if len(folders) == 0 {
		t.Fatal("expected at least one root")
	}
	for _, folder := range folders {
		if !isAbsoluteRequest(folder.Path) {
			t.Fatalf("root %q should be absolute", folder.Path)
		}
	}
}

func TestBrowseReturnsAbsolutePathsWhenPickingTheBase(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "Games"), 0o755); err != nil {
		t.Fatalf("create folder: %v", err)
	}

	result, errObj := handleBrowse(mustConfigJSON(t, map[string]any{
		"path":   filepath.ToSlash(base),
		"config": map[string]any{},
	}))
	if errObj != nil {
		t.Fatalf("browse absolute: %+v", errObj)
	}
	folders := result.(map[string]any)["folders"].([]browseFolder)
	if len(folders) != 1 || folders[0].Name != "Games" {
		t.Fatalf("unexpected folders: %+v", folders)
	}
	if !isAbsoluteRequest(folders[0].Path) || folders[0].DisplayPath != folders[0].Path {
		t.Fatalf("expected an absolute path and display path, got %+v", folders[0])
	}
}

func TestBrowseReturnsRelativePathsOnceABaseIsChosen(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "Games", "SNES"), 0o755); err != nil {
		t.Fatalf("create folders: %v", err)
	}

	result, errObj := handleBrowse(mustConfigJSON(t, map[string]any{
		"path": "Games",
		"config": map[string]any{
			"base_path":     base,
			"include_paths": []any{},
		},
	}))
	if errObj != nil {
		t.Fatalf("browse relative: %+v", errObj)
	}
	folders := result.(map[string]any)["folders"].([]browseFolder)
	if len(folders) != 1 || folders[0].Path != "Games/SNES" {
		t.Fatalf("expected a base-relative path, got %+v", folders)
	}
}

func TestBrowseRefusesARelativePathWithoutABase(t *testing.T) {
	// Ambiguous by construction: without a base there is nothing to resolve
	// "Games" against, so guessing a drive would be worse than refusing.
	_, errObj := handleBrowse(mustConfigJSON(t, map[string]any{"path": "Games", "config": map[string]any{}}))
	if errObj == nil || errObj.Code != "INVALID_PARAMS" {
		t.Fatalf("expected INVALID_PARAMS, got %+v", errObj)
	}
}

func TestBrowseCannotEscapeTheBase(t *testing.T) {
	base := t.TempDir()
	_, errObj := handleBrowse(mustConfigJSON(t, map[string]any{
		"path":   "../",
		"config": map[string]any{"base_path": base, "include_paths": []any{}},
	}))
	if errObj == nil {
		t.Fatal("expected traversal out of the base to be refused")
	}
}

func TestBrowseOmitsFilesAndLinks(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "game.sfc"), "rom")
	if err := os.MkdirAll(filepath.Join(base, "Games"), 0o755); err != nil {
		t.Fatalf("create folder: %v", err)
	}

	result, errObj := handleBrowse(mustConfigJSON(t, map[string]any{
		"path":   "",
		"config": map[string]any{"base_path": base, "include_paths": []any{}},
	}))
	if errObj != nil {
		t.Fatalf("browse: %+v", errObj)
	}
	folders := result.(map[string]any)["folders"].([]browseFolder)
	if len(folders) != 1 || folders[0].Name != "Games" {
		t.Fatalf("expected only the folder, got %+v", folders)
	}
}

// --------------- check_config ---------------

func TestCheckConfigReportsIdentityForAUsableFolder(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "Games"), 0o755); err != nil {
		t.Fatalf("create folder: %v", err)
	}

	result, errObj := handleCheckConfig(mustConfigJSON(t, map[string]any{
		"config": map[string]any{
			"base_path":     base,
			"include_paths": []any{map[string]any{"path": "Games", "recursive": true}},
		},
	}))
	if errObj != nil {
		t.Fatalf("check config: %+v", errObj)
	}
	payload := result.(map[string]any)
	if payload["status"] != "ok" {
		t.Fatalf("expected ok, got %+v", payload)
	}
	if identity, _ := payload["source_identity"].(string); !strings.HasPrefix(identity, "localdir:") {
		t.Fatalf("expected a source identity, got %+v", payload)
	}
}

func TestCheckConfigReportsProblemsAsAResultNotAnError(t *testing.T) {
	// The console renders a result message; an IPC error would surface as a
	// generic failure the operator cannot act on.
	result, errObj := handleCheckConfig(mustConfigJSON(t, map[string]any{
		"config": map[string]any{"base_path": "relative/path", "include_paths": []any{}},
	}))
	if errObj != nil {
		t.Fatalf("expected a result, got an IPC error: %+v", errObj)
	}
	payload := result.(map[string]any)
	if payload["status"] != "error" {
		t.Fatalf("expected an error status, got %+v", payload)
	}
	if _, present := payload["source_identity"]; present {
		t.Fatal("an unverifiable folder must not carry an identity; the server skips duplicate detection when it is empty")
	}
}

func TestCheckConfigRejectsAMistypedIncludePath(t *testing.T) {
	base := t.TempDir()
	result, errObj := handleCheckConfig(mustConfigJSON(t, map[string]any{
		"config": map[string]any{
			"base_path":     base,
			"include_paths": []any{map[string]any{"path": "Typo", "recursive": true}},
		},
	}))
	if errObj != nil {
		t.Fatalf("check config: %+v", errObj)
	}
	// Left alone this scans nothing and looks like an empty library.
	if result.(map[string]any)["status"] != "error" {
		t.Fatalf("expected a mistyped include path to be reported, got %+v", result)
	}
}

// --------------- Deletion ---------------

func TestSourceDeletePathWithinRoot(t *testing.T) {
	for _, test := range []struct {
		root, file string
		want       bool
	}{
		{"Games/SNES", "Games/SNES/game.sfc", true},
		{"Games/SNES", "Games/SNES", true},
		{"Games/SNES", "Games/SNES Extras/manual.pdf", false},
		{"Games/SNES", "Games/Genesis/game.md", false},
		{"", "Games/game.sfc", true},
	} {
		if got := sourceDeletePathWithinRoot(test.root, test.file); got != test.want {
			t.Fatalf("root %q file %q: got %v want %v", test.root, test.file, got, test.want)
		}
	}
}

func TestBuildSourceDeletePlanRefusesFilesOutsideTheRoot(t *testing.T) {
	config := LocalConfig{BasePath: `D:\Games`}
	_, errObj := buildSourceDeletePlan("Games/SNES", config, []sourceDeleteFile{
		{Path: "Games/Genesis/game.md"},
	})
	if errObj == nil || errObj.Code != "NOT_ALLOWED" {
		t.Fatalf("expected NOT_ALLOWED, got %+v", errObj)
	}
}

func TestBuildSourceDeletePlanRefusesFilesOutsideTheIncludeScope(t *testing.T) {
	config := LocalConfig{
		BasePath:     `D:\Games`,
		IncludePaths: []sourcescope.IncludePath{{Path: "Games/SNES", Recursive: true}},
	}
	_, errObj := buildSourceDeletePlan("Other", config, []sourceDeleteFile{{Path: "Other/game.sfc"}})
	if errObj == nil || errObj.Code != "NOT_ALLOWED" {
		t.Fatalf("expected NOT_ALLOWED, got %+v", errObj)
	}
}

func TestBuildSourceDeletePlanMarksDirectoriesForPruning(t *testing.T) {
	config := LocalConfig{BasePath: `D:\Games`}
	items, errObj := buildSourceDeletePlan("Games/SNES", config, []sourceDeleteFile{
		{Path: "Games/SNES/game.sfc", Size: 42},
		{Path: "Games/SNES", IsDir: true},
	})
	if errObj != nil {
		t.Fatalf("build plan: %+v", errObj)
	}
	if items[0].Action != "delete" || items[1].Action != "prune_empty" {
		t.Fatalf("unexpected actions: %+v", items)
	}
}

// fakeDeleteFS records what was removed so ordering and refusal rules can be
// asserted without touching a real filesystem.
type fakeDeleteFS struct {
	infos    map[string]os.FileInfo
	entries  map[string][]os.DirEntry
	errs     map[string]error
	removed  []string
	failures map[string]error
}

func (f *fakeDeleteFS) Lstat(logicalPath string) (os.FileInfo, error) {
	if err, ok := f.errs[logicalPath]; ok {
		return nil, err
	}
	info, ok := f.infos[logicalPath]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return info, nil
}

func (f *fakeDeleteFS) ReadDir(logicalPath string) ([]os.DirEntry, error) {
	return f.entries[logicalPath], nil
}

func (f *fakeDeleteFS) Remove(logicalPath string) error {
	if err, ok := f.failures[logicalPath]; ok {
		return err
	}
	f.removed = append(f.removed, logicalPath)
	return nil
}

type fakeFileInfo struct {
	name  string
	mode  os.FileMode
	isDir bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return nil }

func regularFile(name string) os.FileInfo { return fakeFileInfo{name: name} }
func directory(name string) os.FileInfo {
	return fakeFileInfo{name: name, mode: os.ModeDir, isDir: true}
}

func TestExecuteSourceDeletePlanRemovesFilesBeforeDeepestDirectories(t *testing.T) {
	target := &fakeDeleteFS{
		infos: map[string]os.FileInfo{
			"Games/SNES/Sub/game.sfc": regularFile("game.sfc"),
			"Games/SNES/Sub":          directory("Sub"),
			"Games/SNES":              directory("SNES"),
		},
	}
	items := []sourceDeletePlanItem{
		{Path: "Games/SNES", IsDir: true, Action: "prune_empty"},
		{Path: "Games/SNES/Sub", IsDir: true, Action: "prune_empty"},
		{Path: "Games/SNES/Sub/game.sfc", Action: "delete"},
	}

	deleted, _, errObj := executeSourceDeletePlan(target, items)
	if errObj != nil {
		t.Fatalf("execute: %+v", errObj)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 removals, got %d", deleted)
	}
	want := []string{"Games/SNES/Sub/game.sfc", "Games/SNES/Sub", "Games/SNES"}
	for i, path := range want {
		if target.removed[i] != path {
			t.Fatalf("removal order %v, want %v", target.removed, want)
		}
	}
}

func TestExecuteSourceDeletePlanRefusesALinkBeforeDeletingAnything(t *testing.T) {
	target := &fakeDeleteFS{
		infos: map[string]os.FileInfo{
			"Games/SNES/game.sfc": regularFile("game.sfc"),
			"Games/SNES/link.sfc": fakeFileInfo{name: "link.sfc", mode: os.ModeSymlink},
		},
	}
	items := []sourceDeletePlanItem{
		{Path: "Games/SNES/game.sfc", Action: "delete"},
		{Path: "Games/SNES/link.sfc", Action: "delete"},
	}

	_, _, errObj := executeSourceDeletePlan(target, items)
	if errObj == nil || errObj.Code != "NOT_ALLOWED" {
		t.Fatalf("expected NOT_ALLOWED, got %+v", errObj)
	}
	// The whole point of the preflight: one refused item deletes nothing.
	if len(target.removed) != 0 {
		t.Fatalf("preflight failed to run before mutation, removed %v", target.removed)
	}
}

func TestExecuteSourceDeletePlanKeepsDirectoriesHoldingUnauthorizedFiles(t *testing.T) {
	target := &fakeDeleteFS{
		infos: map[string]os.FileInfo{
			"Games/SNES/game.sfc": regularFile("game.sfc"),
			"Games/SNES":          directory("SNES"),
		},
		entries: map[string][]os.DirEntry{
			"Games/SNES": {fs.FileInfoToDirEntry(regularFile("stranger.txt"))},
		},
	}
	items := []sourceDeletePlanItem{
		{Path: "Games/SNES/game.sfc", Action: "delete"},
		{Path: "Games/SNES", IsDir: true, Action: "prune_empty"},
	}

	deleted, warnings, errObj := executeSourceDeletePlan(target, items)
	if errObj != nil {
		t.Fatalf("execute: %+v", errObj)
	}
	if deleted != 1 || len(target.removed) != 1 {
		t.Fatalf("expected only the file removed, got %v", target.removed)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "did not authorize") {
		t.Fatalf("expected a kept-directory warning, got %v", warnings)
	}
}

func TestExecuteSourceDeletePlanToleratesAFileThatIsAlreadyGone(t *testing.T) {
	target := &fakeDeleteFS{
		infos: map[string]os.FileInfo{"Games/SNES/present.sfc": regularFile("present.sfc")},
		errs:  map[string]error{"Games/SNES/absent.sfc": fs.ErrNotExist},
	}
	items := []sourceDeletePlanItem{
		{Path: "Games/SNES/present.sfc", Action: "delete"},
		{Path: "Games/SNES/absent.sfc", Action: "delete"},
	}

	deleted, warnings, errObj := executeSourceDeletePlan(target, items)
	if errObj != nil {
		t.Fatalf("execute: %+v", errObj)
	}
	if deleted != 2 {
		t.Fatalf("an already-deleted file should still count as done, got %d", deleted)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "already deleted") {
		t.Fatalf("expected an already-deleted warning, got %v", warnings)
	}
}

func TestExecuteSourceDeletePlanFailsClosedWhenInspectionFails(t *testing.T) {
	target := &fakeDeleteFS{
		infos: map[string]os.FileInfo{"Games/SNES/game.sfc": regularFile("game.sfc")},
		errs:  map[string]error{"Games/SNES/locked.sfc": errors.New("permission denied")},
	}
	items := []sourceDeletePlanItem{
		{Path: "Games/SNES/game.sfc", Action: "delete"},
		{Path: "Games/SNES/locked.sfc", Action: "delete"},
	}

	_, _, errObj := executeSourceDeletePlan(target, items)
	if errObj == nil || errObj.Code != "DELETE_FAILED" {
		t.Fatalf("expected DELETE_FAILED, got %+v", errObj)
	}
	if len(target.removed) != 0 {
		t.Fatalf("nothing should have been removed, got %v", target.removed)
	}
}

func TestExecuteSourceDeletePlanRefusesATypeMismatch(t *testing.T) {
	target := &fakeDeleteFS{
		infos: map[string]os.FileInfo{"Games/SNES": regularFile("SNES")},
	}
	items := []sourceDeletePlanItem{{Path: "Games/SNES", IsDir: true, Action: "prune_empty"}}

	_, _, errObj := executeSourceDeletePlan(target, items)
	if errObj == nil || errObj.Code != "NOT_ALLOWED" {
		t.Fatalf("expected NOT_ALLOWED for a directory that became a file, got %+v", errObj)
	}
}

func TestHandleSourceDeleteDryRunDoesNotTouchTheFilesystem(t *testing.T) {
	base := t.TempDir()
	gamePath := filepath.Join(base, "Games", "SNES", "game.sfc")
	writeFile(t, gamePath, "rom")

	result, errObj := handleSourceDelete(mustConfigJSON(t, map[string]any{
		"config":         map[string]any{"base_path": base, "include_paths": []any{}},
		"root_path":      "Games/SNES",
		"source_game_id": "sg-1",
		"dry_run":        true,
		"files":          []any{map[string]any{"path": "Games/SNES/game.sfc", "size": 3}},
	}))
	if errObj != nil {
		t.Fatalf("dry run: %+v", errObj)
	}

	payload := result.(map[string]any)
	if payload["plugin_id"] != pluginID || payload["action"] != "delete" {
		t.Fatalf("unexpected response shape: %+v", payload)
	}
	if payload["deleted_count"].(int) != 0 {
		t.Fatalf("a dry run must delete nothing, got %+v", payload["deleted_count"])
	}
	if _, err := os.Stat(gamePath); err != nil {
		t.Fatalf("dry run removed the file: %v", err)
	}
}

func TestHandleSourceDeleteRefusesARootOutsideTheIncludeScope(t *testing.T) {
	base := t.TempDir()
	_, errObj := handleSourceDelete(mustConfigJSON(t, map[string]any{
		"config": map[string]any{
			"base_path":     base,
			"include_paths": []any{map[string]any{"path": "Games", "recursive": true}},
		},
		"root_path": "Elsewhere",
		"files":     []any{map[string]any{"path": "Elsewhere/game.sfc"}},
	}))
	if errObj == nil || errObj.Code != "NOT_ALLOWED" {
		t.Fatalf("expected NOT_ALLOWED, got %+v", errObj)
	}
}

func TestHandleSourceDeleteRemovesFilesAndPrunesEmptyFolders(t *testing.T) {
	base := t.TempDir()
	gamePath := filepath.Join(base, "Games", "SNES", "game.sfc")
	writeFile(t, gamePath, "rom")

	result, errObj := handleSourceDelete(mustConfigJSON(t, map[string]any{
		"config":         map[string]any{"base_path": base, "include_paths": []any{}},
		"root_path":      "Games/SNES",
		"source_game_id": "sg-1",
		"files": []any{
			map[string]any{"path": "Games/SNES/game.sfc", "size": 3},
			map[string]any{"path": "Games/SNES", "is_dir": true},
		},
	}))
	if errObj != nil {
		t.Fatalf("delete: %+v", errObj)
	}
	if result.(map[string]any)["deleted_count"].(int) != 2 {
		t.Fatalf("expected two removals, got %+v", result)
	}
	if _, err := os.Stat(gamePath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("file survived deletion: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "Games", "SNES")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty folder was not pruned: %v", err)
	}
}

// --------------- Dispatch ---------------

func TestDispatchRejectsUnknownMethods(t *testing.T) {
	resp := dispatch(Request{ID: "1", Method: "source.transfer.begin"}, nil)
	if resp.Error == nil || resp.Error.Code != "NOT_SUPPORTED" {
		t.Fatalf("move support is deliberately not declared; expected NOT_SUPPORTED, got %+v", resp.Error)
	}
}

func TestPluginInfoMatchesTheManifest(t *testing.T) {
	raw, err := os.ReadFile("game-source-local.plugin.json")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	info := pluginInfo()
	if info["plugin_id"] != manifest["plugin_id"] {
		t.Fatalf("plugin_id drifted: %v vs %v", info["plugin_id"], manifest["plugin_id"])
	}
	if info["plugin_version"] != manifest["plugin_version"] {
		t.Fatalf("plugin_version drifted: %v vs %v", info["plugin_version"], manifest["plugin_version"])
	}

	manifestProvides := manifest["provides"].([]any)
	infoProvides := info["provides"].([]string)
	if len(manifestProvides) != len(infoProvides) {
		t.Fatalf("provides drifted: %v vs %v", infoProvides, manifestProvides)
	}
	for i, value := range manifestProvides {
		if value.(string) != infoProvides[i] {
			t.Fatalf("provides drifted at %d: %v vs %v", i, infoProvides[i], value)
		}
	}

	// The exec name has to match the directory name or build.ps1 produces a
	// binary the plugin host cannot find.
	if manifest["exec"] != "local.exe" {
		t.Fatalf("exec must match the plugin directory name, got %v", manifest["exec"])
	}
}

func keysOf(files map[string]map[string]any) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	return keys
}

func TestListFilesReportsProgressWhileWalking(t *testing.T) {
	base := t.TempDir()
	// Enough entries to cross the reporting threshold more than once.
	for index := 0; index < 600; index++ {
		writeFile(t, filepath.Join(base, "Games", fmt.Sprintf("game-%03d.sfc", index)), "rom")
	}

	type report struct {
		current int64
		item    string
	}
	var reports []report
	if _, err := listFiles(LocalConfig{BasePath: base}, func(current int64, item string) {
		reports = append(reports, report{current, item})
	}); err != nil {
		t.Fatalf("list files: %v", err)
	}

	if len(reports) < 3 {
		t.Fatalf("expected several progress reports during a 600-entry walk, got %d", len(reports))
	}
	// A silent walk is indistinguishable from a hung one, which is the whole
	// reason this exists.
	if reports[0].item == "" {
		t.Fatal("the first report should name the folder being read")
	}
	last := reports[len(reports)-1]
	if last.current < 600 {
		t.Fatalf("the final report should cover every entry, got %d", last.current)
	}
	for index := 1; index < len(reports); index++ {
		if reports[index].current < reports[index-1].current {
			t.Fatalf("progress went backwards: %d then %d", reports[index-1].current, reports[index].current)
		}
	}
}

func TestListFilesWorksWithoutAProgressReporter(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "game.sfc"), "rom")
	if _, err := listFiles(LocalConfig{BasePath: base}, nil); err != nil {
		t.Fatalf("a nil reporter must be accepted: %v", err)
	}
}
