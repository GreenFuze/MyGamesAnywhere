package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
	"github.com/go-chi/chi/v5"
)

func TestAchievementRefreshControllerStartsRejectsDuplicateAndPolls(t *testing.T) {
	runner := &blockingAchievementRefreshRunner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	bus := events.New()
	sub := bus.Subscribe()
	controller := NewAchievementRefreshController(runner, bus, noopLogger{})

	router := chi.NewRouter()
	router.Post("/api/achievements/refresh", controller.Start)
	router.Get("/api/achievements/refresh/jobs/{job_id}", controller.GetJob)

	recStart := httptest.NewRecorder()
	router.ServeHTTP(recStart, httptest.NewRequest(http.MethodPost, "/api/achievements/refresh", nil))
	if recStart.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202 body=%s", recStart.Code, recStart.Body.String())
	}

	var started core.AchievementRefreshJobStatus
	if err := json.Unmarshal(recStart.Body.Bytes(), &started); err != nil {
		t.Fatalf("unmarshal start response: %v", err)
	}

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("achievement refresh runner did not start")
	}
	waitForAchievementRefreshEvent(t, sub, "achievement_refresh_waiting")

	recDuplicate := httptest.NewRecorder()
	router.ServeHTTP(recDuplicate, httptest.NewRequest(http.MethodPost, "/api/achievements/refresh", nil))
	if recDuplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409 body=%s", recDuplicate.Code, recDuplicate.Body.String())
	}

	var duplicate core.AchievementRefreshJobStatus
	if err := json.Unmarshal(recDuplicate.Body.Bytes(), &duplicate); err != nil {
		t.Fatalf("unmarshal duplicate response: %v", err)
	}
	if duplicate.JobID != started.JobID {
		t.Fatalf("duplicate job id = %q, want active job %q", duplicate.JobID, started.JobID)
	}

	close(runner.release)
	waitForAchievementRefreshJob(t, 2*time.Second, func(job *core.AchievementRefreshJobStatus) bool {
		return job != nil &&
			job.Status == "completed" &&
			job.ItemsTotal == 2 &&
			job.ItemsCompleted == 2 &&
			job.SuccessCount == 1 &&
			job.WarningCount == 1 &&
			job.ErrorCount == 1 &&
			job.ProviderID == "ra-integration" &&
			job.ProviderLabel == "RetroAchievements"
	}, func() *core.AchievementRefreshJobStatus {
		return controller.jobs.Get(started.JobID)
	})

	recGet := httptest.NewRecorder()
	router.ServeHTTP(recGet, httptest.NewRequest(http.MethodGet, "/api/achievements/refresh/jobs/"+started.JobID, nil))
	if recGet.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200 body=%s", recGet.Code, recGet.Body.String())
	}
}

type blockingAchievementRefreshRunner struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingAchievementRefreshRunner) RefreshAll(ctx context.Context, callbacks scan.AchievementRefreshCallbacks) (*scan.AchievementRefreshResult, error) {
	r.once.Do(func() { close(r.started) })
	if callbacks.SetTotal != nil {
		callbacks.SetTotal(2)
	}
	if callbacks.Progress != nil {
		callbacks.Progress(0, 2, "Game A")
	}
	if callbacks.Waiting != nil {
		callbacks.Waiting(scan.AchievementRefreshWait{
			Completed:     0,
			Total:         2,
			Item:          "Game A",
			ProviderID:    "ra-integration",
			ProviderLabel: "RetroAchievements",
			Message:       "RetroAchievements is rate-limited. Waiting 1s before retrying achievement refresh.",
			WaitingUntil:  time.Now().Add(time.Second).UTC().Format(time.RFC3339),
			Delay:         time.Second,
		})
	}
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if callbacks.Warning != nil {
		callbacks.Warning("Game B: provider unavailable")
	}
	if callbacks.Progress != nil {
		callbacks.Progress(2, 2, "Game B")
	}
	return &scan.AchievementRefreshResult{Targets: 2, Success: 1, Failed: 1}, nil
}

func waitForAchievementRefreshEvent(t *testing.T, sub <-chan events.Event, eventType string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-sub:
			if ev.Type == eventType {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", eventType)
		}
	}
}

func waitForAchievementRefreshJob(
	t *testing.T,
	timeout time.Duration,
	done func(*core.AchievementRefreshJobStatus) bool,
	get func() *core.AchievementRefreshJobStatus,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if job := get(); done(job) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met before timeout; last job: %+v", get())
}
