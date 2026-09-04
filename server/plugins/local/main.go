// Command local is the MGA game source plugin for a directory on the server's
// own filesystem.
//
// It is the SMB plugin's shape with the network removed: one absolute base
// folder per connection, scoped by include and exclude paths that are relative
// to that base. Keeping the include paths relative is not a style choice —
// sourcescope.NormalizeLogicalPath strips a leading slash, so a POSIX absolute
// path stored as an include would silently become a relative one and match the
// wrong subtree. One base per connection keeps every sourcescope function
// correct on both Windows and Linux. Two drives means two connections.
//
// Links are never followed. Symlinks, junctions and other reparse points are
// skipped during listing, refused during deletion, and cannot be resolved
// through at delivery time. A local filesystem is the one place where a cycle
// or an escape is genuinely reachable, so the walker refuses rather than
// guesses in every ambiguous case.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
)

const pluginID = "game-source-local"

// Walk limits. A refusal an operator can act on beats a scan that never
// returns, so both are hard stops rather than warnings.
const (
	maxWalkDepth   = 64
	maxWalkEntries = 2_000_000
	maxBrowseItems = 5000
)

type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type Response struct {
	ID     string `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type LocalConfig struct {
	BasePath     string                    `json:"base_path"`
	IncludePaths []sourcescope.IncludePath `json:"include_paths"`
}

// decodeLocalConfig is the single funnel every handler uses to read config.
//
// The server sends the bare config map for source.filesystem.list but nests it
// under "config" for check_config and delete, so both shapes are accepted.
// Validation and normalization happen here so no handler can skip them.
func decodeLocalConfig(payload json.RawMessage) (LocalConfig, error) {
	var configMap map[string]any
	if err := json.Unmarshal(payload, &configMap); err != nil {
		return LocalConfig{}, err
	}
	if nestedConfig, ok := configMap["config"].(map[string]any); ok {
		configMap = nestedConfig
	}
	if err := sourcescope.ValidateConfig(pluginID, configMap); err != nil {
		return LocalConfig{}, err
	}
	normalized := sourcescope.NormalizeConfig(pluginID, configMap)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return LocalConfig{}, err
	}
	var config LocalConfig
	if err := json.Unmarshal(encoded, &config); err != nil {
		return LocalConfig{}, err
	}
	return config, nil
}

// normalizedIncludePaths returns the connection's scope, defaulting to the
// whole base when nothing is configured.
func normalizedIncludePaths(config LocalConfig) []sourcescope.IncludePath {
	if len(config.IncludePaths) == 0 {
		return []sourcescope.IncludePath{{Path: "", Recursive: true}}
	}
	includes := make([]sourcescope.IncludePath, 0, len(config.IncludePaths))
	for _, include := range config.IncludePaths {
		includes = append(includes, sourcescope.IncludePath{
			Path:         sourcescope.NormalizeLogicalPath(include.Path),
			Recursive:    include.Recursive,
			ExcludePaths: normalizeStringPaths(include.ExcludePaths),
		})
	}
	return includes
}

func normalizeStringPaths(paths []string) []string {
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		if cleaned := sourcescope.NormalizeLogicalPath(path); cleaned != "" {
			normalized = append(normalized, cleaned)
		}
	}
	return normalized
}

func pathExcluded(logicalPath string, excludes []string) bool {
	logicalPath = sourcescope.NormalizeLogicalPath(logicalPath)
	for _, exclude := range excludes {
		exclude = sourcescope.NormalizeLogicalPath(exclude)
		if exclude == "" {
			continue
		}
		if logicalPath == exclude || strings.HasPrefix(logicalPath, exclude+"/") {
			return true
		}
	}
	return false
}

func joinLogicalPath(basePath, child string) string {
	base := sourcescope.NormalizeLogicalPath(basePath)
	part := sourcescope.NormalizeLogicalPath(child)
	if base == "" {
		return part
	}
	if part == "" {
		return base
	}
	return base + "/" + part
}

// isMGAControlDir reports whether a directory belongs to MGA's own bookkeeping
// rather than to the operator's library.
func isMGAControlDir(name string) bool {
	return strings.EqualFold(name, ".mga") || strings.HasPrefix(strings.ToLower(name), ".mga-transfer-")
}

// --------------- Path containment ---------------

// resolveBase canonicalizes the connection's base folder and proves it is a
// real, readable directory. Every other path in this plugin is resolved against
// the value it returns, so a base that cannot be verified fails the whole
// operation rather than defaulting to something.
func resolveBase(config LocalConfig) (string, error) {
	basePath := strings.TrimSpace(config.BasePath)
	if basePath == "" {
		return "", errors.New("base_path is required")
	}
	if !filepath.IsAbs(basePath) {
		return "", fmt.Errorf("base_path %q must be an absolute path", basePath)
	}

	absolute, err := filepath.Abs(filepath.Clean(basePath))
	if err != nil {
		return "", fmt.Errorf("resolve base_path %q: %w", basePath, err)
	}

	// Refuse a base that is itself a link. Allowing one would mean the scope an
	// operator reviewed and the bytes MGA serves could diverge later without
	// the configuration changing.
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("read base_path %q: %w", basePath, err)
	}
	if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return "", fmt.Errorf("base_path %q is a link or reparse point; point the connection at the real folder", basePath)
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve base_path %q: %w", basePath, err)
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("read base_path %q: %w", basePath, err)
	}
	if !resolvedInfo.IsDir() {
		return "", fmt.Errorf("base_path %q is not a folder", basePath)
	}
	return resolved, nil
}

// resolveWithinBase turns a logical path into a real one and proves it stays
// inside the base once every link on the way has been followed.
//
// mustExist separates the two callers: listing and deleting work on paths that
// are supposed to be there, while a plan is still worth building for a file
// that raced away, so the caller decides whether absence is fatal.
func resolveWithinBase(base, logicalPath string, mustExist bool) (string, error) {
	// Inspect the caller's own text before normalizing. NormalizeLogicalPath
	// clamps "../secret" to "secret" rather than rejecting it, so a traversal
	// would come back as a different file that happens to be in scope. Reading
	// something other than what was asked for is worse than refusing.
	for _, segment := range strings.FieldsFunc(logicalPath, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == ".." {
			return "", fmt.Errorf("path %q escapes the base folder", logicalPath)
		}
	}

	normalized := sourcescope.NormalizeLogicalPath(logicalPath)
	if normalized == "" {
		return base, nil
	}

	candidate := filepath.Join(base, filepath.FromSlash(normalized))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		if !mustExist && errors.Is(err, fs.ErrNotExist) {
			// The parent chain still has to be contained, so check the
			// unresolved candidate rather than trusting it.
			if err := containedIn(base, candidate); err != nil {
				return "", fmt.Errorf("path %q %w", logicalPath, err)
			}
			return candidate, nil
		}
		return "", fmt.Errorf("resolve %q: %w", logicalPath, err)
	}
	if err := containedIn(base, resolved); err != nil {
		// Report what the caller asked for, never where the link pointed. A
		// link out of the base must not become a way to read the server's
		// directory layout out of an error message.
		return "", fmt.Errorf("path %q %w", logicalPath, err)
	}
	return resolved, nil
}

// containedIn refuses anything that is not genuinely under base. The volume
// check comes first because filepath.Rel across volumes on Windows produces a
// result that no ".." test would catch.
func containedIn(base, candidate string) error {
	if !strings.EqualFold(filepath.VolumeName(base), filepath.VolumeName(candidate)) {
		return errors.New("is on a different volume than the base folder")
	}
	relative, err := filepath.Rel(base, candidate)
	if err != nil {
		return errors.New("could not be compared against the base folder")
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("resolves outside the base folder")
	}
	return nil
}

// sourceIdentity names the folder this connection points at, so the server can
// tell an operator they have already connected it.
//
// The path is canonicalized first, which collapses junction and symlink
// aliases, and lowercased only on Windows: NTFS treats Games and games as one
// folder while a POSIX filesystem treats them as two. The result is hashed so
// the server's filesystem layout never travels through an API response.
//
// It cannot see through two drive letters mapped to the same network share,
// bind mounts, or subst drives. Those stay the operator's to notice.
func sourceIdentity(resolvedBase string) string {
	key := filepath.ToSlash(resolvedBase)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	sum := sha256.Sum256([]byte(key))
	return "localdir:" + hex.EncodeToString(sum[:])
}

// --------------- Listing ---------------

type walkFrame struct {
	realPath    string
	logicalPath string
	depth       int
}

// listFiles returns a flat listing of every file and directory in scope. No
// filtering or classification — the server's scanner owns that.
func listFiles(config LocalConfig) ([]map[string]any, error) {
	base, err := resolveBase(config)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]map[string]any)
	visited := make(map[string]bool)
	skippedLinks := 0

	for _, include := range normalizedIncludePaths(config) {
		root, err := resolveWithinBase(base, include.Path, true)
		if err != nil {
			// One include path that has gone away must not lose the operator
			// the rest of their library.
			log.Printf("include path %q is unavailable, skipping: %v", include.Path, err)
			continue
		}

		stack := []walkFrame{{realPath: root, logicalPath: include.Path, depth: 0}}
		for len(stack) > 0 {
			frame := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			if frame.depth > maxWalkDepth {
				return nil, fmt.Errorf("folder nesting under %q exceeds %d levels; narrow include_paths", include.Path, maxWalkDepth)
			}
			// Even though links are never traversed, a canonical-path guard is
			// cheap insurance against overlapping includes and bind mounts.
			if visited[frame.realPath] {
				continue
			}
			visited[frame.realPath] = true

			entries, err := os.ReadDir(frame.realPath)
			if err != nil {
				log.Printf("read folder %q, skipping: %v", frame.logicalPath, err)
				continue
			}

			for _, entry := range entries {
				name := entry.Name()
				if entry.IsDir() && isMGAControlDir(name) {
					continue
				}

				// A link is not emitted and not followed. Emitting one would
				// leave a row that delivery resolves elsewhere and deletion
				// refuses to touch — permanently stuck.
				if entry.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
					skippedLinks++
					continue
				}
				if !entry.IsDir() && entry.Type()&fs.ModeType != 0 {
					continue
				}

				logicalPath := joinLogicalPath(frame.logicalPath, name)
				if pathExcluded(logicalPath, include.ExcludePaths) {
					continue
				}
				if len(seen) >= maxWalkEntries {
					return nil, fmt.Errorf("more than %d entries in scope; narrow include_paths", maxWalkEntries)
				}

				realPath := filepath.Join(frame.realPath, name)
				if entry.IsDir() {
					recordEntry(seen, logicalPath, name, true, nil)
					if include.Recursive {
						stack = append(stack, walkFrame{realPath: realPath, logicalPath: logicalPath, depth: frame.depth + 1})
					}
					continue
				}

				info, err := entry.Info()
				if err != nil {
					// The file raced away mid-walk; the next scan will settle it.
					log.Printf("read file %q, skipping: %v", logicalPath, err)
					continue
				}
				recordEntry(seen, logicalPath, name, false, info)
			}
		}
	}

	if skippedLinks > 0 {
		log.Printf("skipped %d link or reparse entries; MGA does not scan through them", skippedLinks)
	}

	paths := make([]string, 0, len(seen))
	for logicalPath := range seen {
		paths = append(paths, logicalPath)
	}
	sort.Strings(paths)

	files := make([]map[string]any, 0, len(paths))
	for _, logicalPath := range paths {
		files = append(files, seen[logicalPath])
	}
	return files, nil
}

// recordEntry deduplicates by logical path so overlapping include paths report
// a file once.
func recordEntry(seen map[string]map[string]any, logicalPath, name string, isDir bool, info os.FileInfo) {
	if logicalPath == "" {
		return
	}
	if _, exists := seen[logicalPath]; exists {
		return
	}
	record := map[string]any{"path": logicalPath, "name": name, "is_dir": isDir}
	if !isDir && info != nil {
		record["size"] = info.Size()
		record["mod_time"] = info.ModTime().UTC().Format(time.RFC3339)
	}
	seen[logicalPath] = record
}

// --------------- Browsing ---------------

type browseFolder struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	DisplayPath string `json:"display_path"`
}

// handleBrowse powers the folder picker.
//
// The same method serves two different fields, so the shape of what it returns
// depends on which one is asking. Choosing the base folder browses the machine
// and answers with absolute paths; choosing an include or exclude path browses
// under an already-chosen base and answers with paths relative to it. A
// relative request with no base is genuinely ambiguous, so it refuses.
func handleBrowse(params json.RawMessage) (any, *Error) {
	var body struct {
		Path   string          `json:"path"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(params, &body); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	var config LocalConfig
	if len(body.Config) > 0 {
		// A half-filled form is normal while browsing, so a config that does
		// not validate yet only means "no base chosen".
		if decoded, err := decodeLocalConfig(body.Config); err == nil {
			config = decoded
		}
	}

	requestedPath := strings.TrimSpace(body.Path)
	basePath := strings.TrimSpace(config.BasePath)

	if requestedPath == "" && basePath == "" {
		return map[string]any{"folders": listRoots()}, nil
	}
	if isAbsoluteRequest(requestedPath) {
		return browseAbsolute(requestedPath)
	}
	if basePath == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "Choose a base folder before browsing include paths."}
	}
	return browseRelative(config, requestedPath)
}

