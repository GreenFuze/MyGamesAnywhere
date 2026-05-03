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

func (c *UpdateController) Download(w http.ResponseWriter, r *http.Request) {
	result, err := c.updateSvc.Download(r.Context())
	if err != nil {
		c.logger.Error("update download failed", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeUpdateJSON(w, result)
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
