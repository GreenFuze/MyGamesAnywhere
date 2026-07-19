package http

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
	"github.com/go-chi/chi/v5"
)

const archiveTransferLifetime = 12 * time.Hour
const emulatorContentTransferLifetime = 10 * time.Minute

type archiveTransfer struct {
	Path      string
	Name      string
	ExpiresAt time.Time
}

type archiveTransferRegistry struct {
	mu    sync.Mutex
	items map[string]archiveTransfer
	now   func() time.Time
}

func newArchiveTransferRegistry() *archiveTransferRegistry {
	return &archiveTransferRegistry{items: map[string]archiveTransfer{}, now: time.Now}
}

func (r *archiveTransferRegistry) Create(path, name string) (string, error) {
	return r.create(path, name, archiveTransferLifetime)
}

func (r *archiveTransferRegistry) CreateEmulatorContent(path, name string) (string, error) {
	return r.create(path, name, emulatorContentTransferLifetime)
}

func (r *archiveTransferRegistry) create(path, name string, lifetime time.Duration) (string, error) {
	if r == nil || strings.TrimSpace(path) == "" {
		return "", errors.New("archive transfer registry is unavailable")
	}
	if lifetime <= 0 {
		return "", errors.New("transfer lifetime must be positive")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate archive transfer token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	for key, item := range r.items {
		if !now.Before(item.ExpiresAt) {
			delete(r.items, key)
		}
	}
	r.items[token] = archiveTransfer{Path: path, Name: name, ExpiresAt: now.Add(lifetime)}
	return token, nil
}

func (r *archiveTransferRegistry) Get(token string) (archiveTransfer, bool) {
	if r == nil || strings.TrimSpace(token) == "" {
		return archiveTransfer{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[token]
	if !ok || !r.now().Before(item.ExpiresAt) {
		delete(r.items, token)
		return archiveTransfer{}, false
	}
	return item, true
}

func (c *DeviceController) InstallArchive(w http.ResponseWriter, r *http.Request) {
	if c.gameStore == nil || c.integrationRepo == nil || c.archiveTransfers == nil {
		http.Error(w, "archive installation is unavailable", http.StatusServiceUnavailable)
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
	source, archive := findSupportedArchive(game, strings.TrimSpace(body.SourceGameID))
	if source == nil || archive == nil {
		http.Error(w, "the selected source does not contain exactly one supported ZIP, 7z, or RAR archive", http.StatusBadRequest)
		return
	}
	archiveFormat, _ := supportedArchiveFormat(archive.Path)
	archivePath, err := c.resolveArchiveSource(r.Context(), source, archive)
	if err != nil {
		c.logger.Warn("resolve archive install source", "game_id", gameID, "source_game_id", source.ID, "error", err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	info, err := os.Stat(archivePath)
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "archive is not available on the MGA Server", http.StatusConflict)
		return
	}
	token, err := c.archiveTransfers.Create(archivePath, archive.FileName)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	request := devicev1.ArchiveInstallRequest{
		GameID: game.ID, SourceGameID: source.ID, Title: game.Title, ArchiveName: archive.FileName,
		ArchiveFormat: archiveFormat, ArchiveSize: uint64(info.Size()),
		DownloadURL:     "/api/device-transfers/archive",
		DownloadToken:   token,
		DestinationRoot: destinationRoot, DestinationName: safeInstallFolderName(game.Title),
	}
	payload, err := json.Marshal(request)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, core.ProfileIDFromContext(r.Context()), devicev1.CapabilityGameInstallArchive, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) UseExistingInstallation(w http.ResponseWriter, r *http.Request) {
	if c.gameStore == nil {
		http.Error(w, "game installation reuse is unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID := chi.URLParam(r, "id")
	gameID := chi.URLParam(r, "game_id")
	var body struct {
		LocalInstallationID string `json:"local_installation_id"`
		SourceGameID        string `json:"source_game_id"`
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
	sourceID := strings.TrimSpace(body.SourceGameID)
	foundSource := false
	for _, source := range game.SourceGames {
		if source != nil && source.ID == sourceID {
			foundSource = true
			break
		}
	}
	if !foundSource {
		http.Error(w, "the selected library source does not belong to this game", http.StatusBadRequest)
		return
	}
	request := devicev1.UseExistingInstallationRequest{
		LocalInstallationID: strings.TrimSpace(body.LocalInstallationID), GameID: game.ID, SourceGameID: sourceID, Title: game.Title,
	}
	if err := request.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload, err := json.Marshal(request)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, core.ProfileIDFromContext(r.Context()), devicev1.CapabilityGameUseExisting, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) UninstallGame(w http.ResponseWriter, r *http.Request) {
	endpointID := chi.URLParam(r, "id")
	gameID := chi.URLParam(r, "game_id")
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	endpoints, err := c.service.ListEndpoints(r.Context(), profileID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	var installation *devices.GameInstallation
	for endpointIndex := range endpoints {
		if endpoints[endpointIndex].ID != endpointID {
			continue
		}
		for index := range endpoints[endpointIndex].Installations {
			candidate := &endpoints[endpointIndex].Installations[index]
			if candidate.GameID == gameID && candidate.SourceGameID == sourceGameID {
				installation = candidate
				break
			}
		}
	}
	if installation == nil {
		http.NotFound(w, r)
		return
	}
	if installation.AuthorityMode == devicev1.InstallationAuthorityShared || installation.InstallKind == devicev1.InstallKindSharedExisting {
		http.Error(w, "this server has launch-only access and cannot uninstall the existing game", http.StatusConflict)
		return
	}
	var (
		payload []byte
		name    string
	)
	switch installation.InstallKind {
	case devicev1.InstallKindGogInno:
		name = devicev1.CapabilityGameUninstallGogInno
		payload, err = json.Marshal(devicev1.GogInnoUninstallRequest{
			GameID: gameID, SourceGameID: sourceGameID, InstallPath: installation.InstallPath,
			InstallerFamily: devicev1.GogInnoInstallerFamily, UninstallTarget: installation.UninstallTarget,
		})
	default:
		name = devicev1.CapabilityGameUninstall
		payload, err = json.Marshal(devicev1.GameUninstallRequest{GameID: gameID, SourceGameID: sourceGameID, InstallPath: installation.InstallPath})
	}
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, profileID, name, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) LaunchGame(w http.ResponseWriter, r *http.Request) {
	endpointID := chi.URLParam(r, "id")
	gameID := chi.URLParam(r, "game_id")
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	installation, err := c.findInstallation(r.Context(), endpointID, gameID, sourceGameID, profileID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	if installation.InstallState != devicev1.InstallStateInstalled {
		http.Error(w, "this installation needs attention on the device before it can be played", http.StatusConflict)
		return
	}
	if strings.TrimSpace(installation.LaunchTarget) == "" {
		http.Error(w, "choose an executable before playing on this device", http.StatusConflict)
		return
	}
	payload, err := json.Marshal(devicev1.GameLaunchRequest{
		GameID: gameID, SourceGameID: sourceGameID, InstallPath: installation.InstallPath, LaunchTarget: installation.LaunchTarget,
		LocalInstallationID: installation.LocalInstallationID,
	})
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, profileID, devicev1.CapabilityGameLaunch, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) SetLaunchTarget(w http.ResponseWriter, r *http.Request) {
	endpointID := chi.URLParam(r, "id")
	gameID := chi.URLParam(r, "game_id")
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body struct {
		LaunchTarget string `json:"launch_target"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	if err := c.service.SetInstallationLaunchTarget(r.Context(), endpointID, gameID, sourceGameID, core.ProfileIDFromContext(r.Context()), body.LaunchTarget); err != nil {
		writeDeviceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *DeviceController) findInstallation(ctx context.Context, endpointID, gameID, sourceGameID, profileID string) (*devices.GameInstallation, error) {
	endpoints, err := c.service.ListEndpoints(ctx, profileID)
	if err != nil {
		return nil, err
	}
	for endpointIndex := range endpoints {
		if endpoints[endpointIndex].ID != endpointID {
			continue
		}
		for installationIndex := range endpoints[endpointIndex].Installations {
			installation := &endpoints[endpointIndex].Installations[installationIndex]
			if installation.GameID == gameID && installation.SourceGameID == sourceGameID {
				return installation, nil
			}
		}
	}
	return nil, devices.ErrInstallationNotFound
}

func (c *DeviceController) ServeArchiveTransfer(w http.ResponseWriter, r *http.Request) {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	if token == authorization {
		http.NotFound(w, r)
		return
	}
	transfer, ok := c.archiveTransfers.Get(token)
	if !ok {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(transfer.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	contentType := mime.TypeByExtension(filepath.Ext(transfer.Name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": transfer.Name}))
	http.ServeContent(w, r, transfer.Name, info.ModTime(), file)
}

func findSupportedArchive(game *core.CanonicalGame, sourceGameID string) (*core.SourceGame, *core.GameFile) {
	if game == nil || sourceGameID == "" {
		return nil, nil
	}
	for _, source := range game.SourceGames {
		if source == nil || source.ID != sourceGameID {
			continue
		}
		var archive *core.GameFile
		for index := range source.Files {
			file := &source.Files[index]
			if _, supported := supportedArchiveFormat(file.Path); !file.IsDir && supported {
				if archive != nil {
					return source, nil
				}
				archive = file
			}
		}
		return source, archive
	}
	return nil, nil
}

func supportedArchiveFormat(name string) (string, bool) {
	format := devicev1.NormalizeArchiveFormat(filepath.Ext(strings.TrimSpace(name)))
	if err := devicev1.ValidateArchiveFormat(format); err != nil {
		return "", false
	}
	return format, true
}

func (c *DeviceController) resolveArchiveSource(ctx context.Context, source *core.SourceGame, file *core.GameFile) (string, error) {
	if supportsDirectSourceGame(source) {
		return resolveUnderGameRoot(source.RootPath, file.Path)
	}
	if source.PluginID != "game-source-google-drive" {
		return "", errors.New("this archive source is not directly readable by the MGA Server")
	}
	root := strings.TrimSpace(c.googleDriveRoot)
	if root == "" {
		return "", errors.New("set MGA_GOOGLE_DRIVE_DESKTOP_ROOT to the local Google Drive for desktop root before installing this archive")
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("MGA_GOOGLE_DRIVE_DESKTOP_ROOT must be absolute")
	}
	full, err := resolveUnderGameRoot(root, file.Path)
	if err != nil {
		return "", err
	}
	_ = ctx
	return full, nil
}

var unsafeInstallFolderCharacters = regexp.MustCompile(`[\\/:*?"<>|]+`)

func safeInstallFolderName(title string) string {
	name := strings.TrimSpace(unsafeInstallFolderCharacters.ReplaceAllString(title, " "))
	name = strings.TrimRight(name, ". ")
	if name == "" {
		name = "Game"
	}
	if len(name) > 100 {
		name = strings.TrimRight(name[:100], ". ")
	}
	return name
}