// isAbsoluteRequest also accepts a bare drive token such as "C:", which is what
// the roots listing hands back on Windows.
func isAbsoluteRequest(path string) bool {
	if path == "" {
		return false
	}
	if filepath.IsAbs(path) || filepath.IsAbs(filepath.FromSlash(path)) {
		return true
	}
	return len(path) == 2 && path[1] == ':'
}

func listRoots() []browseFolder {
	if runtime.GOOS != "windows" {
		return []browseFolder{{Name: "/", Path: "/", DisplayPath: "/"}}
	}
	folders := make([]browseFolder, 0, 26)
	for letter := 'A'; letter <= 'Z'; letter++ {
		root := string(letter) + ":\\"
		if _, err := os.Stat(root); err != nil {
			continue
		}
		folders = append(folders, browseFolder{
			Name:        string(letter) + ":",
			Path:        string(letter) + ":/",
			DisplayPath: string(letter) + ":/",
		})
	}
	return folders
}

func browseAbsolute(requestedPath string) (any, *Error) {
	target := filepath.FromSlash(requestedPath)
	if len(requestedPath) == 2 && requestedPath[1] == ':' {
		target = requestedPath + string(filepath.Separator)
	}
	names, err := readDirNames(target)
	if err != nil {
		return nil, &Error{Code: "LIST_FAILED", Message: fmt.Sprintf("cannot read folder %q: %v", requestedPath, err)}
	}
	folders := make([]browseFolder, 0, len(names))
	for _, name := range names {
		full := filepath.ToSlash(filepath.Join(target, name))
		folders = append(folders, browseFolder{Name: name, Path: full, DisplayPath: full})
	}
	return map[string]any{"folders": folders}, nil
}

