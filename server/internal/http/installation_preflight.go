package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

type installationPreflightBody struct {
	SourceGameID    string `json:"source_game_id"`
	DestinationRoot string `json:"destination_root,omitempty"`
	InstallKind     string `json:"install_kind"`
}

// PreflightInstallation resolves server-owned package facts and asks the
// selected MGA Client endpoint to evaluate device-owned facts read-only.
func (c *DeviceController) PreflightInstallation(w http.ResponseWriter, r *http.Request) {
	if c.gameStore == nil || c.integrationRepo == nil {
		http.Error(w, "installation checks are unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID := chi.URLParam(r, "id")
	gameID := chi.URLParam(r, "game_id")
	var body installationPreflightBody
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
	sourceID := strings.TrimSpace(body.SourceGameID)
	request := devicev1.InstallationPreflightRequest{
		SchemaVersion: devicev1.InstallationPreflightSchemaVersion,
		GameID:        game.ID, SourceGameID: sourceID, DestinationRoot: destinationRoot,
	}
	switch strings.TrimSpace(body.InstallKind) {
	case string(devicev1.InstallKindManagedArchive):
		source, archive := findSupportedArchive(game, sourceID)
		if source == nil || archive == nil {
			http.Error(w, "the selected source is not a supported archive package", http.StatusBadRequest)
			return
		}
		size, statErr := c.resolvedPackageSize(r, source, archive)
		if statErr != nil {
			http.Error(w, statErr.Error(), http.StatusConflict)
			return
		}
		request.Category = devicev1.InstallationCategoryManagedArchive
		request.RequiredStorageBytes = size
	case string(devicev1.InstallKindGogInno):
		source, installer, companions, selectErr := findSupportedGogInnoPackage(game, sourceID)
		if selectErr != nil || source == nil || installer == nil {
			if selectErr == nil {
				selectErr = errors.New("the selected source is not a supported GOG installer package")
			}
			http.Error(w, selectErr.Error(), http.StatusBadRequest)
			return
		}
		size, statErr := c.resolvedPackageSize(r, source, installer)
		if statErr != nil {
			http.Error(w, statErr.Error(), http.StatusConflict)
			return
		}
		for _, companion := range companions {
			companionSize, companionErr := c.resolvedPackageSize(r, source, companion)
			if companionErr != nil {
				http.Error(w, companionErr.Error(), http.StatusConflict)
				return
			}
			size += companionSize
		}
		request.Category = devicev1.InstallationCategoryNativeInstaller
		request.RequiredStorageBytes = size
	default:
		http.Error(w, "unsupported installation kind", http.StatusBadRequest)
		return
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
	command, err := c.service.DispatchCommand(r.Context(), endpointID, core.ProfileIDFromContext(r.Context()), devicev1.CapabilityInstallationPreflight, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) resolvedPackageSize(r *http.Request, source *core.SourceGame, file *core.GameFile) (uint64, error) {
	path, err := c.resolveArchiveSource(r.Context(), source, file)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0, errors.New("package file is not available on MGA Server")
	}
	return uint64(info.Size()), nil
}
