package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
	"github.com/go-chi/chi/v5"
)

type integrationRefreshTestRunner struct {
	run func(context.Context, *core.Integration, integrationRefreshCallbacks) error
}

type integrationRefreshCallbacks = scan.IntegrationRefreshCallbacks

func (r integrationRefreshTestRunner) RunIntegrationRefresh(ctx context.Context, integration *core.Integration, callbacks integrationRefreshCallbacks) error {
	return r.run(ctx, integration, callbacks)
}

type integrationRefreshTestHost struct {
	plugins map[string]*core.Plugin
}

func (h integrationRefreshTestHost) GetPlugin(pluginID string) (*core.Plugin, bool) {
	plugin, ok := h.plugins[pluginID]
	return plugin, ok
}

func TestIntegrationRefreshControllerStartsAndTracksWarnings(t *testing.T) {
	repo := &fakeControllerIntegrationRepo{
		byID: map[string]*core.Integration{
			"steam-1": {
				ID:       "steam-1",
				PluginID: "game-source-steam",
				Label:    "Steam",
			},
		},
	}
	controller := NewIntegrationRefreshController(
		repo,
		integrationRefreshTestHost{plugins: map[string]*core.Plugin{
			"game-source-steam": {
				Manifest: core.PluginManifest{
					ID:       "game-source-steam",
					Provides: []string{"achievements.game.get"},
				},
			},
		}},
		integrationRefreshTestRunner{
			run: func(_ context.Context, integration *core.Integration, callbacks integrationRefreshCallbacks) error {
				callbacks.SetPhase("refreshing_achievements", 2)
				callbacks.Progress(1, 2, integration.Label+" One")
				callbacks.Warning("first warning")
				callbacks.Progress(2, 2, integration.Label+" Two")
				return nil
			},
		},
		events.New(),
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/integrations/{id}/refresh", controller.Start)
	router.Get("/api/integration-refresh/jobs/{job_id}", controller.GetJob)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/integrations/steam-1/refresh", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202 body=%s", rec.Code, rec.Body.String())
	}

	var started core.IntegrationRefreshJobStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatalf("unmarshal start response: %v", err)
	}

	waitForIntegrationRefreshJob(t, 2*time.Second, func(job *core.IntegrationRefreshJobStatus) bool {
		return job != nil && job.Status == "completed"
	}, func() *core.IntegrationRefreshJobStatus {
		return controller.jobs.Get(started.JobID)
	})

	final := controller.jobs.Get(started.JobID)
	if final == nil {
		t.Fatal("expected final job")
	}
	if final.WarningCount != 1 {
		t.Fatalf("warning_count = %d, want 1", final.WarningCount)
	}
	if final.ItemsCompleted != 2 || final.ItemsTotal != 2 {
		t.Fatalf("progress = %d/%d, want 2/2", final.ItemsCompleted, final.ItemsTotal)
	}
}

func TestIntegrationRefreshControllerRejectsIneligibleIntegration(t *testing.T) {
	repo := &fakeControllerIntegrationRepo{
		byID: map[string]*core.Integration{
			"sync-1": {
				ID:       "sync-1",
				PluginID: "sync-settings-google-drive",
				Label:    "Settings Sync",
			},
		},
	}
	controller := NewIntegrationRefreshController(
		repo,
		integrationRefreshTestHost{plugins: map[string]*core.Plugin{
			"sync-settings-google-drive": {
				Manifest: core.PluginManifest{
					ID:       "sync-settings-google-drive",
					Provides: []string{"sync.push"},
				},
			},
		}},
		integrationRefreshTestRunner{
			run: func(context.Context, *core.Integration, integrationRefreshCallbacks) error {
				t.Fatal("runner should not be called")
				return nil
			},
		},
		nil,
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/integrations/{id}/refresh", controller.Start)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/integrations/sync-1/refresh", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func waitForIntegrationRefreshJob(
	t *testing.T,
	timeout time.Duration,
	ok func(*core.IntegrationRefreshJobStatus) bool,
	current func() *core.IntegrationRefreshJobStatus,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job := current()
		if ok(job) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for integration refresh job; last=%+v", current())
}