func browseRelative(config LocalConfig, requestedPath string) (any, *Error) {
	base, err := resolveBase(config)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	current, err := resolveWithinBase(base, requestedPath, true)
	if err != nil {
		return nil, &Error{Code: "NOT_ALLOWED", Message: err.Error()}
	}
	names, err := readDirNames(current)
	if err != nil {
		return nil, &Error{Code: "LIST_FAILED", Message: fmt.Sprintf("cannot read folder %q: %v", requestedPath, err)}
	}
	logicalBase := sourcescope.NormalizeLogicalPath(requestedPath)
	folders := make([]browseFolder, 0, len(names))
	for _, name := range names {
		logicalPath := joinLogicalPath(logicalBase, name)
		folders = append(folders, browseFolder{Name: name, Path: logicalPath, DisplayPath: logicalPath})
	}
	return map[string]any{"folders": folders}, nil
}

// readDirNames returns the browsable child folders of one directory. Links and
// MGA control folders are omitted, matching what a scan would actually read.
func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			continue
		}
		if isMGAControlDir(entry.Name()) {
			continue
		}
		if len(names) >= maxBrowseItems {
			break
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// --------------- Deletion ---------------

type sourceDeleteFile struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type sourceDeletePlanItem struct {
	Path   string `json:"path"`
	IsDir  bool   `json:"is_dir,omitempty"`
	Size   int64  `json:"size,omitempty"`
	Action string `json:"action"`
}

func handleSourceDelete(params json.RawMessage) (any, *Error) {
	var body struct {
		RootPath     string             `json:"root_path"`
		SourceGameID string             `json:"source_game_id"`
		Files        []sourceDeleteFile `json:"files"`
		DryRun       bool               `json:"dry_run"`
	}
	if err := json.Unmarshal(params, &body); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	config, err := decodeLocalConfig(params)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	rootPath := sourcescope.NormalizeLogicalPath(body.RootPath)
	if rootPath == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "root_path is required"}
	}
	if !sourcescope.ScopeContainsRootPath(rootPath, normalizedIncludePaths(config)) {
		return nil, &Error{Code: "NOT_ALLOWED", Message: "root_path is outside the configured include_paths scope"}
	}
	if len(body.Files) == 0 {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "files are required"}
	}

	items, errObj := buildSourceDeletePlan(rootPath, config, body.Files)
	if errObj != nil {
		return nil, errObj
	}
	if body.DryRun {
		return sourceDeleteResponse(body.SourceGameID, "delete", items, 0, []string{}), nil
	}

	base, err := resolveBase(config)
	if err != nil {
		return nil, &Error{Code: "DELETE_FAILED", Message: err.Error()}
	}

	deletedCount, warnings, errObj := executeSourceDeletePlan(&osDeleteFS{base: base}, items)
	if errObj != nil {
		return nil, errObj
	}
	return sourceDeleteResponse(body.SourceGameID, "delete", items, deletedCount, warnings), nil
}

