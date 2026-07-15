package http

import (
	"context"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
)

const (
	libraryScanScheduleSetting   = "library_scan_schedule"
	defaultLibraryScanInterval   = 15
	minimumLibraryScanInterval   = 5
	maximumLibraryScanInterval   = 24 * 60
	backgroundScanSchedulerTick  = 15 * time.Second
	backgroundScanInitialDelay   = time.Minute
	backgroundScanBusyRetryDelay = time.Minute
)

type backgroundScanJobCoordinator interface {
	StartScan(ctx context.Context, req ScanRequest) (*core.ScanJobStatus, bool, error)
	ScanJobStatus(jobID string) *core.ScanJobStatus
}

type backgroundScanProfileState struct {
	status      core.LibraryScanScheduleStatus
	activeJobID string
}

// BackgroundScanService schedules profile-scoped library scans through the
// same job coordinator used by manual Rescan operations.
type BackgroundScanService struct {
	coordinator backgroundScanJobCoordinator
	profiles    core.ProfileRepository
	settings    core.SettingRepository
	logger      core.Logger
	eventBus    *events.EventBus
	now         func() time.Time

	mu     sync.RWMutex
	states map[string]*backgroundScanProfileState
}

func NewBackgroundScanService(
	coordinator backgroundScanJobCoordinator,
	profiles core.ProfileRepository,
	settings core.SettingRepository,
	logger core.Logger,
	eventBus *events.EventBus,
) (*BackgroundScanService, error) {
	if coordinator == nil {
		return nil, fmt.Errorf("background scan coordinator is required")
	}
	if profiles == nil {
		return nil, fmt.Errorf("profile repository is required")
	}
	if settings == nil {
		return nil, fmt.Errorf("setting repository is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	return &BackgroundScanService{
		coordinator: coordinator,
		profiles:    profiles,
		settings:    settings,
		logger:      logger,
		eventBus:    eventBus,
		now:         time.Now,
		states:      make(map[string]*backgroundScanProfileState),
	}, nil
}

func (s *BackgroundScanService) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("background scan context is required")
	}
	if err := s.tick(ctx); err != nil {
		return fmt.Errorf("initialize background library scans: %w", err)
	}
	go s.run(ctx)
	return nil
}

func (s *BackgroundScanService) run(ctx context.Context) {
	ticker := time.NewTicker(backgroundScanSchedulerTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				s.logger.Error("background library scan scheduler", err)
			}
		}
	}
}

func (s *BackgroundScanService) tick(ctx context.Context) error {
	profiles, err := s.profiles.List(ctx)
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}
	for _, profile := range profiles {
		if profile == nil || profile.ID == "" {
			continue
		}
		profileCtx := core.WithProfile(ctx, profile)
		if err := s.tickProfile(profileCtx, profile.ID); err != nil {
			s.recordProfileError(profile.ID, err)
			s.logger.Error("background library scan profile", err, "profile_id", profile.ID)
		}
	}
	return nil
}

