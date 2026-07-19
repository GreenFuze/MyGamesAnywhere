package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maxAchievementRefreshWarnings = 20

type achievementRefreshRunner interface {
	RefreshAll(ctx context.Context, callbacks scan.AchievementRefreshCallbacks) (*scan.AchievementRefreshResult, error)
}

type achievementRefreshJobRecord struct {
	profileID string
	status    *core.AchievementRefreshJobStatus
}

type achievementRefreshJobManager struct {
	runner achievementRefreshRunner
	bus    *events.EventBus
	logger core.Logger

	mu          sync.RWMutex
	activeJobID string
	jobs        map[string]*achievementRefreshJobRecord
}

func newAchievementRefreshJobManager(runner achievementRefreshRunner, bus *events.EventBus, logger core.Logger) *achievementRefreshJobManager {
	return &achievementRefreshJobManager{
		runner: runner,
		bus:    bus,
		logger: logger,
		jobs:   make(map[string]*achievementRefreshJobRecord),
	}
}

func (m *achievementRefreshJobManager) Start(parent context.Context, trigger string) (*core.AchievementRefreshJobStatus, bool, error) {
	profileID := core.ProfileIDFromContext(parent)
	if profileID == "" {
		return nil, false, ErrProfileRequired
	}
	m.mu.Lock()
	if m.activeJobID != "" {
		if record := m.jobs[m.activeJobID]; record != nil && !achievementRefreshTerminal(record.status.Status) {
			if record.profileID != profileID {
				m.mu.Unlock()
				return nil, true, ErrProfileJobBusy
			}
			out := cloneAchievementRefreshJobStatus(record.status)
			m.mu.Unlock()
			return out, true, nil
		}
		m.activeJobID = ""
	}

	ctx := context.Background()
	if profile, ok := core.ProfileFromContext(parent); ok {
		ctx = core.WithProfile(ctx, profile)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	status := &core.AchievementRefreshJobStatus{
		JobID:     uuid.NewString(),
		Status:    "queued",
		StartedAt: now,
		Trigger:   trigger,
	}
	m.jobs[status.JobID] = &achievementRefreshJobRecord{profileID: profileID, status: status}
	m.activeJobID = status.JobID
	out := cloneAchievementRefreshJobStatus(status)
	m.mu.Unlock()

	go m.run(ctx, status.JobID)
	return out, false, nil
}

func (m *achievementRefreshJobManager) Get(jobID string) *core.AchievementRefreshJobStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record := m.jobs[jobID]
	if record == nil {
		return nil
	}
	return cloneAchievementRefreshJobStatus(record.status)
}

func (m *achievementRefreshJobManager) GetForProfile(profileID, jobID string) *core.AchievementRefreshJobStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record := m.jobs[jobID]
	if record == nil || record.profileID != profileID {
		return nil
	}
	return cloneAchievementRefreshJobStatus(record.status)
}

func (m *achievementRefreshJobManager) run(ctx context.Context, jobID string) {
	m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
		status.Status = "running"
	})
	m.publish(jobID, "achievement_refresh_started", cloneAchievementRefreshJobStatus(m.Get(jobID)))

	result, err := m.runner.RefreshAll(ctx, scan.AchievementRefreshCallbacks{
		SetTotal: func(total int) {
			m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
				status.ItemsTotal = total
			})
		},
		Progress: func(completed, total int, item string) {
			m.applyProgress(jobID, scan.AchievementRefreshProgress{Completed: completed, Total: total, Item: item})
		},
		ProgressDetail: func(progress scan.AchievementRefreshProgress) {
			m.applyProgress(jobID, progress)
		},
		Waiting: func(wait scan.AchievementRefreshWait) {
			m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
				status.ProviderID = wait.ProviderID
				status.ProviderLabel = wait.ProviderLabel
				status.ItemsCompleted = wait.Completed
				if wait.Total > 0 {
					status.ItemsTotal = wait.Total
				}
				status.CurrentItem = wait.Item
				status.WaitingUntil = wait.WaitingUntil
				status.Message = wait.Message
			})
			m.publish(jobID, "achievement_refresh_waiting", map[string]any{
				"job_id":          jobID,
				"provider_id":     wait.ProviderID,
				"provider_label":  wait.ProviderLabel,
				"items_completed": wait.Completed,
				"items_total":     wait.Total,
				"current_item":    wait.Item,
				"waiting_until":   wait.WaitingUntil,
				"message":         wait.Message,
			})
		},
		Warning: func(message string) {
			m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
				status.WarningCount++
				status.ErrorCount++
				if len(status.Warnings) < maxAchievementRefreshWarnings {
					status.Warnings = append(status.Warnings, message)
				}
			})
			m.publish(jobID, "achievement_refresh_warning", map[string]any{
				"job_id":  jobID,
				"message": message,
			})
		},
		Skipped: func(item string) {
			m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
				status.SkippedCount++
			})
		},
	})

	finishedAt := time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
			status.Status = "failed"
			status.ErrorCount++
			status.Error = err.Error()
			status.FinishedAt = finishedAt
		})
		m.publish(jobID, "achievement_refresh_failed", map[string]any{
			"job_id":      jobID,
			"error":       err.Error(),
			"finished_at": finishedAt,
		})
	} else {
		m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
			status.Status = "completed"
			status.FinishedAt = finishedAt
			status.CurrentItem = ""
			status.WaitingUntil = ""
			status.Message = ""
			if result != nil {
				status.SuccessCount = result.Success
				status.SkippedCount = result.Skipped
				status.WarningCount = result.Failed
				status.ErrorCount = result.Failed
				status.ItemsTotal = result.Targets
				status.ItemsCompleted = result.Targets
			}
		})
		m.publish(jobID, "achievement_refresh_completed", map[string]any{
			"job_id":          jobID,
			"items_total":     m.Get(jobID).ItemsTotal,
			"items_completed": m.Get(jobID).ItemsCompleted,
			"success_count":   m.Get(jobID).SuccessCount,
			"skipped_count":   m.Get(jobID).SkippedCount,
			"warning_count":   m.Get(jobID).WarningCount,
			"finished_at":     finishedAt,
		})
	}

	m.mu.Lock()
	if m.activeJobID == jobID {
		m.activeJobID = ""
	}
	m.mu.Unlock()
}