// buildSourceDeletePlan refuses the whole request if any single file fails a
// boundary check. A partially authorized deletion is not a thing.
func buildSourceDeletePlan(rootPath string, config LocalConfig, files []sourceDeleteFile) ([]sourceDeletePlanItem, *Error) {
	items := make([]sourceDeletePlanItem, 0, len(files))
	for _, file := range files {
		filePath := sourcescope.NormalizeLogicalPath(file.Path)
		if filePath == "" {
			return nil, &Error{Code: "INVALID_PARAMS", Message: "file path is required"}
		}
		if !sourceDeletePathWithinRoot(rootPath, filePath) {
			return nil, &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("file %q is outside root_path %q", filePath, rootPath)}
		}
		if !sourcescope.ScopeContainsRootPath(filePath, normalizedIncludePaths(config)) {
			return nil, &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("file %q is outside the configured include_paths scope", filePath)}
		}
		items = append(items, sourceDeletePlanItem{
			Path:   filePath,
			IsDir:  file.IsDir,
			Size:   file.Size,
			Action: sourceDeleteAction(file.IsDir),
		})
	}
	return items, nil
}

func sourceDeleteAction(isDir bool) string {
	if isDir {
		return "prune_empty"
	}
	return "delete"
}

// localDeleteFS is the deletion surface, kept narrow and injectable so the
// ordering and refusal rules can be tested without a real filesystem.
type localDeleteFS interface {
	Lstat(logicalPath string) (os.FileInfo, error)
	ReadDir(logicalPath string) ([]os.DirEntry, error)
	Remove(logicalPath string) error
}

