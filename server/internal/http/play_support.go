package http

import (
	"encoding/base64"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type GamePlayDTO struct {
	Available         bool                     `json:"available"`
	PlatformSupported bool                     `json:"platform_supported"`
	Options           []GameLaunchOptionDTO    `json:"options,omitempty"`
	LaunchSources     []GameLaunchSourceDTO    `json:"launch_sources,omitempty"`
	LaunchCandidates  []GameLaunchCandidateDTO `json:"launch_candidates,omitempty"`
}

type GameLaunchOptionDTO struct {
	Kind             string `json:"kind"`
	SourceGameID     string `json:"source_game_id"`
	SourceTitle      string `json:"source_title,omitempty"`
	Platform         string `json:"platform,omitempty"`
	PluginID         string `json:"plugin_id,omitempty"`
	IntegrationID    string `json:"integration_id,omitempty"`
	IntegrationLabel string `json:"integration_label,omitempty"`
	Launchable       bool   `json:"launchable"`
	FileID           string `json:"file_id,omitempty"`
	RootFileID       string `json:"root_file_id,omitempty"`
	Path             string `json:"path,omitempty"`
	FileKind         string `json:"file_kind,omitempty"`
	Size             int64  `json:"size,omitempty"`
	Profile          string `json:"profile,omitempty"`
	URL              string `json:"url,omitempty"`
}

type GameLaunchSourceDTO struct {
	SourceGameID string `json:"source_game_id"`
	Launchable   bool   `json:"launchable"`
	RootFileID   string `json:"root_file_id,omitempty"`
}

type SourceGamePlayDTO struct {
	Launchable bool   `json:"launchable"`
	RootFileID string `json:"root_file_id,omitempty"`
}

type GameLaunchCandidateDTO struct {
	SourceGameID string `json:"source_game_id"`
	FileID       string `json:"file_id"`
	Path         string `json:"path"`
	FileKind     string `json:"file_kind,omitempty"`
	Size         int64  `json:"size"`
}

func supportsBrowserPlayPlatform(platform core.Platform) bool {
	_, ok := core.BrowserPlayRuntimeForPlatform(platform)
	return ok
}

func supportsDirectSourceGame(sourceGame *core.SourceGame) bool {
	if sourceGame == nil {
		return false
	}
	rootPath := strings.TrimSpace(sourceGame.RootPath)
	return rootPath != "" && filepath.IsAbs(rootPath)
}

func supportsBrowserPlaySourceGame(sourceGame *core.SourceGame) bool {
	return supportsDirectSourceGame(sourceGame)
}

func supportsScummVMLaunchSource(files []core.GameFile) bool {
	if len(files) == 0 {
		return false
	}

	signatures := map[string]bool{
		"resource.map": true,
		"resource.000": true,
		"resource.001": true,
		"monster.sou":  true,
		"sky.dnr":      true,
		"sky.dsk":      true,
		"atlantis.000": true,
		"atlantis.001": true,
		"touche.dat":   true,
		"queen.1":      true,
		"queen.1c":     true,
		"toon.dat":     true,
		"comi.la0":     true,
	}

	byDir := map[string]map[string]bool{}
	for _, file := range files {
		dir := path.Clean(path.Dir(strings.ReplaceAll(file.Path, "\\", "/")))
		if dir == "." {
			dir = ""
		}
		names, ok := byDir[dir]
		if !ok {
			names = map[string]bool{}
			byDir[dir] = names
		}
		base := strings.ToLower(path.Base(strings.ReplaceAll(file.Path, "\\", "/")))
		names[base] = true
	}

	for _, names := range byDir {
		if hasScummVMKnownSignature(names) {
			return true
		}
		for name := range names {
			if signatures[name] {
				return true
			}
			if strings.HasPrefix(name, "monkey.") {
				return true
			}
		}
	}
	return false
}

func hasScummVMKnownSignature(names map[string]bool) bool {
	switch {
	case names["resource.map"] && (names["resource.000"] || names["resource.001"]):
		return true
	case names["sky.dnr"] && names["sky.dsk"]:
		return true
	case names["atlantis.000"] && names["atlantis.001"]:
		return true
	case names["queen.1"] && names["queen.1c"]:
		return true
	default:
		return false
	}
}

func encodeGameFileID(sourceGameID, path string) string {
	raw := sourceGameID + "\n" + filepath.ToSlash(path)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeGameFileID(fileID string) (string, string, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return "", "", fmt.Errorf("file_id is required")
	}
	raw, err := base64.RawURLEncoding.DecodeString(fileID)
	if err != nil {
		return "", "", fmt.Errorf("invalid file_id")
	}
	parts := strings.SplitN(string(raw), "\n", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid file_id")
	}
	sourceGameID := strings.TrimSpace(parts[0])
	path := strings.TrimSpace(parts[1])
	if sourceGameID == "" || path == "" {
		return "", "", fmt.Errorf("invalid file_id")
	}
	return sourceGameID, filepath.ToSlash(path), nil
}
