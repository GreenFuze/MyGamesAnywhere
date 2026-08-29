package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/runtimeartifact"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type RuntimeArtifactService interface {
	List(ctx context.Context) ([]runtimeartifact.Artifact, error)
	Get(ctx context.Context, artifactID string) (*runtimeartifact.Artifact, error)
	Upsert(ctx context.Context, artifact runtimeartifact.Artifact) (*runtimeartifact.Artifact, error)
	Open(ctx context.Context, artifactID string) (*runtimeartifact.OpenResult, error)
}

type RuntimeArtifactController struct {
	service RuntimeArtifactService
	logger  core.Logger
}

func NewRuntimeArtifactController(service RuntimeArtifactService, logger core.Logger) (*RuntimeArtifactController, error) {
	if service == nil || logger == nil {
		return nil, errors.New("runtime artifact service and logger are required")
	}
	return &RuntimeArtifactController{service: service, logger: logger}, nil
}

func (c *RuntimeArtifactController) List(w http.ResponseWriter, r *http.Request) {
	artifacts, err := c.service.List(r.Context())
	if err != nil {
		c.writeError(w, r, err, "list")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifacts": artifacts})
}

func (c *RuntimeArtifactController) Get(w http.ResponseWriter, r *http.Request) {
	artifact, err := c.service.Get(r.Context(), strings.TrimSpace(chi.URLParam(r, "artifact_id")))
	if err != nil {
		c.writeError(w, r, err, "get")
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}

func (c *RuntimeArtifactController) Create(w http.ResponseWriter, r *http.Request) {
	artifact, err := decodeRuntimeArtifact(w, r)
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_artifact", err.Error())
		return
	}
	artifact.Normalize()
	if err := artifact.Validate(); err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_artifact", err.Error())
		return
	}
	saved, err := c.service.Upsert(r.Context(), artifact)
	if err != nil {
		c.writeError(w, r, err, "create")
		return
	}
	writeJSON(w, http.StatusCreated, saved)
	c.audit(r, http.StatusCreated, "create", saved.ID)
}

func (c *RuntimeArtifactController) Update(w http.ResponseWriter, r *http.Request) {
	artifactID := strings.TrimSpace(chi.URLParam(r, "artifact_id"))
	artifact, err := decodeRuntimeArtifact(w, r)
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_artifact", err.Error())
		return
	}
	artifact.Normalize()
	if artifact.ID == "" {
		artifact.ID = artifactID
	}
	if artifactID == "" || artifact.ID != artifactID {
		writeContentError(w, http.StatusBadRequest, "artifact_identity_mismatch", "path and body artifact identity must match")
		return
	}
	if err := artifact.Validate(); err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_artifact", err.Error())
		return
	}
	saved, err := c.service.Upsert(r.Context(), artifact)
	if err != nil {
		c.writeError(w, r, err, "update")
		return
	}
	writeJSON(w, http.StatusOK, saved)
	c.audit(r, http.StatusOK, "update", saved.ID)
}

func (c *RuntimeArtifactController) Content(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	artifactID := strings.TrimSpace(chi.URLParam(r, "artifact_id"))
	if strings.Contains(r.Header.Get("Range"), ",") {
		writeContentError(w, http.StatusRequestedRangeNotSatisfiable, "multiple_ranges_unsupported", "request one byte range at a time")
		return
	}
	opened, err := c.service.Open(r.Context(), artifactID)
	if err != nil {
		c.writeError(w, r, err, "content")
		return
	}
	defer opened.File.Close()
	name := fmt.Sprintf("%s-%s-%s-%s.bin", opened.Artifact.PackageID, opened.Artifact.Version, opened.Artifact.OS, opened.Artifact.Architecture)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("ETag", `"sha256:`+opened.Artifact.SHA256+`"`)
	recorder := &contentResponseRecorder{ResponseWriter: w, status: http.StatusOK}
	http.ServeContent(recorder, r, name, opened.Artifact.UpdatedAt, opened.File)
	c.audit(r, recorder.status, "content", artifactID, "bytes_written", recorder.bytes, "duration_ms", time.Since(started).Milliseconds())
}

func decodeRuntimeArtifact(w http.ResponseWriter, r *http.Request) (runtimeartifact.Artifact, error) {
	var artifact runtimeartifact.Artifact
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&artifact); err != nil {
		return artifact, fmt.Errorf("decode runtime artifact: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return artifact, errors.New("runtime artifact body must contain exactly one JSON object")
	}
	return artifact, nil
}

func (c *RuntimeArtifactController) writeError(w http.ResponseWriter, r *http.Request, err error, outcome string) {
	status, code, message := http.StatusInternalServerError, "internal_error", "runtime artifact request failed"
	switch {
	case errors.Is(err, runtimeartifact.ErrProfileRequired):
		status, code, message = http.StatusForbidden, "profile_required", "profile authorization is required"
	case errors.Is(err, runtimeartifact.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "runtime artifact not found"
	case errors.Is(err, runtimeartifact.ErrDeliveryBlocked):
		status, code, message = http.StatusConflict, "delivery_blocked", "runtime artifact is not approved for byte delivery"
	case errors.Is(err, runtimeartifact.ErrIntegrity):
		status, code, message = http.StatusConflict, "integrity_failed", "runtime artifact failed integrity verification"
	default:
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "unsupported") || strings.Contains(err.Error(), "must") || strings.Contains(err.Error(), "cannot") {
			status, code, message = http.StatusBadRequest, "invalid_artifact", err.Error()
		} else {
			c.logger.Error("runtime artifact request failed", err, "outcome", outcome)
		}
	}
	writeContentError(w, status, code, message)
	c.audit(r, status, outcome, strings.TrimSpace(chi.URLParam(r, "artifact_id")))
}

func (c *RuntimeArtifactController) audit(r *http.Request, status int, outcome, artifactID string, fields ...any) {
	args := []any{"profile_id", core.ProfileIDFromContext(r.Context()), "request_id", middleware.GetReqID(r.Context()), "method", r.Method, "status", status, "outcome", outcome, "artifact_id", artifactID}
	args = append(args, fields...)
	c.logger.Info("runtime artifact audit", args...)
}
