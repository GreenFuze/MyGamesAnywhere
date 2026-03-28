package http

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type GamePlayDTO struct {
	Available          bool                    `json:"available"`
	PlatformSupported  bool                    `json:"platform_supported"`
	LaunchSources      []GameLaunchSourceDTO   `json:"launch_sources,omitempty"`
	LaunchCandidates   []GameLaunchCandidateDTO `json:"launch_candidates,omitempty"`
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

var browserPlayablePlatforms = map[core.Platform]bool{
	core.PlatformNES:              true,
	core.PlatformSNES:             true,
	core.PlatformGB:               true,
	core.PlatformGBC:              true,
	core.PlatformGBA:              true,
	core.PlatformGenesis:          true,
	core.PlatformSegaMasterSystem: true,
	core.PlatformGameGear:         true,
	core.PlatformSegaCD:           true,
	core.PlatformSega32X:          true,
	core.PlatformPS1:              true,
	core.PlatformArcade:           true,
	core.PlatformMSDOS:            true,
	core.PlatformScummVM:          true,
}

func supportsBrowserPlayPlatform(platform core.Platform) bool {
	return browserPlayablePlatforms[platform]
}

func allowsRootlessLaunch(platform core.Platform) bool {
	return platform == core.PlatformScummVM
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