// osDeleteFS re-resolves containment inside every method, so a path is checked
// at the moment it is acted on and not merely when the plan was built.
type osDeleteFS struct {
	base string
}

func (f *osDeleteFS) resolve(logicalPath string) (string, error) {
	return resolveWithinBase(f.base, logicalPath, false)
}

func (f *osDeleteFS) Lstat(logicalPath string) (os.FileInfo, error) {
	resolved, err := f.resolve(logicalPath)
	if err != nil {
		return nil, err
	}
	return os.Lstat(resolved)
}

func (f *osDeleteFS) ReadDir(logicalPath string) ([]os.DirEntry, error) {
	resolved, err := f.resolve(logicalPath)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(resolved)
}

func (f *osDeleteFS) Remove(logicalPath string) error {
	resolved, err := f.resolve(logicalPath)
	if err != nil {
		return err
	}
	return os.Remove(resolved)
}

// executeSourceDeletePlan inspects every item before removing any of them, so a
// plan containing one file MGA must not touch deletes nothing at all.
func executeSourceDeletePlan(target localDeleteFS, items []sourceDeletePlanItem) (int, []string, *Error) {
	sortedItems := append([]sourceDeletePlanItem(nil), items...)
	sort.SliceStable(sortedItems, func(i, j int) bool {
		if sortedItems[i].IsDir != sortedItems[j].IsDir {
			return !sortedItems[i].IsDir
		}
		if sortedItems[i].IsDir {
			return len(sortedItems[i].Path) > len(sortedItems[j].Path)
		}
		return false
	})

	alreadyGone := make(map[string]bool)
	warnings := []string{}
	for _, item := range sortedItems {
		info, err := target.Lstat(item.Path)
		if err != nil {
			if sourceDeleteAlreadyGone(err) {
				alreadyGone[item.Path] = true
				warnings = append(warnings, fmt.Sprintf("%q was already deleted before this operation.", item.Path))
				continue
			}
			return 0, warnings, &Error{Code: "DELETE_FAILED", Message: fmt.Sprintf("inspect %q before deletion: %v", item.Path, err)}
		}
		if errObj := validateSourceDeleteFileInfo(item, info); errObj != nil {
			return 0, warnings, errObj
		}
	}

	deletedCount := 0
	for _, item := range sortedItems {
		if alreadyGone[item.Path] {
			deletedCount++
			continue
		}
		if item.IsDir {
			info, err := target.Lstat(item.Path)
			if err != nil {
				if sourceDeleteAlreadyGone(err) {
					warnings = append(warnings, fmt.Sprintf("%q was already deleted before empty-folder cleanup.", item.Path))
					deletedCount++
					continue
				}
				return deletedCount, warnings, &Error{Code: "DELETE_FAILED", Message: fmt.Sprintf("re-inspect directory %q before cleanup: %v", item.Path, err)}
			}
			if errObj := validateSourceDeleteFileInfo(item, info); errObj != nil {
				return deletedCount, warnings, errObj
			}
			entries, err := target.ReadDir(item.Path)
			if err != nil {
				if sourceDeleteAlreadyGone(err) {
					warnings = append(warnings, fmt.Sprintf("%q was already deleted before empty-folder cleanup.", item.Path))
					deletedCount++
					continue
				}
				return deletedCount, warnings, &Error{Code: "DELETE_FAILED", Message: fmt.Sprintf("check whether directory %q is empty: %v", item.Path, err)}
			}
			if len(entries) != 0 {
				warnings = append(warnings, fmt.Sprintf("Directory %q was kept because it contains files MGA did not authorize for deletion.", item.Path))
				continue
			}
		}
		if err := target.Remove(item.Path); err != nil {
			if sourceDeleteAlreadyGone(err) {
				warnings = append(warnings, fmt.Sprintf("%q was already deleted before this operation.", item.Path))
				deletedCount++
				continue
			}
			if item.IsDir && sourceDeleteDirectoryNotEmpty(err) {
				warnings = append(warnings, fmt.Sprintf("Directory %q was kept because it became non-empty during cleanup.", item.Path))
				continue
			}
			return deletedCount, warnings, &Error{Code: "DELETE_FAILED", Message: fmt.Sprintf("remove %q: %v", item.Path, err)}
		}
		deletedCount++
	}
	return deletedCount, warnings, nil
}

