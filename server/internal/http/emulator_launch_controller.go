package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

func (c *DeviceController) LaunchEmulatorGame(w http.ResponseWriter, r *http.Request) {
	if c.emulators == nil || c.gameStore == nil || c.archiveTransfers == nil {
		http.Error(w, "local emulator play is unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID := chi.URLParam(r, "id")
	gameID := chi.URLParam(r, "game_id")
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body struct {
		EmulatorID string `json:"emulator_id"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	game, err := c.gameStore.GetCanonicalGameByID(r.Context(), gameID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	source := findEmulatorSourceGame(game, sourceGameID)
	if source == nil || source.Status != "found" || source.GroupKind != core.GroupKindSelfContained {
		http.Error(w, "the selected game source is not ready for local emulator play", http.StatusConflict)
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	emulatorID := strings.ToLower(strings.TrimSpace(body.EmulatorID))
	option, err := c.emulators.RequireReady(r.Context(), endpointID, profileID, game.Platform, emulatorID)
	if err != nil {
		writeEmulationError(w, err)
		return
	}
	artifacts, err := c.createEmulatorArtifacts(source)
	if err != nil {
		c.logger.Warn("prepare emulator content", "game_id", gameID, "source_game_id", sourceGameID, "error", err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	contentPath, err := selectEmulatorContentPath(emulatorID, artifacts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	routeFingerprint, err := devicev1.EmulatorRouteFingerprint(artifacts)
	if err != nil {
		http.Error(w, "MGA could not identify this exact emulator copy", http.StatusConflict)
		return
	}
	request := devicev1.EmulatorLaunchRequest{
		GameID: gameID, SourceGameID: sourceGameID, Title: game.Title, Platform: string(game.Platform),
		EmulatorID: emulatorID, CoreID: option.ResolvedCore, ContentPath: contentPath, Artifacts: artifacts, RouteFingerprint: routeFingerprint,
	}
	payload, err := json.Marshal(request)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, profileID, devicev1.CapabilityGameLaunchEmulator, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func selectEmulatorContentPath(emulatorID string, artifacts []devicev1.EmulatorContentArtifact) (string, error) {
	if emulatorID != "retroarch" {
		return "", nil
	}
	if len(artifacts) == 1 {
		return artifacts[0].Path, nil
	}
	for _, extensions := range [][]string{{".m3u"}, {".cue"}, {".ccd"}, {".chd"}, {".pbp"}, {".zip"}, {".7z"}} {
		match := ""
		for _, artifact := range artifacts {
			extension := strings.ToLower(path.Ext(artifact.Path))
			for _, candidate := range extensions {
				if extension != candidate {
					continue
				}
				if match != "" {
					return "", errors.New("choose a single launchable image or playlist for this game source")
				}
				match = artifact.Path
			}
		}
		if match != "" {
			return match, nil
		}
	}
	return "", errors.New("MGA could not safely choose which downloaded file RetroArch should open")
}

func findEmulatorSourceGame(game *core.CanonicalGame, sourceGameID string) *core.SourceGame {
	if game == nil {
		return nil
	}
	for _, source := range game.SourceGames {
		if source != nil && source.ID == sourceGameID {
			return source
		}
	}
	return nil
}

func (c *DeviceController) createEmulatorArtifacts(source *core.SourceGame) ([]devicev1.EmulatorContentArtifact, error) {
	if !c.supportsEmulatorContentSource(source) {
		return nil, errors.New("this source must be downloaded to the MGA Server before local emulator play")
	}
	if len(source.Files) == 0 || len(source.Files) > 4096 {
		return nil, errors.New("this source has an unsupported number of game files")
	}
	artifacts := make([]devicev1.EmulatorContentArtifact, 0, len(source.Files))
	seen := make(map[string]bool, len(source.Files))
	for _, gameFile := range source.Files {
		if gameFile.IsDir {
			continue
		}
		fullPath, relative, err := c.resolveEmulatorContentFile(source, gameFile)
		if err != nil {
			return nil, err
		}
		if relative == "." || relative == "" || path.IsAbs(relative) || strings.HasPrefix(relative, "../") || strings.Contains(relative, ":") {
			return nil, fmt.Errorf("unsafe source file path %q", gameFile.Path)
		}
		key := strings.ToLower(relative)
		if seen[key] {
			return nil, fmt.Errorf("duplicate source file path %q", relative)
		}
		seen[key] = true
		info, err := os.Stat(fullPath)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("source file is unavailable: %s", relative)
		}
		digest, err := fileSHA256(fullPath)
		if err != nil {
			return nil, err
		}
		token, err := c.archiveTransfers.CreateEmulatorContent(fullPath, filepath.Base(relative))
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, devicev1.EmulatorContentArtifact{
			Path: relative, SizeBytes: uint64(info.Size()), SHA256: digest,
			DownloadURL: "/api/device-transfers/content", DownloadToken: token,
		})
	}
	if len(artifacts) == 0 {
		return nil, errors.New("this source has no downloadable game files")
	}
	return artifacts, nil
}

func (c *DeviceController) supportsEmulatorContentSource(source *core.SourceGame) bool {
	return supportsEmulatorContentSource(source, c.googleDriveRoot)
}

func supportsEmulatorContentSource(source *core.SourceGame, googleDriveRoot string) bool {
	if supportsDirectSourceGame(source) {
		return true
	}
	return source != nil &&
		source.PluginID == "game-source-google-drive" &&
		strings.TrimSpace(source.RootPath) != "" &&
		filepath.IsAbs(strings.TrimSpace(googleDriveRoot))
}

func (c *DeviceController) resolveEmulatorContentFile(source *core.SourceGame, gameFile core.GameFile) (string, string, error) {
	if source == nil {
		return "", "", errors.New("game source is required")
	}
	filePath := path.Clean(strings.ReplaceAll(strings.TrimSpace(gameFile.Path), `\`, "/"))
	if supportsDirectSourceGame(source) {
		fullPath, err := resolveUnderGameRoot(source.RootPath, filePath)
		return fullPath, filePath, err
	}
	if !c.supportsEmulatorContentSource(source) {
		return "", "", errors.New("this source is not directly readable by the MGA Server")
	}
	relative, err := relativeSourceFilePath(source.RootPath, filePath)
	if err != nil {
		return "", "", err
	}
	fullPath, err := resolveUnderGameRoot(c.googleDriveRoot, filePath)
	return fullPath, relative, err
}

func relativeSourceFilePath(sourceRoot, filePath string) (string, error) {
	root := strings.TrimSuffix(path.Clean(strings.ReplaceAll(strings.TrimSpace(sourceRoot), `\`, "/")), "/")
	file := path.Clean(strings.ReplaceAll(strings.TrimSpace(filePath), `\`, "/"))
	prefix := root + "/"
	if root == "." || root == "" || len(file) <= len(prefix) || !strings.EqualFold(file[:len(prefix)], prefix) {
		return "", fmt.Errorf("source file is outside the game root: %s", filePath)
	}
	return file[len(prefix):], nil
}

func fileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
