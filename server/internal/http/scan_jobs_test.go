package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/go-chi/chi/v5"
)

func TestDiscoveryControllerCancelScanJobLifecycle(t *testing.T) {
	runner := &blockingScanRunner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	bus := events.New()
	controller := NewDiscoveryController(runner, &fakeGameStore{}, noopLogger{}, bus)

	router := chi.NewRouter()
	router.Post("/api/scan", controller.Scan)
	router.Get("/api/scan/jobs/{job_id}", controller.GetScanJob)
	router.Post("/api/scan/jobs/{job_id}/cancel", controller.CancelScanJob)

	recStart := httptest.NewRecorder()
	router.ServeHTTP(recStart, httptest.NewRequest(http.MethodPost, "/api/scan", nil))
	if recStart.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202 body=%s", recStart.Code, recStart.Body.String())
	}

	var started core.ScanJobStatus
	if err := json.Unmarshal(recStart.Body.Bytes(), &started); err != nil {
		t.Fatalf("unmarshal start response: %v", err)
	}

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("scan runner did not start")
	}

	events.PublishJSON(bus, "scan_started", map[string]any{
		"job_id":            started.JobID,
		"integration_count": 1,
		"integrations": []map[string]any{{
			"integration_id": "library-1",
			"plugin_id":      "source-filesystem",
			"label":          "Main Library",
		}},
	})
	events.PublishJSON(bus, "scan_integration_started", map[string]any{
		"job_id":         started.JobID,
		"integration_id": "library-1",
		"plugin_id":      "source-filesystem",
		"label":          "Main Library",
	})
	events.PublishJSON(bus, "scan_scanner_progress", map[string]any{
		"job_id":          started.JobID,
		"integration_id":  "library-1",
		"processed_count": 25,
		"file_count":      100,
	})

	waitForScanJob(t, 2*time.Second, func(job *core.ScanJobStatus) bool {
		return job != nil &&
			len(job.Integrations) == 1 &&
			len(job.RecentEvents) >= 2 &&
			job.Integrations[0].SourceProgress != nil &&
			job.Integrations[0].SourceProgress.Current == 25
	}, func() *core.ScanJobStatus {
		return controller.scanJobs.Get(started.JobID)
	})

	recCancel := httptest.NewRecorder()
	router.ServeHTTP(recCancel, httptest.NewRequest(http.MethodPost, "/api/scan/jobs/"+started.JobID+"/cancel", nil))
	if recCancel.Code != http.StatusAccepted {
		t.Fatalf("cancel status = %d, want 202 body=%s", recCancel.Code, recCancel.Body.String())
	}

	var cancelling core.ScanJobStatus
	if err := json.Unmarshal(recCancel.Body.Bytes(), &cancelling); err != nil {
		t.Fatalf("unmarshal cancel response: %v", err)
	}
	if cancelling.Status != "cancelling" {
		t.Fatalf("cancel response status = %q, want cancelling", cancelling.Status)
	}

	waitForScanJob(t, 2*time.Second, func(job *core.ScanJobStatus) bool {
		return job != nil && job.Status == "cancelled"
	}, func() *core.ScanJobStatus {
		return controller.scanJobs.Get(started.JobID)
	})

	final := controller.scanJobs.Get(started.JobID)
	if final == nil {
		t.Fatal("expected final job")
	}
	if final.Status != "cancelled" {
		t.Fatalf("final status = %q, want cancelled", final.Status)
	}
	if final.Error != "" {
		t.Fatalf("final error = %q, want empty", final.Error)
	}
	if len(final.RecentEvents) == 0 || final.RecentEvents[len(final.RecentEvents)-1].Type != "scan_cancelled" {
		t.Fatalf("expected final recent event to be scan_cancelled, got %+v", final.RecentEvents)
	}
}

func TestDiscoveryControllerCancelCompletedScanJobConflicts(t *testing.T) {
	runner := &blockingScanRunner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	controller := NewDiscoveryController(runner, &fakeGameStore{}, noopLogger{}, events.New())

	router := chi.NewRouter()
	router.Post("/api/scan", controller.Scan)
	router.Post("/api/scan/jobs/{job_id}/cancel", controller.CancelScanJob)

	recStart := httptest.NewRecorder()
	router.ServeHTTP(recStart, httptest.NewRequest(http.MethodPost, "/api/scan", nil))
	if recStart.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202 body=%s", recStart.Code, recStart.Body.String())
	}

	var started core.ScanJobStatus
	if err := json.Unmarshal(recStart.Body.Bytes(), &started); err != nil {
		t.Fatalf("unmarshal start response: %v", err)
	}

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("scan runner did not start")
	}
	close(runner.release)

	waitForScanJob(t, 2*time.Second, func(job *core.ScanJobStatus) bool {
		return job != nil && job.Status == "completed"
	}, func() *core.ScanJobStatus {
		return controller.scanJobs.Get(started.JobID)
	})

	recCancel := httptest.NewRecorder()
	router.ServeHTTP(recCancel, httptest.NewRequest(http.MethodPost, "/api/scan/jobs/"+started.JobID+"/cancel", nil))
	if recCancel.Code != http.StatusConflict {
		t.Fatalf("cancel completed status = %d, want 409 body=%s", recCancel.Code, recCancel.Body.String())
	}

	var completed core.ScanJobStatus
	if err := json.Unmarshal(recCancel.Body.Bytes(), &completed); err != nil {
		t.Fatalf("unmarshal completed cancel response: %v", err)
	}
	if completed.Status != "completed" {
		t.Fatalf("completed cancel response status = %q, want completed", completed.Status)
	}
}