func validateSourceDeleteFileInfo(item sourceDeletePlanItem, info os.FileInfo) *Error {
	if info == nil {
		return &Error{Code: "DELETE_FAILED", Message: fmt.Sprintf("inspect %q before deletion: no file information returned", item.Path)}
	}
	if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("%q is a link or reparse point; MGA will not delete through it", item.Path)}
	}
	if item.IsDir != info.IsDir() {
		expected := "file"
		if item.IsDir {
			expected = "directory"
		}
		return &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("%q is no longer the expected %s", item.Path, expected)}
	}
	if !item.IsDir && info.Mode()&os.ModeType != 0 {
		return &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("%q is not an ordinary file", item.Path)}
	}
	return nil
}

func sourceDeleteDirectoryNotEmpty(err error) bool {
	if err == nil {
		return false
	}
	// Go surfaces this as ENOTEMPTY on POSIX and ERROR_DIR_NOT_EMPTY on
	// Windows, and neither is portably comparable without pulling in
	// x/sys, so match the wording both produce.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "directory not empty") || strings.Contains(msg, "the directory is not empty")
}

func sourceDeleteAlreadyGone(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "file does not exist") ||
		strings.Contains(msg, "cannot find the file") ||
		strings.Contains(msg, "cannot find the path")
}

func sourceDeleteResponse(sourceGameID, action string, items []sourceDeletePlanItem, deletedCount int, warnings []string) map[string]any {
	return map[string]any{
		"source_game_id": sourceGameID,
		"plugin_id":      pluginID,
		"action":         action,
		"summary":        fmt.Sprintf("%d item(s) will be permanently deleted.", len(items)),
		"items":          items,
		"warnings":       warnings,
		"deleted_count":  deletedCount,
	}
}

