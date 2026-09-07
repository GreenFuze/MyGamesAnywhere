package http

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// The periodic run is not only about finding games. A library accumulates work
// that needs another attempt later, and nothing else comes back to it.
//
// Measured on one library: 75 games were found on a drive and shown nowhere,
// because no metadata provider had identified them. A repair existed —
// redetect — but it ran inside a web request bounded at a minute, so the first
// provider lookup timed out, the provider was reported unavailable, and the
// other 74 games were never attempted. The games stayed invisible for as long
// as that kept happening.
//
// So the repair runs after a scan, on a context with no request behind it.

func TestMaintenanceRunsAfterASuccessfulScan(t *testing.T) {
	service := newMaintenanceTestService(t)

	var ran sync.WaitGroup
	ran.Add(1)
	var gotDeadline bool
	service.SetMaintenance(func(ctx context.Context) {
		_, gotDeadline = ctx.Deadline()
		ran.Done()
	})

	service.finishJob("profile-1", &core.ScanJobStatus{Status: "completed"}, core.LibraryScanScheduleConfig{Enabled: true, IntervalMinutes: 15}, time.Now(), "")

	if !waitFor(&ran, 5*time.Second) {
		t.Fatal("maintenance never ran after a successful scan")
	}
	if !gotDeadline {
		// A ceiling, so a wedged repair cannot run for the life of the process.
		t.Error("maintenance ran with no time limit at all")
	}
}

func TestMaintenanceIsSkippedAfterAFailedScan(t *testing.T) {
	// Repair reads what the scan produced. Running it against a partial
	// library would be drawing conclusions from an incomplete answer — the
	// same mistake as treating a failed source as an emptied one.
	service := newMaintenanceTestService(t)

	var mu sync.Mutex
	ranCount := 0
	service.SetMaintenance(func(ctx context.Context) {
		mu.Lock()
		ranCount++
		mu.Unlock()
	})

	service.finishJob("profile-1", &core.ScanJobStatus{Status: "failed", Error: "source unreachable"}, core.LibraryScanScheduleConfig{Enabled: true, IntervalMinutes: 15}, time.Now(), "")
	service.finishJob("profile-1", nil, core.LibraryScanScheduleConfig{Enabled: true, IntervalMinutes: 15}, time.Now(), "job disappeared")

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if ranCount != 0 {
		t.Fatalf("maintenance ran %d times after scans that did not succeed", ranCount)
	}
}

func newMaintenanceTestService(t *testing.T) *BackgroundScanService {
	t.Helper()
	return &BackgroundScanService{
		logger: noopLogger{},
		states: map[string]*backgroundScanProfileState{},
		now:    time.Now,
	}
}

func waitFor(group *sync.WaitGroup, limit time.Duration) bool {
	done := make(chan struct{})
	go func() {
		group.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(limit):
		return false
	}
}
