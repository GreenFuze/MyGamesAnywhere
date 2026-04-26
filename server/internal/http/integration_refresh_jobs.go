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

const maxIntegrationRefreshWarnings = 20

type integrationRefreshRunner interface {
	RunIntegrationRefresh(ctx context.Context, integration *core.Integration, callbacks scan.IntegrationRefreshCallbacks) error
}

type integrationRefreshJobRecord struct {
	status *core.IntegrationRefreshJobStatus
}

type integrationRefreshJobManager struct {
	runner integrationRefreshRunner
	bus    *events.EventBus
	logger core.Logger

	mu                  sync.RWMutex
	jobs                map[string]*integrationRefreshJobRecord
	activeByIntegration map[string]string
}

func newIntegrationRefreshJobManager(runner integrationRefreshRunner, bus *events.EventBus, logger core.Logger) *integrationRefreshJobManager {
	return &integrationRefreshJobManager{
		runner:              runner,
		bus:                 bus,
		logger:              logger,
		jobs:                make(map[string]*integrationRefreshJobRecord),
		activeByIntegration: make(map[string]string),
	}
}

func (m *integrationRefreshJobManager) Start(integration *core.Integration) (*core.IntegrationRefreshJobStatus, bool, error) {
	if integration == nil || integration.ID == "" {
		return nil, false, http.ErrMissingFile
	}

	m.mu.Lock()
	if activeJobID, ok := m.activeByIntegration[integration.ID]; ok {
		if record := m.jobs[activeJobID]; record != nil && !integrationRefreshTerminal(record.status.Status) {
			out := cloneIntegrationRefreshJobStatus(record.status)
			m.mu.Unlock()
			return out, true, nil
		}
		delete(m.activeByIntegration, integration.ID)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	status := &core.IntegrationRefreshJobStatus{
		JobID:         uuid.NewString(),
		IntegrationID: integration.ID,
		PluginID:      integration.PluginID,
		Label:         integration.Label,
		Status:        "queued",
		StartedAt:     now,
	}
	m.jobs[status.JobID] = &integrationRefreshJobRecord{status: status}
	m.activeByIntegration[integration.ID] = status.JobID
	out := cloneIntegrationRefreshJobStatus(status)
	m.mu.Unlock()

	go m.run(integration, status.JobID)
	return out, false, nil
}

func (m *integrationRefreshJobManager) Get(jobID string) *core.IntegrationRefreshJobStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record := m.jobs[jobID]
	if record == nil {
		return nil
	}
	return cloneIntegrationRefreshJobStatus(record.status)
}

func (m *integrationRefreshJobManager) run(integration *core.Integration, jobID string) {
	ctx := context.Background()
	m.update(jobID, func(status *core.IntegrationRefreshJobStatus) {
		status.Status = "running"
		status.Phase = "starting"
	})
	m.publish("integration_refresh_started", cloneIntegrationRefreshJobStatus(m.Get(jobID)))

	err := m.runner.RunIntegrationRefresh(ctx, integration, scan.IntegrationRefreshCallbacks{
		SetPhase: func(phase string, total int) {
			m.update(jobID, func(status *core.IntegrationRefreshJobStatus) {
				status.Phase = phase
				status.ItemsTotal = total
				status.ItemsCompleted = 0
				status.CurrentItem = ""
			})
			m.publish("integration_refresh_phase", map[string]any{
				"job_id":         jobID,
				"integration_id": integration.ID,
				"phase":          phase,
				"items_total":    total,
			})
		},
		Progress: func(completed, total int, item string) {
			m.update(jobID, func(status *core.IntegrationRefreshJobStatus) {
				status.ItemsCompleted = completed
				if total > 0 {
					status.ItemsTotal = total
				}
				status.CurrentItem = item
			})
			m.publish("integration_refresh_progress", map[string]any{
				"job_id":          jobID,
				"integration_id":  integration.ID,
				"phase":           m.Get(jobID).Phase,
				"items_completed": completed,
				"items_total":     total,
				"current_item":    item,
			})
		},
		Warning: func(message string) {
			m.update(jobID, func(status *core.IntegrationRefreshJobStatus) {
				status.WarningCount++
				if len(status.Warnings) < maxIntegrationRefreshWarnings {
					status.Warnings = append(status.Warnings, message)
				}
			})
			m.publish("integration_refresh_warning", map[string]any{
				"job_id":         jobID,
				"integration_id": integration.ID,
				"message":        message,
			})
		},
	})

	finishedAt := time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		m.update(jobID, func(status *core.IntegrationRefreshJobStatus) {
			status.Status = "failed"
			status.ErrorCount++
			status.Error = err.Error()
			status.FinishedAt = finishedAt
		})
		m.publish("integration_refresh_failed", map[string]any{
			"job_id":         jobID,
			"integration_id": integration.ID,
			"error":          err.Error(),
			"finished_at":    finishedAt,
		})
	} else {
		m.update(jobID, func(status *core.IntegrationRefreshJobStatus) {
			status.Status = "completed"
			status.FinishedAt = finishedAt
			status.CurrentItem = ""
		})
		m.publish("integration_refresh_complete", map[string]any{
			"job_id":          jobID,
			"integration_id":  integration.ID,
			"warning_count":   m.Get(jobID).WarningCount,
			"items_total":     m.Get(jobID).ItemsTotal,
			"items_completed": m.Get(jobID).ItemsCompleted,
			"finished_at":     finishedAt,
		})
	}

	m.mu.Lock()
	if current := m.activeByIntegration[integration.ID]; current == jobID {
		delete(m.activeByIntegration, integration.ID)
	}
	m.mu.Unlock()
}

