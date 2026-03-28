package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/buildinfo"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/go-chi/chi/v5"
)

type blockingScanRunner struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func (r *blockingScanRunner) RunScan(ctx context.Context, _ []string) ([]*core.CanonicalGame, error) {
	close(r.started)
	select {
	case <-r.release:
		return nil, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *blockingScanRunner) RunMetadataRefresh(ctx context.Context, _ []string) ([]*core.CanonicalGame, error) {
	return r.RunScan(ctx, nil)
}

func TestAboutControllerGetAbout(t *testing.T) {
	controller := NewAboutController(noopLogger{})

	req := httptest.NewRequest(http.MethodGet, "/api/about", nil)
	rec := httptest.NewRecorder()
	controller.GetAbout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body core.AboutInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	expected := buildinfo.AboutInfo()
	if body.Version != expected.Version || body.Commit != expected.Commit || body.BuildDate != expected.BuildDate {
		t.Fatalf("unexpected about payload: %+v", body)
	}
}

func TestAboutControllerGetLicense(t *testing.T) {
	controller := NewAboutController(noopLogger{})

	req := httptest.NewRequest(http.MethodGet, "/api/about/license", nil)
	rec := httptest.NewRecorder()
	controller.GetLicense(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected license body")
	}
}

func TestDiscoveryControllerScanJobLifecycleAndConflict(t *testing.T) {
	runner := &blockingScanRunner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	controller := NewDiscoveryController(runner, &fakeGameStore{}, noopLogger{}, events.New())

	router := chi.NewRouter()
	router.Post("/api/scan", controller.Scan)
	router.Get("/api/scan/jobs/{job_id}", controller.GetScanJob)

	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, httptest.NewRequest(http.MethodPost, "/api/scan", nil))
	if rec1.Code != http.StatusAccepted {
		t.Fatalf("first scan status = %d, want 202 body=%s", rec1.Code, rec1.Body.String())
	}

	var first core.ScanJobStatus
	if err := json.Unmarshal(rec1.Body.Bytes(), &first); err != nil {
		t.Fatalf("unmarshal first job: %v", err)
	}
	if first.JobID == "" {
		t.Fatal("expected job id")
	}

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("scan runner did not start")
	}

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/scan", nil))
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second scan status = %d, want 409 body=%s", rec2.Code, rec2.Body.String())
	}

	var conflict core.ScanJobStatus
	if err := json.Unmarshal(rec2.Body.Bytes(), &conflict); err != nil {
		t.Fatalf("unmarshal conflict job: %v", err)
	}
	if conflict.JobID != first.JobID {
		t.Fatalf("conflict job id = %q, want %q", conflict.JobID, first.JobID)
	}

	recStatus := httptest.NewRecorder()
	reqStatus := httptest.NewRequest(http.MethodGet, "/api/scan/jobs/"+first.JobID, nil)
	router.ServeHTTP(recStatus, reqStatus)
	if recStatus.Code != http.StatusOK {
		t.Fatalf("status lookup code = %d, want 200", recStatus.Code)
	}

	var current core.ScanJobStatus
	if err := json.Unmarshal(recStatus.Body.Bytes(), &current); err != nil {
		t.Fatalf("unmarshal current job: %v", err)
	}
	if current.Status != "running" && current.Status != "queued" {
		t.Fatalf("job status = %q, want queued or running", current.Status)
	}

	close(runner.release)

	deadline := time.Now().Add(2 * time.Second)
	for {
		job := controller.scanJobs.Get(first.JobID)
		if job != nil && job.Status == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scan job did not reach completed state")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