// sourceDeletePathWithinRoot matches on a full path segment, so a game rooted
// at "SNES" never authorizes deleting anything under "SNES Extras".
func sourceDeletePathWithinRoot(rootPath, filePath string) bool {
	rootPath = sourcescope.NormalizeLogicalPath(rootPath)
	filePath = sourcescope.NormalizeLogicalPath(filePath)
	if rootPath == "" {
		return filePath != ""
	}
	return filePath == rootPath || strings.HasPrefix(filePath, rootPath+"/")
}

// --------------- Config check ---------------

// handleCheckConfig verifies the connection before it is saved.
//
// Validation problems come back as a successful result carrying a message
// rather than as an IPC error, so the console shows the operator what to fix
// instead of a generic failure. An unverifiable base returns no
// source_identity: the server skips duplicate detection on an empty identity,
// so guessing one would let an unverified folder slip past that check.
func handleCheckConfig(params json.RawMessage) (any, *Error) {
	config, err := decodeLocalConfig(params)
	if err != nil {
		return map[string]any{"status": "error", "message": err.Error()}, nil
	}

	base, err := resolveBase(config)
	if err != nil {
		return map[string]any{"status": "error", "message": err.Error()}, nil
	}
	if _, err := os.ReadDir(base); err != nil {
		return map[string]any{"status": "error", "message": fmt.Sprintf("cannot read base_path %q: %v", config.BasePath, err)}, nil
	}

	// A mistyped include path must fail here. Left alone it would scan nothing
	// and look like an empty library.
	for _, include := range normalizedIncludePaths(config) {
		if include.Path == "" {
			continue
		}
		resolved, err := resolveWithinBase(base, include.Path, true)
		if err != nil {
			return map[string]any{"status": "error", "message": fmt.Sprintf("include path %q is not usable: %v", include.Path, err)}, nil
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return map[string]any{"status": "error", "message": fmt.Sprintf("include path %q is not a folder inside the base folder", include.Path)}, nil
		}
	}

	return map[string]any{
		"status":          "ok",
		"source_identity": sourceIdentity(base),
	}, nil
}

