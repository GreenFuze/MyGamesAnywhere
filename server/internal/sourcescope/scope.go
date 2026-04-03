package sourcescope

import (
	"path"
	"strings"
)

type IncludePath struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
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
		serialized = append(serialized, map[string]any{
			"path":      include.Path,
			"recursive": include.Recursive,
		})
	}
	normalized["include_paths"] = serialized
	delete(normalized, "path")
	delete(normalized, "root_path")
	return normalized
}

func ReadIncludePaths(pluginID string, config map[string]any) []IncludePath {
	if !IsFilesystemBackedPlugin(pluginID) {
		return nil
	}
	if config == nil {
		return []IncludePath{{Path: "", Recursive: true}}
	}

	if raw, ok := config["include_paths"]; ok {
		if includes := parseIncludePaths(raw); len(includes) > 0 {
			return dedupeIncludePaths(includes)
		}
	}

	legacyKey := legacyPathKey(pluginID)
	if legacyKey != "" {
		if raw, ok := config[legacyKey]; ok {
			if v, ok := raw.(string); ok {
				return []IncludePath{{Path: NormalizeLogicalPath(v), Recursive: true}}
			}
		}
	}

	return []IncludePath{{Path: "", Recursive: true}}
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
		includes = append(includes, IncludePath{
			Path:      NormalizeLogicalPath(pathValue),
			Recursive: recursive,
		})
	}
	return includes
}

func dedupeIncludePaths(includes []IncludePath) []IncludePath {
	seen := make(map[string]bool, len(includes))
	deduped := make([]IncludePath, 0, len(includes))
	for _, include := range includes {
		normalized := IncludePath{
			Path:      NormalizeLogicalPath(include.Path),
			Recursive: include.Recursive,
		}
		key := normalized.Path + "|" + boolKey(normalized.Recursive)
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, normalized)
	}
	if len(deduped) == 0 {
		return []IncludePath{{Path: "", Recursive: true}}
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
