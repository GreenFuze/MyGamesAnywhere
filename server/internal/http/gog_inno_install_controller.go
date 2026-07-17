package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

func (c *DeviceController) InstallGogInno(w http.ResponseWriter, r *http.Request) {
	if c.gameStore == nil || c.integrationRepo == nil || c.archiveTransfers == nil {
		http.Error(w, "GOG installer installation is unavailable", http.StatusServiceUnavailable)
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
	source, installer, companions, selectErr := findSupportedGogInnoPackage(game, strings.TrimSpace(body.SourceGameID))
	if selectErr != nil {
		http.Error(w, selectErr.Error(), http.StatusBadRequest)
		return
	}
	if source == nil || installer == nil {
		http.Error(w, "the selected source does not contain exactly one supported GOG Inno Setup package", http.StatusBadRequest)
		return
	}
	installerPath, err := c.resolveArchiveSource(r.Context(), source, installer)
	if err != nil {
		c.logger.Warn("resolve gog inno installer source", "game_id", gameID, "source_game_id", source.ID, "error", err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	info, err := os.Stat(installerPath)
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "installer is not available on the MGA Server", http.StatusConflict)
		return
	}
	installerName := strings.TrimSpace(installer.FileName)
	if installerName == "" {
		installerName = filepath.Base(installer.Path)
	}
	token, err := c.archiveTransfers.Create(installerPath, installerName)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	request := devicev1.GogInnoInstallRequest{
		GameID: game.ID, SourceGameID: source.ID, Title: game.Title,
		DestinationRoot: destinationRoot, DestinationName: safeInstallFolderName(game.Title),
		Installer: devicev1.PackageTransferDescriptor{
			FileName: installerName, Role: devicev1.PackageTransferRoleInstaller,
			SizeBytes: uint64(info.Size()), DownloadURL: "/api/device-transfers/archive", DownloadToken: token,
		},
	}
	for _, companion := range companions {
		companionName := strings.TrimSpace(companion.FileName)
		if companionName == "" {
			companionName = filepath.Base(companion.Path)
		}
		companionPath, resolveErr := c.resolveArchiveSource(r.Context(), source, companion)
		if resolveErr != nil {
			http.Error(w, resolveErr.Error(), http.StatusConflict)
			return
		}
		companionInfo, statErr := os.Stat(companionPath)
		if statErr != nil || !companionInfo.Mode().IsRegular() {
			http.Error(w, "companion package file is not available on the MGA Server", http.StatusConflict)
			return
		}
		companionToken, createErr := c.archiveTransfers.Create(companionPath, companionName)
		if createErr != nil {
			writeDeviceError(w, createErr)
			return
		}
		request.Companions = append(request.Companions, devicev1.PackageTransferDescriptor{
			FileName: companionName, Role: devicev1.PackageTransferRoleCompanion,
			SizeBytes: uint64(companionInfo.Size()), DownloadURL: "/api/device-transfers/archive", DownloadToken: companionToken,
		})
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
	command, err := c.service.DispatchCommand(r.Context(), endpointID, core.ProfileIDFromContext(r.Context()), devicev1.CapabilityGameInstallGogInno, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) CleanupFailedGogInno(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	if len(body) != 0 {
		http.Error(w, "cleanup request body must be empty", http.StatusBadRequest)
		return
	}
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
	if installation.InstallKind != devicev1.InstallKindGogInno || strings.TrimSpace(installation.CleanupMarkerID) == "" {
		http.Error(w, "this failed installation has no verified cleanup marker", http.StatusConflict)
		return
	}
	payload, err := json.Marshal(devicev1.GogInnoFailedCleanupRequest{
		GameID: gameID, SourceGameID: sourceGameID, InstallRoot: installation.InstallRoot, InstallPath: installation.InstallPath,
		InstallerFamily: installation.InstallerFamily, CleanupMarkerID: installation.CleanupMarkerID,
		PrimarySHA256: installation.ArchiveSHA256, UninstallTarget: installation.UninstallTarget,
	})
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	if _, err := c.service.TransitionInstallationFailure(r.Context(), endpointID, gameID, sourceGameID, profileID, "cleanup_started", "Cleanup requested"); err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, profileID, devicev1.CapabilityGameCleanupGogInnoFailed, payload)
	if err != nil {
		_, _ = c.service.TransitionInstallationFailure(r.Context(), endpointID, gameID, sourceGameID, profileID, "cleanup_failed", err.Error())
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) IgnoreFailedGogInno(w http.ResponseWriter, r *http.Request) {
	c.transitionFailedGogInno(w, r, "ignore")
}

func (c *DeviceController) ReopenFailedGogInno(w http.ResponseWriter, r *http.Request) {
	c.transitionFailedGogInno(w, r, "reopen")
}

func (c *DeviceController) transitionFailedGogInno(w http.ResponseWriter, r *http.Request, action string) {
	var body map[string]json.RawMessage
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	if len(body) != 0 {
		http.Error(w, "request body must be empty", http.StatusBadRequest)
		return
	}
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	installation, err := c.service.TransitionInstallationFailure(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "game_id"), sourceGameID,
		core.ProfileIDFromContext(r.Context()), action, "")
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, installation)
}

func findSupportedGogInnoPackage(game *core.CanonicalGame, sourceGameID string) (*core.SourceGame, *core.GameFile, []*core.GameFile, error) {
	if game == nil || sourceGameID == "" {
		return nil, nil, nil, nil
	}
	for _, source := range game.SourceGames {
		if source == nil || source.ID != sourceGameID {
			continue
		}
		if source.Kind != core.GameKindBaseGame {
			return nil, nil, nil, errors.New("This installer isn’t supported yet.")
		}
		var installer *core.GameFile
		var companions []*core.GameFile
		var otherExecutables int
		for index := range source.Files {
			file := &source.Files[index]
			if file.IsDir {
				continue
			}
			base := filepath.Base(file.Path)
			switch {
			case devicev1.IsGogInnoSetupFileName(base):
				if installer != nil {
					return source, nil, nil, nil
				}
				installer = file
			case devicev1.IsGogInnoCompanionFileName(base):
				companions = append(companions, file)
			case strings.EqualFold(filepath.Ext(base), ".exe"):
				otherExecutables++
			}
		}
		if installer == nil || otherExecutables > 0 {
			return source, nil, nil, nil
		}
		if len(companions) > devicev1.MaxGogInnoCompanions {
			return source, nil, nil, nil
		}
		stem := devicev1.GogInnoSetupStem(filepath.Base(installer.Path))
		matched := make([]*core.GameFile, 0, len(companions))
		seen := map[string]struct{}{}
		for _, companion := range companions {
			base := filepath.Base(companion.Path)
			if !strings.EqualFold(devicev1.GogInnoCompanionStem(base), stem) {
				return source, nil, nil, nil
			}
			key := strings.ToLower(base)
			if _, exists := seen[key]; exists {
				return source, nil, nil, nil
			}
			seen[key] = struct{}{}
			matched = append(matched, companion)
		}
		return source, installer, matched, nil
	}
	return nil, nil, nil, nil
}
