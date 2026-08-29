package http

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/contentdelivery"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5/middleware"
)

const maxConcurrentContentStreamsPerProfile = 4

type ContentDeliveryService interface {
	Manifest(ctx context.Context, copyID string) (*contentdelivery.Manifest, error)
	OpenFile(ctx context.Context, copyID, fileID string) (*contentdelivery.OpenFileResult, error)
	Prepare(ctx context.Context, copyID string) (*core.SourceCacheJobStatus, bool, error)
	GetJob(ctx context.Context, jobID string) (*core.SourceCacheJobStatus, error)
	CancelJob(ctx context.Context, jobID string) (*core.SourceCacheJobStatus, bool, error)
}

type ContentController struct {
	service ContentDeliveryService
	logger  core.Logger
	streams *profileStreamLimiter
}

func NewContentController(service ContentDeliveryService, logger core.Logger) (*ContentController, error) {
	if service == nil {
		return nil, errors.New("content delivery service is required")
	}
	return &ContentController{
		service: service,
		logger:  logger,
		streams: newProfileStreamLimiter(maxConcurrentContentStreamsPerProfile),
	}, nil
}

func (c *ContentController) Manifest(w http.ResponseWriter, r *http.Request) {
	copyID, err := decodedPathParam(r, "copy_id")
	if err != nil || strings.TrimSpace(copyID) == "" {
		writeContentError(w, http.StatusBadRequest, "invalid_copy_id", "copy_id is required")
		return
	}
	manifest, err := c.service.Manifest(r.Context(), copyID)
	if err != nil {
		c.writeServiceError(w, r, "build content manifest", err, "copy_id", copyID)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", manifest.ETag)
	if etagMatches(r.Header.Get("If-None-Match"), manifest.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_ = json.NewEncoder(w).Encode(manifest)
	c.audit(r, http.StatusOK, 0, "manifest", "copy_id", copyID)
}

func (c *ContentController) File(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	copyID, copyErr := decodedPathParam(r, "copy_id")
	fileID, fileErr := decodedPathParam(r, "file_id")
	if copyErr != nil || fileErr != nil || strings.TrimSpace(copyID) == "" || !contentdelivery.ValidFileID(fileID) {
		writeContentError(w, http.StatusBadRequest, "invalid_content_id", "copy_id and file_id are required")
		return
	}
	if strings.Contains(r.Header.Get("Range"), ",") {
		w.Header().Set("Content-Range", "bytes */*")
		writeContentError(w, http.StatusRequestedRangeNotSatisfiable, "multiple_ranges_unsupported", "request one byte range at a time")
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	release, ok := c.streams.Acquire(profileID)
	if !ok {
		w.Header().Set("Retry-After", "1")
		writeContentError(w, http.StatusTooManyRequests, "stream_limit", "too many concurrent content streams for this profile")
		c.audit(r, http.StatusTooManyRequests, 0, "stream_rejected", "copy_id", copyID, "file_id", fileID)
		return
	}
	defer release()

	opened, err := c.service.OpenFile(r.Context(), copyID, fileID)
	if err != nil {
		c.writeServiceError(w, r, "open content file", err, "copy_id", copyID, "file_id", fileID)
		return
	}
	defer opened.Reader.Close()

	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(opened.Name)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": opened.Name}))
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("ETag", opened.File.ETag)

	recorder := &contentResponseRecorder{ResponseWriter: w, status: http.StatusOK}
	http.ServeContent(recorder, r, opened.Name, opened.ModTime, opened.Reader)
	c.audit(r, recorder.status, opened.Size, "file", "copy_id", copyID, "file_id", fileID, "bytes_written", recorder.bytes, "duration_ms", time.Since(started).Milliseconds())
}

func (c *ContentController) Prepare(w http.ResponseWriter, r *http.Request) {
	copyID, err := decodedPathParam(r, "copy_id")
	if err != nil || strings.TrimSpace(copyID) == "" {
		writeContentError(w, http.StatusBadRequest, "invalid_copy_id", "copy_id is required")
		return
	}
	job, immediate, err := c.service.Prepare(r.Context(), copyID)
	if err != nil {
		c.writeServiceError(w, r, "prepare content materialization", err, "copy_id", copyID)
		return
	}
	status := http.StatusAccepted
	if immediate {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"immediate": immediate, "job": safeMaterializationJob(job)})
	c.audit(r, status, 0, "materialization_prepare", "copy_id", copyID, "job_id", jobID(job))
}

func (c *ContentController) GetMaterialization(w http.ResponseWriter, r *http.Request) {
	jobIDValue, err := decodedPathParam(r, "job_id")
	if err != nil || strings.TrimSpace(jobIDValue) == "" {
		writeContentError(w, http.StatusBadRequest, "invalid_job_id", "job_id is required")
		return
	}
	job, err := c.service.GetJob(r.Context(), jobIDValue)
	if err != nil {
		c.writeServiceError(w, r, "get content materialization", err, "job_id", jobIDValue)
		return
	}
	if job == nil {
		writeContentError(w, http.StatusNotFound, "not_found", "materialization not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(safeMaterializationJob(job))
	c.audit(r, http.StatusOK, 0, "materialization_status", "job_id", jobIDValue)
}

func (c *ContentController) CancelMaterialization(w http.ResponseWriter, r *http.Request) {
	jobIDValue, err := decodedPathParam(r, "job_id")
	if err != nil || strings.TrimSpace(jobIDValue) == "" {
		writeContentError(w, http.StatusBadRequest, "invalid_job_id", "job_id is required")
		return
	}
	job, _, err := c.service.CancelJob(r.Context(), jobIDValue)
	if err != nil {
		c.writeServiceError(w, r, "cancel content materialization", err, "job_id", jobIDValue)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(safeMaterializationJob(job))
	c.audit(r, http.StatusAccepted, 0, "materialization_cancel", "job_id", jobIDValue)
}

func (c *ContentController) writeServiceError(w http.ResponseWriter, r *http.Request, operation string, err error, fields ...any) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "content request failed"
	switch {
	case errors.Is(err, contentdelivery.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "content resource not found"
	case errors.Is(err, contentdelivery.ErrInvalidContent):
		status, code, message = http.StatusConflict, "invalid_manifest", "source content metadata is invalid; refresh the source"
	case errors.Is(err, contentdelivery.ErrMaterializationRequired):
		status, code, message = http.StatusConflict, "materialization_required", "materialize this content before downloading"
	case errors.Is(err, contentdelivery.ErrUnavailable):
		status, code, message = http.StatusConflict, "delivery_unavailable", "this source cannot deliver content"
	case errors.Is(err, contentdelivery.ErrSourceChanged):
		status, code, message = http.StatusConflict, "source_changed", "source content changed; refresh the source and manifest"
	case errors.Is(err, contentdelivery.ErrJobNotCancellable):
		status, code, message = http.StatusConflict, "job_not_cancellable", "materialization is already terminal"
	}
	if status >= http.StatusInternalServerError && c.logger != nil {
		c.logger.Error(operation, err, fields...)
	}
	writeContentError(w, status, code, message)
	c.audit(r, status, 0, code, fields...)
}

func (c *ContentController) audit(r *http.Request, status int, knownLength int64, outcome string, fields ...any) {
	if c == nil || c.logger == nil {
		return
	}
	args := []any{
		"profile_id", core.ProfileIDFromContext(r.Context()),
		"request_id", middleware.GetReqID(r.Context()),
		"method", r.Method,
		"status", status,
		"outcome", outcome,
		"known_length", knownLength,
	}
	if value := strings.TrimSpace(r.Header.Get("Range")); value != "" {
		if len(value) > 256 {
			value = value[:256]
		}
		args = append(args, "range", value)
	}
	args = append(args, fields...)
	c.logger.Info("content delivery audit", args...)
}

type contentErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeContentError(w http.ResponseWriter, status int, code, message string) {
	response := contentErrorResponse{}
	response.Error.Code = code
	response.Error.Message = message
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

type materializationJobDTO struct {
	JobID           string     `json:"job_id"`
	CopyID          string     `json:"copy_id"`
	Status          string     `json:"status"`
	Message         string     `json:"message,omitempty"`
	ErrorCode       string     `json:"error_code,omitempty"`
	ProgressCurrent int        `json:"progress_current"`
	ProgressTotal   int        `json:"progress_total"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

func safeMaterializationJob(job *core.SourceCacheJobStatus) *materializationJobDTO {
	if job == nil {
		return nil
	}
	message := job.Message
	errorCode := ""
	if job.Status == "failed" {
		message = "Materialization failed. Check the source connection and server logs, then retry."
		errorCode = "source_materialization_failed"
	}
	return &materializationJobDTO{
		JobID:           job.JobID,
		CopyID:          job.SourceGameID,
		Status:          job.Status,
		Message:         message,
		ErrorCode:       errorCode,
		ProgressCurrent: job.ProgressCurrent,
		ProgressTotal:   job.ProgressTotal,
		CreatedAt:       job.CreatedAt,
		UpdatedAt:       job.UpdatedAt,
		FinishedAt:      job.FinishedAt,
	}
}

func jobID(job *core.SourceCacheJobStatus) string {
	if job == nil {
		return ""
	}
	return job.JobID
}

func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}

type contentResponseRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (w *contentResponseRecorder) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *contentResponseRecorder) Write(payload []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	written, err := w.ResponseWriter.Write(payload)
	w.bytes += int64(written)
	return written, err
}

type profileStreamLimiter struct {
	mu     sync.Mutex
	limit  int
	active map[string]int
}

func newProfileStreamLimiter(limit int) *profileStreamLimiter {
	return &profileStreamLimiter{limit: limit, active: make(map[string]int)}
}

func (l *profileStreamLimiter) Acquire(profileID string) (func(), bool) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return func() {}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[profileID] >= l.limit {
		return func() {}, false
	}
	l.active[profileID]++
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.active[profileID]--
		if l.active[profileID] <= 0 {
			delete(l.active, profileID)
		}
	}, true
}