func (m *achievementRefreshJobManager) applyProgress(jobID string, progress scan.AchievementRefreshProgress) {
	m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
		status.ItemsCompleted = progress.Completed
		if progress.Total > 0 {
			status.ItemsTotal = progress.Total
		}
		status.CurrentItem = progress.Item
		if progress.ProviderID != "" {
			status.ProviderID = progress.ProviderID
		}
		if progress.ProviderLabel != "" {
			status.ProviderLabel = progress.ProviderLabel
		}
		status.Message = progress.Message
		status.WaitingUntil = progress.WaitingUntil
	})
	m.publish(jobID, "achievement_refresh_progress", map[string]any{
		"job_id":          jobID,
		"provider_id":     progress.ProviderID,
		"provider_label":  progress.ProviderLabel,
		"items_completed": progress.Completed,
		"items_total":     progress.Total,
		"current_item":    progress.Item,
		"message":         progress.Message,
		"waiting_until":   progress.WaitingUntil,
	})
}

func (m *achievementRefreshJobManager) update(jobID string, mutate func(*core.AchievementRefreshJobStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record := m.jobs[jobID]
	if record == nil {
		return
	}
	mutate(record.status)
}

func (m *achievementRefreshJobManager) publish(jobID, eventType string, payload any) {
	if m.bus == nil {
		return
	}
	data, err := marshalProfileOwnedJobEvent(payload, m.profileID(jobID))
	if err != nil {
		m.logger.Warn("achievement refresh: event marshal failed", "error", err)
		return
	}
	m.bus.Publish(events.Event{Type: eventType, Data: data})
}

func (m *achievementRefreshJobManager) profileID(jobID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if record := m.jobs[jobID]; record != nil {
		return record.profileID
	}
	return ""
}

func cloneAchievementRefreshJobStatus(status *core.AchievementRefreshJobStatus) *core.AchievementRefreshJobStatus {
	if status == nil {
		return nil
	}
	clone := *status
	if len(status.Warnings) > 0 {
		clone.Warnings = append([]string(nil), status.Warnings...)
	}
	return &clone
}

func achievementRefreshTerminal(status string) bool {
	return status == "completed" || status == "failed"
}

type AchievementRefreshController struct {
	jobs *achievementRefreshJobManager
}

func NewAchievementRefreshController(runner achievementRefreshRunner, eventBus *events.EventBus, logger core.Logger) *AchievementRefreshController {
	return &AchievementRefreshController{
		jobs: newAchievementRefreshJobManager(runner, eventBus, logger),
	}
}

func (c *AchievementRefreshController) Start(w http.ResponseWriter, r *http.Request) {
	status, alreadyRunning, err := c.StartAutomatic(r.Context(), "manual")
	if err != nil {
		if errors.Is(err, ErrProfileJobBusy) {
			http.Error(w, "achievement refresh is busy", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if alreadyRunning {
		w.WriteHeader(http.StatusConflict)
	} else {
		w.WriteHeader(http.StatusAccepted)
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (c *AchievementRefreshController) StartAutomatic(ctx context.Context, trigger string) (*core.AchievementRefreshJobStatus, bool, error) {
	if c == nil || c.jobs == nil {
		return nil, false, nil
	}
	return c.jobs.Start(ctx, trigger)
}

func (c *AchievementRefreshController) GetJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	if jobID == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}
	status := c.jobs.GetForProfile(core.ProfileIDFromContext(r.Context()), jobID)
	if status == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}
