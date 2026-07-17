package http

import (
	"errors"
	"net/http"
	"strings"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/installprefs"
	"github.com/go-chi/chi/v5"
)

func (c *DeviceController) SetInstallPreferenceService(service *installprefs.Service) {
	c.installPreferences = service
}

func (c *DeviceController) GetProfileInstallPreference(w http.ResponseWriter, r *http.Request) {
	if c.installPreferences == nil {
		http.Error(w, "install preferences are unavailable", http.StatusServiceUnavailable)
		return
	}
	preference, err := c.installPreferences.GetProfile(r.Context(), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		writeInstallPreferenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preference)
}

func (c *DeviceController) SetProfileInstallPreference(w http.ResponseWriter, r *http.Request) {
	if c.installPreferences == nil {
		http.Error(w, "install preferences are unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		RootTemplate string `json:"root_template"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	preference, err := c.installPreferences.SetProfile(r.Context(), core.ProfileIDFromContext(r.Context()), body.RootTemplate)
	if err != nil {
		writeInstallPreferenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preference)
}

func (c *DeviceController) GetEndpointInstallPreference(w http.ResponseWriter, r *http.Request) {
	if c.installPreferences == nil {
		http.Error(w, "install preferences are unavailable", http.StatusServiceUnavailable)
		return
	}
	preference, err := c.installPreferences.GetEndpoint(r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		writeInstallPreferenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preference)
}

func (c *DeviceController) SetEndpointInstallPreference(w http.ResponseWriter, r *http.Request) {
	if c.installPreferences == nil {
		http.Error(w, "install preferences are unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		RootTemplate string `json:"root_template"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	preference, err := c.installPreferences.SetEndpoint(r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()), body.RootTemplate)
	if err != nil {
		writeInstallPreferenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preference)
}

func (c *DeviceController) resolveInstallRoot(r *http.Request, endpointID, requested string) (string, error) {
	if c.installPreferences == nil {
		if root := strings.TrimSpace(requested); root != "" {
			return root, nil
		}
		return devicev1.DefaultInstallRootTemplate, nil
	}
	return c.installPreferences.ResolveForInstall(r.Context(), endpointID, core.ProfileIDFromContext(r.Context()), requested)
}

func writeInstallPreferenceError(w http.ResponseWriter, err error) {
	if errors.Is(err, installprefs.ErrInvalidRootTemplate) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeDeviceError(w, err)
}
