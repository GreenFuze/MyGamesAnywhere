package http

import (
	"encoding/json"
	"errors"
	"net/http"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/emulation"
	"github.com/go-chi/chi/v5"
)

func (c *DeviceController) GetEndpointEmulators(w http.ResponseWriter, r *http.Request) {
	if c.emulators == nil {
		http.Error(w, "emulator settings are unavailable", http.StatusServiceUnavailable)
		return
	}
	configuration, err := c.emulators.Get(r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		writeEmulationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configuration)
}

func (c *DeviceController) SetEndpointEmulatorCore(w http.ResponseWriter, r *http.Request) {
	if c.emulators == nil {
		http.Error(w, "emulator settings are unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		CoreID string `json:"core_id"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	configuration, err := c.emulators.SetCoreDefault(
		r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()),
		core.Platform(chi.URLParam(r, "platform")), chi.URLParam(r, "emulator_id"), body.CoreID,
	)
	if err != nil {
		writeEmulationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configuration)
}

func (c *DeviceController) SetupEndpointEmulator(w http.ResponseWriter, r *http.Request) {
	if c.emulators == nil {
		http.Error(w, "emulator settings are unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	endpointID := chi.URLParam(r, "id")
	profileID := core.ProfileIDFromContext(r.Context())
	request, err := c.emulators.PrepareSetup(r.Context(), endpointID, profileID, chi.URLParam(r, "emulator_id"), body.Action)
	if err != nil {
		writeEmulationError(w, err)
		return
	}
	payload, err := json.Marshal(request)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, profileID, devicev1.CapabilityEmulatorSetup, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) SetEndpointEmulatorDefault(w http.ResponseWriter, r *http.Request) {
	if c.emulators == nil {
		http.Error(w, "emulator settings are unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		EmulatorID string `json:"emulator_id"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	configuration, err := c.emulators.SetDefault(
		r.Context(),
		chi.URLParam(r, "id"),
		core.ProfileIDFromContext(r.Context()),
		core.Platform(chi.URLParam(r, "platform")),
		body.EmulatorID,
	)
	if err != nil {
		writeEmulationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configuration)
}

func writeEmulationError(w http.ResponseWriter, err error) {
	if errors.Is(err, emulation.ErrInvalidPreference) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if errors.Is(err, emulation.ErrSetupUnavailable) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeDeviceError(w, err)
}