func (s *BackgroundScanService) tickProfile(ctx context.Context, profileID string) error {
	config, err := s.loadConfig(ctx)
	if err != nil {
		return err
	}
	now := s.now().UTC()

	s.mu.Lock()
	state := s.ensureStateLocked(profileID, config, now)
	state.status.LibraryScanScheduleConfig = config
	activeJobID := state.activeJobID
	s.mu.Unlock()

	if activeJobID != "" {
		job := s.coordinator.ScanJobStatus(activeJobID)
		if job == nil {
			s.finishJob(profileID, nil, config, now, "background scan job disappeared")
			return nil
		}
		if !scanJobTerminal(job.Status) {
			s.mu.Lock()
			state = s.ensureStateLocked(profileID, config, now)
			state.status.State = "running"
			state.status.ActiveJob = job
			s.mu.Unlock()
			return nil
		}
		s.finishJob(profileID, job, config, now, "")
		return nil
	}
	if !config.Enabled {
		s.mu.Lock()
		state = s.ensureStateLocked(profileID, config, now)
		state.status.State = "disabled"
		state.status.NextRunAt = ""
		state.status.ActiveJob = nil
		s.mu.Unlock()
		return nil
	}

	s.mu.RLock()
	nextRunAt := s.states[profileID].status.NextRunAt
	s.mu.RUnlock()
	nextRun, err := time.Parse(time.RFC3339, nextRunAt)
	if err != nil {
		return fmt.Errorf("parse next background scan time: %w", err)
	}
	if now.Before(nextRun) {
		return nil
	}

	job, alreadyRunning, err := s.coordinator.StartScan(ctx, ScanRequest{Trigger: "background"})
	if err != nil {
		return fmt.Errorf("start scan: %w", err)
	}
	if alreadyRunning || job == nil {
		s.mu.Lock()
		state = s.ensureStateLocked(profileID, config, now)
		state.status.State = "waiting"
		state.status.NextRunAt = now.Add(backgroundScanBusyRetryDelay).Format(time.RFC3339)
		s.mu.Unlock()
		return nil
	}

	s.mu.Lock()
	state = s.ensureStateLocked(profileID, config, now)
	state.activeJobID = job.JobID
	state.status.State = "running"
	state.status.LastStartedAt = job.StartedAt
	state.status.NextRunAt = ""
	state.status.ActiveJob = job
	s.mu.Unlock()
	events.PublishJSON(s.eventBus, "background_scan_started", map[string]any{
		"profile_id": profileID,
		"job_id":     job.JobID,
	})
	return nil
}

func (s *BackgroundScanService) ensureStateLocked(profileID string, config core.LibraryScanScheduleConfig, now time.Time) *backgroundScanProfileState {
	state := s.states[profileID]
	if state == nil {
		state = &backgroundScanProfileState{status: core.LibraryScanScheduleStatus{
			LibraryScanScheduleConfig: config,
			State:                     "scheduled",
			NextRunAt:                 now.Add(backgroundScanInitialDelay).Format(time.RFC3339),
		}}
		s.states[profileID] = state
	}
	return state
}

func (s *BackgroundScanService) finishJob(profileID string, job *core.ScanJobStatus, config core.LibraryScanScheduleConfig, now time.Time, fallbackError string) {
	s.mu.Lock()
	state := s.ensureStateLocked(profileID, config, now)
	state.activeJobID = ""
	state.status.ActiveJob = nil
	state.status.State = "scheduled"
	state.status.LastFinishedAt = now.Format(time.RFC3339)
	if config.Enabled {
		state.status.NextRunAt = now.Add(time.Duration(config.IntervalMinutes) * time.Minute).Format(time.RFC3339)
	} else {
		state.status.State = "disabled"
		state.status.NextRunAt = ""
	}
	state.status.LastError = fallbackError
	if fallbackError != "" {
		state.status.LastStatus = "failed"
	}
	if job != nil {
		state.status.LastStatus = job.Status
		if job.FinishedAt != "" {
			state.status.LastFinishedAt = job.FinishedAt
		}
		state.status.LastError = job.Error
	}
	status := state.status
	s.mu.Unlock()
	events.PublishJSON(s.eventBus, "background_scan_finished", map[string]any{
		"profile_id":  profileID,
		"status":      status.LastStatus,
		"error":       status.LastError,
		"next_run_at": status.NextRunAt,
	})
}

func (s *BackgroundScanService) recordProfileError(profileID string, err error) {
	now := s.now().UTC()
	s.mu.Lock()
	state := s.states[profileID]
	if state == nil {
		state = &backgroundScanProfileState{}
		s.states[profileID] = state
	}
	state.status.State = "error"
	state.status.LastStatus = "failed"
	state.status.LastError = err.Error()
	state.status.NextRunAt = now.Add(backgroundScanBusyRetryDelay).Format(time.RFC3339)
	s.mu.Unlock()
}

func (s *BackgroundScanService) loadConfig(ctx context.Context) (core.LibraryScanScheduleConfig, error) {
	config := core.LibraryScanScheduleConfig{Enabled: true, IntervalMinutes: defaultLibraryScanInterval}
	setting, err := s.settings.Get(ctx, libraryScanScheduleSetting)
	if err != nil {
		return config, fmt.Errorf("load schedule setting: %w", err)
	}
	if setting == nil || setting.Value == "" {
		return config, nil
	}
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		return config, fmt.Errorf("parse schedule setting: %w", err)
	}
	if err := validateLibraryScanConfig(config); err != nil {
		return config, err
	}
	return config, nil
}

