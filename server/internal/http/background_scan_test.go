package http

import (
	"context"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type backgroundTestCoordinator struct {
	jobs          map[string]*core.ScanJobStatus
	startCount    int
	lastRequest   ScanRequest
	lastProfileID string
}

func (c *backgroundTestCoordinator) StartScan(ctx context.Context, req ScanRequest) (*core.ScanJobStatus, bool, error) {
	c.startCount++
	c.lastRequest = req
	c.lastProfileID = core.ProfileIDFromContext(ctx)
	job := &core.ScanJobStatus{
		JobID:     "background-job",
		Status:    "running",
		Trigger:   req.Trigger,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	c.jobs[job.JobID] = job
	return cloneScanJobStatus(job), false, nil
}

func (c *backgroundTestCoordinator) ScanJobStatus(_ string, jobID string) *core.ScanJobStatus {
	return cloneScanJobStatus(c.jobs[jobID])
}

type backgroundTestProfileRepository struct {
	profiles []*core.Profile
}

func (r *backgroundTestProfileRepository) Create(context.Context, *core.Profile) error { return nil }
func (r *backgroundTestProfileRepository) Update(context.Context, *core.Profile) error { return nil }
func (r *backgroundTestProfileRepository) Delete(context.Context, string) error        { return nil }
func (r *backgroundTestProfileRepository) List(context.Context) ([]*core.Profile, error) {
	return r.profiles, nil
}
func (r *backgroundTestProfileRepository) GetByID(context.Context, string) (*core.Profile, error) {
	return nil, nil
}
func (r *backgroundTestProfileRepository) Count(context.Context) (int, error) {
	return len(r.profiles), nil
}
func (r *backgroundTestProfileRepository) CountAdmins(context.Context) (int, error) { return 1, nil }
func (r *backgroundTestProfileRepository) EnsureDefaultForExistingData(context.Context) (*core.Profile, error) {
	return nil, nil
}

type backgroundTestSettingRepository struct {
	settings map[string]*core.Setting
}

func (r *backgroundTestSettingRepository) key(ctx context.Context, key string) string {
	return core.ProfileIDFromContext(ctx) + ":" + key
}

func (r *backgroundTestSettingRepository) Upsert(ctx context.Context, setting *core.Setting) error {
	copy := *setting
	r.settings[r.key(ctx, setting.Key)] = &copy
	return nil
}

func (r *backgroundTestSettingRepository) Get(ctx context.Context, key string) (*core.Setting, error) {
	setting := r.settings[r.key(ctx, key)]
	if setting == nil {
		return nil, nil
	}
	copy := *setting
	return &copy, nil
}

func (r *backgroundTestSettingRepository) List(context.Context) ([]*core.Setting, error) {
	return nil, nil
}

func TestBackgroundScanUsesSharedCoordinatorAndReportsVisibility(t *testing.T) {
	profile := &core.Profile{ID: "profile-1", DisplayName: "Player"}
	coordinator := &backgroundTestCoordinator{jobs: make(map[string]*core.ScanJobStatus)}
	settings := &backgroundTestSettingRepository{settings: make(map[string]*core.Setting)}
	service, err := NewBackgroundScanService(
		coordinator,
		&backgroundTestProfileRepository{profiles: []*core.Profile{profile}},
		settings,
		noopLogger{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewBackgroundScanService: %v", err)
	}

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if err := service.tick(context.Background()); err != nil {
		t.Fatalf("initial tick: %v", err)
	}
	if coordinator.startCount != 0 {
		t.Fatalf("start count = %d, want 0 during initial delay", coordinator.startCount)
	}

	now = now.Add(backgroundScanInitialDelay)
	if err := service.tick(context.Background()); err != nil {
		t.Fatalf("due tick: %v", err)
	}
	if coordinator.startCount != 1 {
		t.Fatalf("start count = %d, want 1", coordinator.startCount)
	}
	if coordinator.lastRequest.Trigger != "background" {
		t.Fatalf("trigger = %q, want background", coordinator.lastRequest.Trigger)
	}
	if coordinator.lastProfileID != profile.ID {
		t.Fatalf("profile = %q, want %q", coordinator.lastProfileID, profile.ID)
	}

	profileCtx := core.WithProfile(context.Background(), profile)
	status, err := service.Status(profileCtx)
	if err != nil {
		t.Fatalf("Status while running: %v", err)
	}
	if status.State != "running" || status.ActiveJob == nil {
		t.Fatalf("running status = %+v", status)
	}

	coordinator.jobs["background-job"].Status = "completed"
	coordinator.jobs["background-job"].FinishedAt = now.Format(time.RFC3339)
	now = now.Add(backgroundScanSchedulerTick)
	if err := service.tick(context.Background()); err != nil {
		t.Fatalf("completion tick: %v", err)
	}
	status, err = service.Status(profileCtx)
	if err != nil {
		t.Fatalf("Status after completion: %v", err)
	}
	if status.State != "scheduled" || status.LastStatus != "completed" || status.ActiveJob != nil {
		t.Fatalf("completed status = %+v", status)
	}
	wantNext := now.Add(defaultLibraryScanInterval * time.Minute).Format(time.RFC3339)
	if status.NextRunAt != wantNext {
		t.Fatalf("next run = %q, want %q", status.NextRunAt, wantNext)
	}
}

func TestBackgroundScanConfigIsProfileScopedAndValidated(t *testing.T) {
	profile := &core.Profile{ID: "profile-1", DisplayName: "Player"}
	settings := &backgroundTestSettingRepository{settings: make(map[string]*core.Setting)}
	service, err := NewBackgroundScanService(
		&backgroundTestCoordinator{jobs: make(map[string]*core.ScanJobStatus)},
		&backgroundTestProfileRepository{profiles: []*core.Profile{profile}},
		settings,
		noopLogger{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewBackgroundScanService: %v", err)
	}
	ctx := core.WithProfile(context.Background(), profile)
	status, err := service.UpdateConfig(ctx, core.LibraryScanScheduleConfig{Enabled: false, IntervalMinutes: 30})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if status.State != "disabled" || status.IntervalMinutes != 30 {
		t.Fatalf("updated status = %+v", status)
	}
	if settings.settings[profile.ID+":"+libraryScanScheduleSetting] == nil {
		t.Fatal("expected profile-scoped setting")
	}
	if _, err := service.UpdateConfig(ctx, core.LibraryScanScheduleConfig{Enabled: true, IntervalMinutes: 1}); err == nil {
		t.Fatal("expected too-short interval to fail")
	}
}
