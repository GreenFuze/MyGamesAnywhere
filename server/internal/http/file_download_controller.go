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

// DownloadFiles dispatches a non-installing prepared-copy transfer. Remote
// sources must already have completed the shared source-cache preparation job.
func (c *DeviceController) DownloadFiles(w http.ResponseWriter, r *http.Request) {
	if c.gameStore == nil || c.cacheSvc == nil || c.archiveTransfers == nil {
		http.Error(w, "file download is unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID := chi.URLParam(r, "id")
	gameID := chi.URLParam(r, "game_id")
	var body struct {
		SourceGameID    string `json:"source_game_id"`
		DestinationRoot string `json:"destination_root,omitempty"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	destinationRoot, err := c.resolveInstallRoot(r, endpointID, body.DestinationRoot)
	if err != nil {
		writeInstallPreferenceError(w, err)
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
	source := findSourceGame(game, strings.TrimSpace(body.SourceGameID))
	if source == nil {
		http.Error(w, "the selected source does not belong to this game", http.StatusBadRequest)
		return
	}
	files := make([]devicev1.FileDownloadItem, 0)
	for index := range source.Files {
		file := &source.Files[index]
		if file.IsDir {
			continue
		}
		relative, err := safePreparedRelativePath(source, file.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		localPath, err := c.resolvePreparedSourceFile(r, source, file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		hash, size, err := hashPreparedSourceFile(localPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		token, err := c.archiveTransfers.Create(localPath, filepath.Base(localPath))
		if err != nil {
			writeDeviceError(w, err)
			return
		}
		files = append(files, devicev1.FileDownloadItem{
			RelativePath: relative, SizeBytes: size, SHA256: hash,
			DownloadURL: "/api/device-transfers/file", DownloadToken: token,
		})
	}
	request := devicev1.FileDownloadRequest{
		SchemaVersion: devicev1.FileDownloadSchemaVersion,
		GameID:        game.ID, SourceGameID: source.ID, Title: game.Title,
		DestinationRoot: destinationRoot, DestinationName: safeInstallFolderName(game.Title),
		Files: files,
	}
	if err := request.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	payload, err := json.Marshal(request)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, core.ProfileIDFromContext(r.Context()), devicev1.CapabilityGameDownloadFiles, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) resolvePreparedSourceFile(r *http.Request, source *core.SourceGame, file *core.GameFile) (string, error) {
	if source != nil && filepath.IsAbs(strings.TrimSpace(source.RootPath)) {
		return resolveUnderGameRoot(source.RootPath, file.Path)
	}
	_, cached, localPath, err := c.cacheSvc.ResolveCachedFile(r.Context(), source.ID, devicev1.DeviceDownloadSourceProfile, file.Path)
	if err != nil {
		return "", err
	}
	if cached == nil || strings.TrimSpace(localPath) == "" {
		return "", errors.New("source files are not prepared yet; prepare the download and try again")
	}
	return localPath, nil
}

func safePreparedRelativePath(source *core.SourceGame, raw string) (string, error) {
	value := strings.TrimSpace(strings.ReplaceAll(raw, `\`, "/"))
	if value == "" {
		return "", errors.New("source file path is empty")
	}
	if filepath.IsAbs(filepath.FromSlash(value)) && source != nil && filepath.IsAbs(source.RootPath) {
		relative, err := filepath.Rel(source.RootPath, filepath.FromSlash(value))
		if err != nil {
			return "", err
		}
		value = filepath.ToSlash(relative)
	}
	value = strings.TrimPrefix(value, "./")
	value = strings.TrimPrefix(value, "/")
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == "" || cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) || strings.Contains(cleaned, ":") {
		return "", fmt.Errorf("source file path %q cannot be safely downloaded", raw)
	}
	return cleaned, nil
}

func hashPreparedSourceFile(filePath string) (string, uint64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("open prepared source file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", 0, errors.New("prepared source is not a regular file")
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, fmt.Errorf("hash prepared source file: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), uint64(size), nil
}