// --------------- Plugin metadata ---------------

func pluginInfo() map[string]any {
	return map[string]any{
		"plugin_id":      pluginID,
		"plugin_version": "1.0.0",
		"capabilities":   []string{"source"},
		"provides": []string{
			"source.filesystem.list",
			"source.filesystem.delete",
			"source.browse",
			"plugin.check_config",
		},
		"config": map[string]any{
			"base_path": map[string]any{
				"type":        "string",
				"required":    true,
				"description": "Absolute path to the folder on this server that holds your games. Use one connection per drive or root folder.",
			},
			"include_paths": map[string]any{
				"type":        "array",
				"required":    true,
				"description": "Folders inside the base path to scan. Leave a path empty to scan the whole base.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":      map[string]any{"type": "string", "required": true},
						"recursive": map[string]any{"type": "boolean"},
						"exclude_paths": map[string]any{
							"type":        "array",
							"description": "Folders inside this include path to skip recursively.",
							"items":       map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}
}

// --------------- IPC ---------------

func main() {
	log.SetOutput(os.Stderr)
	log.Println("Local folder source plugin started")

	var writeMu sync.Mutex
	for {
		var length uint32
		if err := binary.Read(os.Stdin, binary.BigEndian, &length); err != nil {
			if err == io.EOF {
				return
			}
			log.Printf("read frame length: %v", err)
			return
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(os.Stdin, payload); err != nil {
			log.Printf("read frame body: %v", err)
			return
		}

		var req Request
		if err := json.Unmarshal(payload, &req); err != nil {
			log.Printf("decode request: %v", err)
			continue
		}

		writeResponse(&writeMu, dispatch(req))
	}
}

func dispatch(req Request) Response {
	resp := Response{ID: req.ID}
	switch req.Method {
	case "plugin.init":
		resp.Result = map[string]any{"status": "ok"}
	case "plugin.info":
		resp.Result = pluginInfo()
	case "source.filesystem.list":
		config, err := decodeLocalConfig(req.Params)
		if err != nil {
			resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
			break
		}
		files, err := listFiles(config)
		if err != nil {
			resp.Error = &Error{Code: "SCAN_FAILED", Message: err.Error()}
			break
		}
		resp.Result = map[string]any{"files": files}
	case "source.filesystem.delete":
		result, errObj := handleSourceDelete(req.Params)
		if errObj != nil {
			resp.Error = errObj
			break
		}
		resp.Result = result
	case "source.browse":
		result, errObj := handleBrowse(req.Params)
		if errObj != nil {
			resp.Error = errObj
			break
		}
		resp.Result = result
	case "plugin.check_config":
		result, errObj := handleCheckConfig(req.Params)
		if errObj != nil {
			resp.Error = errObj
			break
		}
		resp.Result = result
	default:
		resp.Error = &Error{Code: "NOT_SUPPORTED", Message: fmt.Sprintf("method %q is not supported", req.Method)}
	}
	return resp
}

func writeResponse(writeMu *sync.Mutex, resp Response) {
	encoded, err := json.Marshal(resp)
	if err != nil {
		log.Printf("encode response: %v", err)
		return
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	if err := binary.Write(os.Stdout, binary.BigEndian, uint32(len(encoded))); err != nil {
		log.Printf("write frame length: %v", err)
		return
	}
	if _, err := os.Stdout.Write(encoded); err != nil {
		log.Printf("write frame body: %v", err)
	}
}
