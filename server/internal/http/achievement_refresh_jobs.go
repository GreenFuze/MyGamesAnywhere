package http

import (
	"context"
	"encoding/json"
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
	status *core.AchievementRefreshJobStatus
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
	m.mu.Lock()
	if m.activeJobID != "" {
		if record := m.jobs[m.activeJobID]; record != nil && !achievementRefreshTerminal(record.status.Status) {
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
	m.jobs[status.JobID] = &achievementRefreshJobRecord{status: status}
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

func (m *achievementRefreshJobManager) run(ctx context.Context, jobID string) {
	m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
		status.Status = "running"
	})
	m.publish("achievement_refresh_started", cloneAchievementRefreshJobStatus(m.Get(jobID)))

	result, err := m.runner.RefreshAll(ctx, scan.AchievementRefreshCallbacks{
		SetTotal: func(total int) {
			m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
				status.ItemsTotal = total
			})
		},
		Progress: func(completed, total int, item string) {
			m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
				status.ItemsCompleted = completed
				if total > 0 {
					status.ItemsTotal = total
				}
				status.CurrentItem = item
			})
			m.publish("achievement_refresh_progress", map[string]any{
				"job_id":          jobID,
				"items_completed": completed,
				"items_total":     total,
				"current_item":    item,
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
			m.publish("achievement_refresh_warning", map[string]any{
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
		m.publish("achievement_refresh_failed", map[string]any{
			"job_id":      jobID,
			"error":       err.Error(),
			"finished_at": finishedAt,
		})
	} else {
		m.update(jobID, func(status *core.AchievementRefreshJobStatus) {
			status.Status = "completed"
			status.FinishedAt = finishedAt
			status.CurrentItem = ""
			if result != nil {
				status.SuccessCount = result.Success
				status.SkippedCount = result.Skipped
				status.WarningCount = result.Failed
				status.ErrorCount = result.Failed
				status.ItemsTotal = result.Targets
				status.ItemsCompleted = result.Targets
			}
		})
		m.publish("achievement_refresh_completed", map[string]any{
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

func (m *achievementRefreshJobManager) update(jobID string, mutate func(*core.AchievementRefreshJobStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record := m.jobs[jobID]
	if record == nil {
		return
	}
	mutate(record.status)
}

func (m *achievementRefreshJobManager) publish(eventType string, payload any) {
	if m.bus == nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		m.logger.Warn("achievement refresh: event marshal failed", "error", err)
		return
	}
	m.bus.Publish(events.Event{Type: eventType, Data: data})
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
	status := c.jobs.Get(jobID)
	if status == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}