func (m *integrationRefreshJobManager) update(jobID string, mutate func(*core.IntegrationRefreshJobStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record := m.jobs[jobID]
	if record == nil {
		return
	}
	mutate(record.status)
}

func (m *integrationRefreshJobManager) publish(eventType string, payload any) {
	if m.bus == nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		m.logger.Warn("integration refresh: event marshal failed", "error", err)
		return
	}
	m.bus.Publish(events.Event{Type: eventType, Data: data})
}

func cloneIntegrationRefreshJobStatus(status *core.IntegrationRefreshJobStatus) *core.IntegrationRefreshJobStatus {
	if status == nil {
		return nil
	}
	clone := *status
	if len(status.Warnings) > 0 {
		clone.Warnings = append([]string(nil), status.Warnings...)
	}
	return &clone
}

func integrationRefreshTerminal(status string) bool {
	return status == "completed" || status == "failed"
}

type integrationRefreshPluginDiscovery interface {
	GetPlugin(pluginID string) (*core.Plugin, bool)
}

type IntegrationRefreshController struct {
	repo       core.IntegrationRepository
	pluginHost integrationRefreshPluginDiscovery
	jobs       *integrationRefreshJobManager
}

func NewIntegrationRefreshController(repo core.IntegrationRepository, pluginHost integrationRefreshPluginDiscovery, runner integrationRefreshRunner, eventBus *events.EventBus, logger core.Logger) *IntegrationRefreshController {
	return &IntegrationRefreshController{
		repo:       repo,
		pluginHost: pluginHost,
		jobs:       newIntegrationRefreshJobManager(runner, eventBus, logger),
	}
}

func (c *IntegrationRefreshController) Start(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	integration, err := c.repo.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if integration == nil {
		http.NotFound(w, r)
		return
	}

	plugin, ok := c.pluginHost.GetPlugin(integration.PluginID)
	if !ok {
		http.Error(w, "plugin not found", http.StatusBadRequest)
		return
	}
	if !pluginProvidesMethod(plugin, "metadata.game.lookup") && !pluginProvidesMethod(plugin, "achievements.game.get") {
		http.Error(w, "integration has no refreshable derived data", http.StatusBadRequest)
		return
	}

	status, alreadyRunning, err := c.jobs.Start(integration)
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

func (c *IntegrationRefreshController) GetJob(w http.ResponseWriter, r *http.Request) {
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

func pluginProvidesMethod(plugin *core.Plugin, method string) bool {
	if plugin == nil {
		return false
	}
	for _, provide := range plugin.Manifest.Provides {
		if provide == method {
			return true
		}
	}
	return false
}
