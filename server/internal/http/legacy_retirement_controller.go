package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/legacyretirement"
)

type LegacyRetirementService interface {
	Report(context.Context, string) (*legacyretirement.Report, error)
}

type LegacyRetirementController struct {
	service LegacyRetirementService
	logger  core.Logger
}

func NewLegacyRetirementController(service LegacyRetirementService, logger core.Logger) (*LegacyRetirementController, error) {
	if service == nil || logger == nil {
		return nil, errors.New("legacy retirement service and logger are required")
	}
	return &LegacyRetirementController{service: service, logger: logger}, nil
}

func (c *LegacyRetirementController) Report(w http.ResponseWriter, r *http.Request) {
	report, err := c.service.Report(r.Context(), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		if errors.Is(err, legacyretirement.ErrProfileRequired) {
			writeContentError(w, http.StatusBadRequest, "profile_required", err.Error())
			return
		}
		c.logger.Error("build legacy retirement report failed", err)
		writeContentError(w, http.StatusInternalServerError, "retirement_report_failed", "legacy client data report could not be generated")
		return
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="mga-legacy-client-data-`+time.Now().UTC().Format("20060102-150405")+`.json"`)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, report)
	c.logger.Info("legacy client data audit", "action", "export", "profile_id", report.ProfileID, "schema_version", report.SchemaVersion)
}