func validateLibraryScanConfig(config core.LibraryScanScheduleConfig) error {
	if config.IntervalMinutes < minimumLibraryScanInterval || config.IntervalMinutes > maximumLibraryScanInterval {
		return fmt.Errorf("interval_minutes must be between %d and %d", minimumLibraryScanInterval, maximumLibraryScanInterval)
	}
	return nil
}

func (s *BackgroundScanService) Status(ctx context.Context) (*core.LibraryScanScheduleStatus, error) {
	profileID := core.ProfileIDFromContext(ctx)
	if profileID == "" {
		return nil, fmt.Errorf("profile is required")
	}
	config, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	s.mu.Lock()
	state := s.ensureStateLocked(profileID, config, now)
	state.status.LibraryScanScheduleConfig = config
	status := state.status
	activeJobID := state.activeJobID
	s.mu.Unlock()
	if activeJobID != "" {
		status.ActiveJob = s.coordinator.ScanJobStatus(activeJobID)
	}
	return &status, nil
}

func (s *BackgroundScanService) UpdateConfig(ctx context.Context, config core.LibraryScanScheduleConfig) (*core.LibraryScanScheduleStatus, error) {
	profileID := core.ProfileIDFromContext(ctx)
	if profileID == "" {
		return nil, fmt.Errorf("profile is required")
	}
	if err := validateLibraryScanConfig(config); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode schedule setting: %w", err)
	}
	if err := s.settings.Upsert(ctx, &core.Setting{Key: libraryScanScheduleSetting, Value: string(raw), UpdatedAt: s.now()}); err != nil {
		return nil, fmt.Errorf("save schedule setting: %w", err)
	}
	now := s.now().UTC()
	s.mu.Lock()
	state := s.ensureStateLocked(profileID, config, now)
	state.status.LibraryScanScheduleConfig = config
	if config.Enabled {
		if state.activeJobID == "" {
			state.status.State = "scheduled"
			state.status.NextRunAt = now.Add(backgroundScanInitialDelay).Format(time.RFC3339)
		} else {
			state.status.State = "running"
		}
	} else {
		if state.activeJobID == "" {
			state.status.State = "disabled"
		} else {
			state.status.State = "running"
		}
		state.status.NextRunAt = ""
	}
	status := state.status
	s.mu.Unlock()
	events.PublishJSON(s.eventBus, "background_scan_schedule_updated", map[string]any{
		"profile_id":       profileID,
		"enabled":          config.Enabled,
		"interval_minutes": config.IntervalMinutes,
		"next_run_at":      status.NextRunAt,
	})
	return &status, nil
}

func (c *DiscoveryController) SetBackgroundScanService(service *BackgroundScanService) {
	c.backgroundScans = service
}

func (c *DiscoveryController) ScanJobStatus(jobID string) *core.ScanJobStatus {
	return c.scanJobs.Get(jobID)
}

func (c *DiscoveryController) GetBackgroundScanStatus(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if c.backgroundScans == nil {
		stdhttp.Error(w, "background library scans are unavailable", stdhttp.StatusServiceUnavailable)
		return
	}
	status, err := c.backgroundScans.Status(r.Context())
	if err != nil {
		stdhttp.Error(w, err.Error(), stdhttp.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (c *DiscoveryController) SetBackgroundScanConfig(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if c.backgroundScans == nil {
		stdhttp.Error(w, "background library scans are unavailable", stdhttp.StatusServiceUnavailable)
		return
	}
	var config core.LibraryScanScheduleConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		stdhttp.Error(w, "invalid JSON", stdhttp.StatusBadRequest)
		return
	}
	status, err := c.backgroundScans.UpdateConfig(r.Context(), config)
	if err != nil {
		stdhttp.Error(w, err.Error(), stdhttp.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}
