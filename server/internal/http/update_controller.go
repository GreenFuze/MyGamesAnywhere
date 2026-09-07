package http

import (
	"encoding/json"
	"net/http"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type UpdateController struct {
	updateSvc core.UpdateService
	logger    core.Logger
}

func NewUpdateController(updateSvc core.UpdateService, logger core.Logger) *UpdateController {
	return &UpdateController{updateSvc: updateSvc, logger: logger}
}

func (c *UpdateController) Status(w http.ResponseWriter, r *http.Request) {
	status, err := c.updateSvc.Status(r.Context())
	if err != nil {
		c.logger.Error("update status failed", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeUpdateJSON(w, status)
}

func (c *UpdateController) Check(w http.ResponseWriter, r *http.Request) {
	status, err := c.updateSvc.Check(r.Context())
	if err != nil {
		c.logger.Error("update check failed", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeUpdateJSON(w, status)
}

// Download starts the transfer and answers immediately.
//
// It does not wait for the bytes. This request is bounded by the router's
// timeout and the installer is well over 100 MB, so waiting meant the update
// only installed when the network was fast enough — on 2026-09-07 a server
// reached 85.6% and was cut off. Progress is reported by Status, which the
// console already polls, so the caller loses nothing by being answered early.
func (c *UpdateController) Download(w http.ResponseWriter, r *http.Request) {
	status, err := c.updateSvc.StartDownload(r.Context())
	if err != nil {
		c.logger.Error("update download failed to start", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeUpdateJSON(w, status)
}

func (c *UpdateController) Apply(w http.ResponseWriter, r *http.Request) {
	result, err := c.updateSvc.Apply(r.Context())
	if err != nil {
		c.logger.Error("update apply failed", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeUpdateJSON(w, result)
}

func writeUpdateJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