func TestScanJobTracksMetadataProvidersByIntegrationID(t *testing.T) {
	runner := &blockingScanRunner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer close(runner.release)

	bus := events.New()
	controller := NewDiscoveryController(runner, &fakeGameStore{}, noopLogger{}, bus)

	router := chi.NewRouter()
	router.Post("/api/scan", controller.Scan)

	recStart := httptest.NewRecorder()
	router.ServeHTTP(recStart, httptest.NewRequest(http.MethodPost, "/api/scan", nil))
	if recStart.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202 body=%s", recStart.Code, recStart.Body.String())
	}

	var started core.ScanJobStatus
	if err := json.Unmarshal(recStart.Body.Bytes(), &started); err != nil {
		t.Fatalf("unmarshal start response: %v", err)
	}

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("scan runner did not start")
	}

	events.PublishJSON(bus, "scan_started", map[string]any{
		"job_id":            started.JobID,
		"integration_count": 1,
		"integrations": []map[string]any{{
			"integration_id": "library-1",
			"plugin_id":      "source-filesystem",
			"label":          "Main Library",
		}},
	})
	events.PublishJSON(bus, "scan_integration_started", map[string]any{
		"job_id":         started.JobID,
		"integration_id": "library-1",
		"plugin_id":      "source-filesystem",
		"label":          "Main Library",
	})
	events.PublishJSON(bus, "scan_metadata_started", map[string]any{
		"job_id":         started.JobID,
		"integration_id": "library-1",
		"game_count":     132,
		"resolver_count": 2,
		"metadata_providers": []map[string]any{
			{
				"integration_id": "metadata-steam-primary",
				"label":          "Steam Metadata",
				"plugin_id":      "metadata-steam",
				"status":         "pending",
			},
			{
				"integration_id": "metadata-steam-secondary",
				"label":          "Steam Metadata Fallback",
				"plugin_id":      "metadata-steam",
				"status":         "error",
				"reason":         "invalid_config",
				"error":          "missing api key",
			},
		},
	})
	events.PublishJSON(bus, "scan_metadata_plugin_started", map[string]any{
		"job_id":                  started.JobID,
		"integration_id":          "library-1",
		"metadata_integration_id": "metadata-steam-primary",
		"metadata_label":          "Steam Metadata",
		"plugin_id":               "metadata-steam",
		"phase":                   "identify",
		"batch_size":              132,
	})

	waitForScanJob(t, 2*time.Second, func(job *core.ScanJobStatus) bool {
		if job == nil || len(job.Integrations) != 1 {
			return false
		}
		integration := job.Integrations[0]
		if integration.PluginID != "source-filesystem" {
			return false
		}
		if integration.MetadataIntegrationID != "metadata-steam-primary" {
			return false
		}
		if len(integration.MetadataProviders) != 2 {
			return false
		}
		return integration.MetadataProviders[0].Status == "running" && integration.MetadataProviders[1].Status == "error"
	}, func() *core.ScanJobStatus {
		return controller.scanJobs.Get(started.JobID)
	})

	job := controller.scanJobs.Get(started.JobID)
	if job == nil {
		t.Fatal("expected scan job snapshot")
	}
	integration := job.Integrations[0]
	if integration.PluginID != "source-filesystem" {
		t.Fatalf("source plugin id = %q, want source-filesystem", integration.PluginID)
	}
	if integration.MetadataLabel != "Steam Metadata" {
		t.Fatalf("metadata label = %q, want Steam Metadata", integration.MetadataLabel)
	}
	if integration.MetadataProviders[0].IntegrationID == integration.MetadataProviders[1].IntegrationID {
		t.Fatalf("metadata providers should remain distinct, got %+v", integration.MetadataProviders)
	}
	if integration.MetadataProviders[1].Reason != "invalid_config" {
		t.Fatalf("fallback provider reason = %q, want invalid_config", integration.MetadataProviders[1].Reason)
	}
}

func waitForScanJob(t *testing.T, timeout time.Duration, done func(*core.ScanJobStatus) bool, get func() *core.ScanJobStatus) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if job := get(); done(job) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for scan job condition, last=%+v", get())
}
