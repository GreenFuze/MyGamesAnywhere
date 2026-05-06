package sourcescope

import (
	"fmt"
	"path"
	"strings"
)

type IncludePath struct {
	Path         string   `json:"path"`
	Recursive    bool     `json:"recursive"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`
}

func IsFilesystemBackedPlugin(pluginID string) bool {
	switch pluginID {
	case "game-source-smb", "game-source-google-drive":
		return true
	default:
		return false
	}
}

func NormalizeConfig(pluginID string, config map[string]any) map[string]any {
	if config == nil {
		config = map[string]any{}
	}
	if !IsFilesystemBackedPlugin(pluginID) {
		return cloneMap(config)
	}

	normalized := cloneMap(config)
	includes := ReadIncludePaths(pluginID, config)
	serialized := make([]map[string]any, 0, len(includes))
	for _, include := range includes {
		item := map[string]any{
			"path":      include.Path,
			"recursive": include.Recursive,
		}
		if len(include.ExcludePaths) > 0 {
			item["exclude_paths"] = include.ExcludePaths
		}
		serialized = append(serialized, item)
	}
	normalized["include_paths"] = serialized
	delete(normalized, "path")
	delete(normalized, "root_path")
	delete(normalized, "exclude_paths")
	return normalized
}

func ValidateConfig(pluginID string, config map[string]any) error {
	if !IsFilesystemBackedPlugin(pluginID) {
		return nil
	}
	includes := readIncludePaths(pluginID, config, false)
	if len(includes) == 0 {
		includes = []IncludePath{{Path: "", Recursive: true}}
	}

	for _, include := range includes {
		for _, exclude := range include.ExcludePaths {
			if exclude == "" {
				continue
			}
			if !IncludeContainsPath(include, exclude) {
				return fmt.Errorf("exclude path %q is outside include path %q", exclude, include.Path)
			}
		}
	}

	for _, legacyExclude := range parseStringPaths(config["exclude_paths"]) {
		if findOwningIncludeIndex(includes, legacyExclude) < 0 {
			return fmt.Errorf("exclude path %q is outside all include paths", legacyExclude)
		}
	}
	return nil
}

func ReadIncludePaths(pluginID string, config map[string]any) []IncludePath {
	return readIncludePaths(pluginID, config, true)
}

func readIncludePaths(pluginID string, config map[string]any, applyLegacyExcludes bool) []IncludePath {
	if !IsFilesystemBackedPlugin(pluginID) {
		return nil
	}
	if config == nil {
		return []IncludePath{{Path: "", Recursive: true}}
	}

	if raw, ok := config["include_paths"]; ok {
		if includes := parseIncludePaths(raw); len(includes) > 0 {
			includes = dedupeIncludePaths(includes)
			if applyLegacyExcludes {
				includes = applyLegacyExcludePaths(includes, config["exclude_paths"])
			}
			return includes
		}
	}

	legacyKey := legacyPathKey(pluginID)
	if legacyKey != "" {
		if raw, ok := config[legacyKey]; ok {
			if v, ok := raw.(string); ok {
				includes := []IncludePath{{Path: NormalizeLogicalPath(v), Recursive: true}}
				if applyLegacyExcludes {
					includes = applyLegacyExcludePaths(includes, config["exclude_paths"])
				}
				return includes
			}
		}
	}

	includes := []IncludePath{{Path: "", Recursive: true}}
	if applyLegacyExcludes {
		includes = applyLegacyExcludePaths(includes, config["exclude_paths"])
	}
	return includes
}

func NormalizeLogicalPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || value == "." || value == "/" || value == `\` {
		return ""
	}
	value = strings.ReplaceAll(value, `\`, "/")
	cleaned := path.Clean("/" + strings.Trim(value, "/"))
	if cleaned == "/" || cleaned == "." {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}

func ScopeContainsRootPath(rootPath string, includes []IncludePath) bool {
	normalizedRoot := NormalizeLogicalPath(rootPath)
	if len(includes) == 0 {
		return true
	}

	rootDepth := segmentDepth(normalizedRoot)
	for _, include := range includes {
		includePath := NormalizeLogicalPath(include.Path)
		if includePath == "" {
			if include.Recursive {
				return true
			}
			if rootDepth <= 1 {
				return true
			}
			continue
		}
		if normalizedRoot == includePath {
			return true
		}
		if include.Recursive {
			if strings.HasPrefix(normalizedRoot, includePath+"/") {
				return true
			}
			continue
		}
		if path.Dir(normalizedRoot) == includePath {
			return true
		}
	}

	return false
}

func IncludeContainsPath(include IncludePath, logicalPath string) bool {
	includePath := NormalizeLogicalPath(include.Path)
	normalizedPath := NormalizeLogicalPath(logicalPath)
	if includePath == "" {
		return true
	}
	return normalizedPath == includePath || strings.HasPrefix(normalizedPath, includePath+"/")
}

func legacyPathKey(pluginID string) string {
	switch pluginID {
	case "game-source-smb":
		return "path"
	case "game-source-google-drive":
		return "root_path"
	default:
		return ""
	}
}

func parseIncludePaths(raw any) []IncludePath {
	var items []map[string]any
	switch typed := raw.(type) {
	case []any:
		items = make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			items = append(items, entry)
		}
	case []map[string]any:
		items = typed
	default:
		return nil
	}
	includes := make([]IncludePath, 0, len(items))
	for _, entry := range items {
		pathValue, _ := entry["path"].(string)
		recursive, ok := entry["recursive"].(bool)
		if !ok {
			recursive = true
		}
		excludes := parseStringPaths(entry["exclude_paths"])
		includes = append(includes, IncludePath{
			Path:         NormalizeLogicalPath(pathValue),
			Recursive:    recursive,
			ExcludePaths: excludes,
		})
	}
	return includes
}

func dedupeIncludePaths(includes []IncludePath) []IncludePath {
	seen := make(map[string]int, len(includes))
	deduped := make([]IncludePath, 0, len(includes))
	for _, include := range includes {
		normalized := IncludePath{
			Path:         NormalizeLogicalPath(include.Path),
			Recursive:    include.Recursive,
			ExcludePaths: dedupeStringPaths(include.ExcludePaths),
		}
		key := normalized.Path + "|" + boolKey(normalized.Recursive)
		if existingIndex, ok := seen[key]; ok {
			deduped[existingIndex].ExcludePaths = dedupeStringPaths(append(deduped[existingIndex].ExcludePaths, normalized.ExcludePaths...))
			continue
		}
		seen[key] = len(deduped)
		deduped = append(deduped, normalized)
	}
	if len(deduped) == 0 {
		return []IncludePath{{Path: "", Recursive: true}}
	}
	return deduped
}

func applyLegacyExcludePaths(includes []IncludePath, raw any) []IncludePath {
	for _, exclude := range parseStringPaths(raw) {
		index := findOwningIncludeIndex(includes, exclude)
		if index < 0 {
			continue
		}
		includes[index].ExcludePaths = dedupeStringPaths(append(includes[index].ExcludePaths, exclude))
	}
	return includes
}

func findOwningIncludeIndex(includes []IncludePath, exclude string) int {
	bestIndex := -1
	bestDepth := -1
	for index, include := range includes {
		if !IncludeContainsPath(include, exclude) {
			continue
		}
		depth := segmentDepth(NormalizeLogicalPath(include.Path))
		if depth > bestDepth {
			bestIndex = index
			bestDepth = depth
		}
	}
	return bestIndex
}

func parseStringPaths(raw any) []string {
	var values []string
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok {
				values = append(values, NormalizeLogicalPath(s))
			}
		}
	case []string:
		for _, item := range typed {
			values = append(values, NormalizeLogicalPath(item))
		}
	}
	return dedupeStringPaths(values)
}

func dedupeStringPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	deduped := make([]string, 0, len(paths))
	for _, item := range paths {
		normalized := NormalizeLogicalPath(item)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		deduped = append(deduped, normalized)
	}
	return deduped
}

func segmentDepth(normalized string) int {
	if normalized == "" {
		return 0
	}
	return len(strings.Split(normalized, "/"))
}

func boolKey(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func cloneMap(src map[string]any) map[string]any {
	cloned := make(map[string]any, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}
