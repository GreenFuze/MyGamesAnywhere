package http

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
	"github.com/google/uuid"
)

const maxRecentScanEvents = 30

type scanRunner interface {
	RunScan(ctx context.Context, integrationIDs []string) ([]*core.CanonicalGame, error)
	RunMetadataRefresh(ctx context.Context, integrationIDs []string) ([]*core.CanonicalGame, error)
}

type scanCancelResult int

const (
	scanCancelAccepted scanCancelResult = iota
	scanCancelNoop
	scanCancelConflict
	scanCancelNotFound
)

type scanJobRecord struct {
	status                *core.ScanJobStatus
	cancel                context.CancelFunc
	cancelRequested       bool
	completedIntegrations map[string]bool
}

type scanJobManager struct {
	runner scanRunner
	bus    *events.EventBus
	logger core.Logger

	mu          sync.RWMutex
	activeJobID string
	jobs        map[string]*scanJobRecord
}

func newScanJobManager(runner scanRunner, bus *events.EventBus, logger core.Logger) *scanJobManager {
	return &scanJobManager{
		runner: runner,
		bus:    bus,
		logger: logger,
		jobs:   make(map[string]*scanJobRecord),
	}
}

func (m *scanJobManager) Start(req ScanRequest) (*core.ScanJobStatus, bool, error) {
	m.mu.Lock()
	if current := m.activeRunningJobLocked(); current != nil {
		out := cloneScanJobStatus(current.status)
		m.mu.Unlock()
		return out, true, nil
	}

	jobID := uuid.NewString()
	ctx, cancel := context.WithCancel(scan.WithScanJobID(context.Background(), jobID))
	now := time.Now().UTC().Format(time.RFC3339)
	job := &core.ScanJobStatus{
		JobID:          jobID,
		Status:         "queued",
		MetadataOnly:   req.MetadataOnly,
		IntegrationIDs: append([]string(nil), req.GameSources...),
		StartedAt:      now,
		CurrentPhase:   "queued",
	}
	for _, integrationID := range req.GameSources {
		job.Integrations = append(job.Integrations, core.ScanJobIntegrationStatus{
			IntegrationID: integrationID,
			Status:        "pending",
			Phase:         "queued",
		})
	}
	job.IntegrationCount = len(job.Integrations)

	record := &scanJobRecord{
		status:                job,
		cancel:                cancel,
		completedIntegrations: make(map[string]bool),
	}
	m.jobs[jobID] = record
	m.activeJobID = jobID
	out := cloneScanJobStatus(job)
	m.mu.Unlock()

	go m.run(jobID, req, ctx)
	return out, false, nil
}

func (m *scanJobManager) Get(jobID string) *core.ScanJobStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record := m.jobs[jobID]
	if record == nil {
		return nil
	}
	return cloneScanJobStatus(record.status)
}

func (m *scanJobManager) Cancel(jobID string) (*core.ScanJobStatus, scanCancelResult) {
	m.mu.Lock()
	record := m.jobs[jobID]
	if record == nil {
		m.mu.Unlock()
		return nil, scanCancelNotFound
	}

	switch record.status.Status {
	case "completed", "failed":
		status := cloneScanJobStatus(record.status)
		m.mu.Unlock()
		return status, scanCancelConflict
	case "cancelling", "cancelled":
		status := cloneScanJobStatus(record.status)
		m.mu.Unlock()
		return status, scanCancelNoop
	}

	record.cancelRequested = true
	record.status.Status = "cancelling"
	record.status.CurrentPhase = "cancelling"
	cancel := record.cancel
	status := cloneScanJobStatus(record.status)
	m.mu.Unlock()

	if cancel != nil {
		events.PublishJSON(m.bus, "scan_cancel_requested", map[string]any{
			"job_id":               jobID,
			"integration_id":       status.CurrentIntegrationID,
			"label":                status.CurrentIntegrationLabel,
			"current_phase":        status.CurrentPhase,
			"integrations_pending": maxInt(0, status.IntegrationCount-status.IntegrationsCompleted),
		})
		cancel()
		return status, scanCancelAccepted
	}

	events.PublishJSON(m.bus, "scan_cancel_requested", map[string]any{
		"job_id":               jobID,
		"integration_id":       status.CurrentIntegrationID,
		"label":                status.CurrentIntegrationLabel,
		"current_phase":        status.CurrentPhase,
		"integrations_pending": maxInt(0, status.IntegrationCount-status.IntegrationsCompleted),
	})

	return status, scanCancelAccepted
}

func (m *scanJobManager) run(jobID string, req ScanRequest, ctx context.Context) {
	done := make(chan struct{})
	var sub <-chan events.Event
	if m.bus != nil {
		sub = m.bus.Subscribe()
		go m.watch(jobID, sub, done)
	}

	m.update(jobID, func(record *scanJobRecord) {
		if record.status.Status == "queued" {
			record.status.Status = "running"
			record.status.CurrentPhase = "starting"
		}
	})

	var err error
	if req.MetadataOnly {
		_, err = m.runner.RunMetadataRefresh(ctx, req.GameSources)
	} else {
		_, err = m.runner.RunScan(ctx, req.GameSources)
	}

	finishedAt := time.Now().UTC().Format(time.RFC3339)
	cancelled := false
	if err != nil {
		m.update(jobID, func(record *scanJobRecord) {
			record.status.FinishedAt = finishedAt
			record.cancel = nil
			if errors.Is(err, context.Canceled) && record.cancelRequested {
				record.status.Status = "cancelled"
				record.status.CurrentPhase = "cancelled"
				cancelled = true
				if integ := currentIntegrationStatus(record.status); integ != nil && !scanIntegrationTerminal(integ.Status) {
					integ.Status = "cancelled"
					integ.Phase = "cancelled"
				}
				return
			}
			record.status.Status = "failed"
			record.status.Error = err.Error()
			record.status.CurrentPhase = "failed"
			if integ := currentIntegrationStatus(record.status); integ != nil && !scanIntegrationTerminal(integ.Status) {
				integ.Status = "failed"
				integ.Phase = "failed"
				if integ.Error == "" {
					integ.Error = err.Error()
				}
			}
		})
	} else {
		m.update(jobID, func(record *scanJobRecord) {
			record.status.Status = "completed"
			record.status.CurrentPhase = ""
			record.status.FinishedAt = finishedAt
			record.cancel = nil
		})
	}

	if cancelled {
		m.update(jobID, func(record *scanJobRecord) {
			applyScanEvent(record, "scan_cancelled", map[string]any{
				"job_id":      jobID,
				"finished_at": finishedAt,
			})
		})
		events.PublishJSON(m.bus, "scan_cancelled", map[string]any{
			"job_id":        jobID,
			"finished_at":   finishedAt,
			"current_phase": "cancelled",
		})
	}

	close(done)
	if m.bus != nil && sub != nil {
		m.bus.Unsubscribe(sub)
	}

	m.mu.Lock()
	if m.activeJobID == jobID {
		m.activeJobID = ""
	}
	m.mu.Unlock()
}

func (m *scanJobManager) watch(jobID string, sub <-chan events.Event, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case ev, ok := <-sub:
			if !ok {
				return
			}
			if !strings.HasPrefix(ev.Type, "scan_") {
				continue
			}
			payload := make(map[string]any)
			if err := json.Unmarshal(ev.Data, &payload); err != nil {
				continue
			}
			if readString(payload["job_id"]) != jobID {
				continue
			}
			m.update(jobID, func(record *scanJobRecord) {
				applyScanEvent(record, ev.Type, payload)
			})
		}
	}
}

func (m *scanJobManager) update(jobID string, mutate func(*scanJobRecord)) {
	m.mu.Lock()
	record := m.jobs[jobID]
	if record != nil {
		mutate(record)
	}
	m.mu.Unlock()
}

func (m *scanJobManager) activeRunningJobLocked() *scanJobRecord {
	if m.activeJobID == "" {
		return nil
	}
	record := m.jobs[m.activeJobID]
	if record == nil {
		m.activeJobID = ""
		return nil
	}
	if scanJobTerminal(record.status.Status) {
		m.activeJobID = ""
		return nil
	}
	return record
}

func applyScanEvent(record *scanJobRecord, eventType string, payload map[string]any) {
	job := record.status
	if job.Status == "cancelled" && eventType == "scan_cancel_requested" {
		return
	}
	job.CurrentPhase = scanPhaseForEvent(eventType, payload)

	integrationID := readString(payload["integration_id"])
	integration := ensureIntegrationStatus(job, integrationID)
	if integration != nil {
		if integrationID != "" {
			job.CurrentIntegrationID = integrationID
		}
		if label := readString(payload["label"]); label != "" {
			integration.Label = label
			job.CurrentIntegrationLabel = label
		} else if integration.Label != "" {
			job.CurrentIntegrationLabel = integration.Label
		}
		if pluginID := readString(payload["plugin_id"]); pluginID != "" && eventUsesSourcePluginID(eventType) {
			integration.PluginID = pluginID
		}
	}

	metadataProvider := currentMetadataProviderStatus(integration)
	if integration != nil {
		metadataIntegrationID := readString(payload["metadata_integration_id"])
		metadataLabel := readString(payload["metadata_label"])
		metadataPluginID := readString(payload["plugin_id"])
		if metadataIntegrationID != "" || metadataLabel != "" || (metadataPluginID != "" && !eventUsesSourcePluginID(eventType)) {
			metadataProvider = ensureMetadataProviderStatus(integration, metadataIntegrationID, metadataLabel, metadataPluginID)
		}
	}

	switch eventType {
	case "scan_started":
		job.MetadataOnly = readBool(payload["metadata_only"])
		integrations := readIntegrationSnapshots(payload["integrations"])
		if len(integrations) > 0 {
			job.Integrations = integrations
			job.IntegrationIDs = make([]string, 0, len(integrations))
			for _, integration := range integrations {
				job.IntegrationIDs = append(job.IntegrationIDs, integration.IntegrationID)
			}
			job.IntegrationCount = len(integrations)
		} else if count := readInt(payload["integration_count"]); count > 0 {
			job.IntegrationCount = count
		}
	case "scan_integration_started":
		if integration != nil {
			integration.Status = "running"
			integration.Phase = "starting"
			integration.Reason = ""
			integration.Error = ""
		}
	case "scan_source_list_started":
		if integration != nil {
			integration.Status = "running"
			integration.Phase = "listing source content"
			integration.SourceProgress = &core.ScanJobProgress{
				Unit:          "items",
				Indeterminate: true,
			}
		}
	case "scan_source_list_complete":
		if integration != nil {
			integration.Phase = "source listing complete"
			if fileCount := readInt(payload["file_count"]); fileCount > 0 {
				integration.SourceProgress = &core.ScanJobProgress{Current: 0, Total: fileCount, Unit: "files"}
			} else if gameCount := readInt(payload["game_count"]); gameCount > 0 {
				integration.SourceProgress = &core.ScanJobProgress{Current: gameCount, Total: gameCount, Unit: "games"}
				integration.GamesFound = gameCount
			}
		}
	case "scan_scanner_started":
		if integration != nil {
			integration.Phase = "scanning files"
			integration.SourceProgress = &core.ScanJobProgress{
				Current: 0,
				Total:   readInt(payload["file_count"]),
				Unit:    "files",
			}
		}
	case "scan_scanner_progress":
		if integration != nil {
			integration.Phase = "scanning files"
			integration.SourceProgress = &core.ScanJobProgress{
				Current: readInt(payload["processed_count"]),
				Total:   readInt(payload["file_count"]),
				Unit:    "files",
			}
		}
	case "scan_scanner_complete":
		if integration != nil {
			integration.Phase = "game detection complete"
			if integration.SourceProgress != nil && integration.SourceProgress.Total > 0 {
				integration.SourceProgress.Current = integration.SourceProgress.Total
			}
		}
	case "scan_metadata_started":
		if integration != nil {
			integration.Phase = "metadata enrichment"
			integration.GamesFound = readInt(payload["game_count"])
			integration.MetadataProviders = readMetadataProviderSnapshots(payload["metadata_providers"])
			integration.MetadataIntegrationID = ""
			integration.MetadataLabel = ""
			integration.MetadataPluginID = ""
			if integration.GamesFound > 0 {
				integration.MetadataProgress = &core.ScanJobProgress{Current: 0, Total: integration.GamesFound, Unit: "games"}
			}
		}
	case "scan_metadata_phase":
		if integration != nil {
			integration.MetadataPhase = readString(payload["phase"])
			integration.Phase = scanPhaseForEvent(eventType, payload)
			integration.MetadataIntegrationID = ""
			integration.MetadataLabel = ""
			integration.MetadataPluginID = ""
		}
	case "scan_metadata_plugin_started":
		if integration != nil {
			integration.MetadataPhase = readString(payload["phase"])
			if metadataProvider != nil {
				metadataProvider.Status = "running"
				metadataProvider.Phase = integration.MetadataPhase
				metadataProvider.Reason = ""
				metadataProvider.Error = ""
				if batchSize := readInt(payload["batch_size"]); batchSize > 0 {
					metadataProvider.Progress = &core.ScanJobProgress{Current: 0, Total: batchSize, Unit: "games"}
				} else {
					metadataProvider.Progress = &core.ScanJobProgress{Unit: "games", Indeterminate: true}
				}
				integration.MetadataIntegrationID = metadataProvider.IntegrationID
				integration.MetadataLabel = metadataProvider.Label
				integration.MetadataPluginID = metadataProvider.PluginID
			} else {
				integration.MetadataPluginID = readString(payload["plugin_id"])
			}
			integration.Phase = scanPhaseForEvent(eventType, payload)
			if batchSize := readInt(payload["batch_size"]); batchSize > 0 {
				integration.MetadataProgress = &core.ScanJobProgress{Current: 0, Total: batchSize, Unit: "games"}
			} else {
				integration.MetadataProgress = &core.ScanJobProgress{Unit: "games", Indeterminate: true}
			}
		}
	case "scan_metadata_game_progress":
		if integration != nil {
			integration.MetadataPhase = readString(payload["phase"])
			if metadataProvider != nil {
				metadataProvider.Status = "running"
				metadataProvider.Phase = integration.MetadataPhase
				metadataProvider.Progress = &core.ScanJobProgress{
					Current: readInt(payload["game_index"]),
					Total:   readInt(payload["game_count"]),
					Unit:    "games",
				}
				integration.MetadataIntegrationID = metadataProvider.IntegrationID
				integration.MetadataLabel = metadataProvider.Label
				integration.MetadataPluginID = metadataProvider.PluginID
			} else {
				integration.MetadataPluginID = readString(payload["plugin_id"])
			}
			integration.Phase = scanPhaseForEvent(eventType, payload)
			integration.MetadataProgress = &core.ScanJobProgress{
				Current: readInt(payload["game_index"]),
				Total:   readInt(payload["game_count"]),
				Unit:    "games",
			}
		}
	case "scan_metadata_plugin_complete":
		if integration != nil {
			integration.MetadataPhase = readString(payload["phase"])
			if metadataProvider != nil {
				metadataProvider.Status = "completed"
				metadataProvider.Phase = integration.MetadataPhase
				switch {
				case readInt(payload["total"]) > 0:
					total := readInt(payload["total"])
					metadataProvider.Progress = &core.ScanJobProgress{Current: total, Total: total, Unit: "games"}
				case readInt(payload["candidates"]) > 0:
					total := readInt(payload["candidates"])
					metadataProvider.Progress = &core.ScanJobProgress{Current: total, Total: total, Unit: "games"}
				}
				integration.MetadataIntegrationID = metadataProvider.IntegrationID
				integration.MetadataLabel = metadataProvider.Label
				integration.MetadataPluginID = metadataProvider.PluginID
			} else {
				integration.MetadataPluginID = readString(payload["plugin_id"])
			}
			integration.Phase = scanPhaseForEvent(eventType, payload)
			switch {
			case readInt(payload["total"]) > 0:
				total := readInt(payload["total"])
				integration.MetadataProgress = &core.ScanJobProgress{Current: total, Total: total, Unit: "games"}
			case readInt(payload["candidates"]) > 0:
				total := readInt(payload["candidates"])
				integration.MetadataProgress = &core.ScanJobProgress{Current: total, Total: total, Unit: "games"}
			}
		}
	case "scan_metadata_plugin_error":
		if integration != nil {
			integration.MetadataPhase = readString(payload["phase"])
			if metadataProvider != nil {
				metadataProvider.Status = "error"
				metadataProvider.Phase = integration.MetadataPhase
				metadataProvider.Error = readString(payload["error"])
				integration.MetadataIntegrationID = metadataProvider.IntegrationID
				integration.MetadataLabel = metadataProvider.Label
				integration.MetadataPluginID = metadataProvider.PluginID
			} else {
				integration.MetadataPluginID = readString(payload["plugin_id"])
			}
			integration.Phase = scanPhaseForEvent(eventType, payload)
		}
	case "scan_metadata_consensus_complete":
		if integration != nil {
			integration.MetadataPhase = "consensus"
			integration.MetadataIntegrationID = ""
			integration.MetadataLabel = ""
			integration.MetadataPluginID = ""
			total := readInt(payload["identified"]) + readInt(payload["unidentified"])
			if total > 0 {
				integration.MetadataProgress = &core.ScanJobProgress{Current: total, Total: total, Unit: "games"}
			}
		}
	case "scan_metadata_finished":
		if integration != nil {
			integration.MetadataPhase = "finished"
			if readString(payload["status"]) == "degraded" {
				integration.Phase = "metadata degraded"
			}
			integration.MetadataIntegrationID = ""
			integration.MetadataLabel = ""
			integration.MetadataPluginID = ""
			markUnusedMetadataProviders(integration, "")
			total := readInt(payload["identified"]) + readInt(payload["unidentified"])
			if total > 0 {
				integration.MetadataProgress = &core.ScanJobProgress{Current: total, Total: total, Unit: "games"}
			}
		}
	case "scan_persist_started":
		if integration != nil {
			integration.Phase = "persisting results"
		}
	case "scan_integration_complete":
		if integration != nil {
			integration.Status = "completed"
			integration.Phase = "completed"
			integration.GamesFound = readInt(payload["games_found"])
			integration.Reason = ""
			integration.MetadataIntegrationID = ""
			integration.MetadataLabel = ""
			integration.MetadataPluginID = ""
		}
		markIntegrationComplete(record, integrationID)
	case "scan_integration_skipped":
		if integration != nil {
			integration.Status = "skipped"
			integration.Phase = "skipped"
			integration.Reason = readString(payload["reason"])
			integration.Error = readString(payload["error"])
			integration.SourceProgress = nil
			integration.MetadataPhase = ""
			integration.MetadataProgress = nil
			integration.MetadataIntegrationID = ""
			integration.MetadataLabel = ""
			integration.MetadataPluginID = ""
		}
		markIntegrationComplete(record, integrationID)
	case "scan_cancel_requested":
		if !scanJobTerminal(job.Status) {
			job.Status = "cancelling"
			job.CurrentPhase = "cancelling"
		}
		if integration != nil && !scanIntegrationTerminal(integration.Status) {
			integration.Phase = "cancelling"
		}
	case "scan_cancelled":
		job.Status = "cancelled"
		if job.FinishedAt == "" {
			job.FinishedAt = readString(payload["finished_at"])
		}
		if integration != nil && !scanIntegrationTerminal(integration.Status) {
			integration.Status = "cancelled"
			integration.Phase = "cancelled"
		}
	case "scan_complete":
		job.ReportID = readString(payload["report_id"])
		job.CurrentIntegrationID = ""
		job.CurrentIntegrationLabel = ""
	case "scan_error":
		if integration != nil {
			integration.Status = "failed"
			integration.Phase = "failed"
			integration.Error = readString(payload["error"])
		}
	}

	appendRecentEvent(job, eventType, payload)
}

func markIntegrationComplete(record *scanJobRecord, integrationID string) {
	if integrationID == "" || record.completedIntegrations[integrationID] {
		return
	}
	record.completedIntegrations[integrationID] = true
	record.status.IntegrationsCompleted++
}

func ensureIntegrationStatus(job *core.ScanJobStatus, integrationID string) *core.ScanJobIntegrationStatus {
	if integrationID == "" {
		return currentIntegrationStatus(job)
	}
	for i := range job.Integrations {
		if job.Integrations[i].IntegrationID == integrationID {
			return &job.Integrations[i]
		}
	}
	job.IntegrationIDs = appendIfMissing(job.IntegrationIDs, integrationID)
	job.Integrations = append(job.Integrations, core.ScanJobIntegrationStatus{
		IntegrationID: integrationID,
		Status:        "pending",
	})
	if len(job.Integrations) > job.IntegrationCount {
		job.IntegrationCount = len(job.Integrations)
	}
	return &job.Integrations[len(job.Integrations)-1]
}

func ensureMetadataProviderStatus(integration *core.ScanJobIntegrationStatus, integrationID, label, pluginID string) *core.ScanJobMetadataProviderStatus {
	if integration == nil {
		return nil
	}

	if integrationID != "" {
		for i := range integration.MetadataProviders {
			if integration.MetadataProviders[i].IntegrationID == integrationID {
				if label != "" {
					integration.MetadataProviders[i].Label = label
				}
				if pluginID != "" {
					integration.MetadataProviders[i].PluginID = pluginID
				}
				return &integration.MetadataProviders[i]
			}
		}
	}

	for i := range integration.MetadataProviders {
		provider := &integration.MetadataProviders[i]
		if provider.IntegrationID == "" && integrationID == "" && provider.PluginID == pluginID && pluginID != "" {
			if label != "" {
				provider.Label = label
			}
			return provider
		}
	}

	provider := core.ScanJobMetadataProviderStatus{
		IntegrationID: integrationID,
		Label:         label,
		PluginID:      pluginID,
		Status:        "pending",
	}
	integration.MetadataProviders = append(integration.MetadataProviders, provider)
	return &integration.MetadataProviders[len(integration.MetadataProviders)-1]
}

func currentMetadataProviderStatus(integration *core.ScanJobIntegrationStatus) *core.ScanJobMetadataProviderStatus {
	if integration == nil || integration.MetadataIntegrationID == "" {
		return nil
	}
	for i := range integration.MetadataProviders {
		if integration.MetadataProviders[i].IntegrationID == integration.MetadataIntegrationID {
			return &integration.MetadataProviders[i]
		}
	}
	return nil
}

func markUnusedMetadataProviders(integration *core.ScanJobIntegrationStatus, reason string) {
	if integration == nil {
		return
	}
	for i := range integration.MetadataProviders {
		if integration.MetadataProviders[i].Status != "pending" {
			continue
		}
		integration.MetadataProviders[i].Status = "not_used"
		integration.MetadataProviders[i].Reason = reason
	}
}

func currentIntegrationStatus(job *core.ScanJobStatus) *core.ScanJobIntegrationStatus {
	if job == nil || job.CurrentIntegrationID == "" {
		return nil
	}
	for i := range job.Integrations {
		if job.Integrations[i].IntegrationID == job.CurrentIntegrationID {
			return &job.Integrations[i]
		}
	}
	return nil
}

func appendRecentEvent(job *core.ScanJobStatus, eventType string, payload map[string]any) {
	message := scanEventMessage(eventType, payload)
	if message == "" {
		return
	}
	entry := core.ScanJobRecentEvent{
		Type:          eventType,
		TS:            readString(payload["ts"]),
		Message:       message,
		IntegrationID: readString(payload["integration_id"]),
		Label:         readString(payload["label"]),
		Data:          cloneMap(payload),
	}
	job.RecentEvents = append(job.RecentEvents, entry)
	if len(job.RecentEvents) > maxRecentScanEvents {
		job.RecentEvents = append([]core.ScanJobRecentEvent(nil), job.RecentEvents[len(job.RecentEvents)-maxRecentScanEvents:]...)
	}
}

func scanEventMessage(eventType string, payload map[string]any) string {
	switch eventType {
	case "scan_started":
		action := "Scan"
		if readBool(payload["metadata_only"]) {
			action = "Metadata refresh"
		}
		count := readInt(payload["integration_count"])
		if count > 0 {
			if count == 1 {
				return action + " started for 1 integration."
			}
			return action + " started for " + itoa(count) + " integrations."
		}
		return action + " started."
	case "scan_integration_started":
		return "Integration started: " + readLabelOrID(payload) + "."
	case "scan_source_list_started":
		return "Listing source content from " + readStringOr(payload["plugin_id"], "source") + "."
	case "scan_source_list_complete":
		if fileCount := readInt(payload["file_count"]); fileCount > 0 {
			return "Source listing complete: " + itoa(fileCount) + " files found."
		}
		return "Source listing complete: " + itoa(readInt(payload["game_count"])) + " games found."
	case "scan_scanner_started":
		return "Scanner started for " + itoa(readInt(payload["file_count"])) + " files."
	case "scan_scanner_progress":
		current := readInt(payload["processed_count"])
		total := readInt(payload["file_count"])
		if !shouldLogProgress(current, total, 25) {
			return ""
		}
		return "Scanner progress: " + itoa(current) + " / " + itoa(total) + " files."
	case "scan_scanner_complete":
		return "Scanner grouped " + itoa(readInt(payload["group_count"])) + " games."
	case "scan_metadata_started":
		return "Metadata started for " + itoa(readInt(payload["game_count"])) + " games across " + itoa(readInt(payload["resolver_count"])) + " providers."
	case "scan_metadata_phase":
		return "Metadata phase: " + metadataPhaseLabel(readString(payload["phase"])) + "."
	case "scan_metadata_plugin_started":
		return readMetadataProviderLabel(payload) + " started " + readStringOr(payload["phase"], "metadata") + " for " + itoa(readInt(payload["batch_size"])) + " games."
	case "scan_metadata_game_progress":
		current := readInt(payload["game_index"])
		total := readInt(payload["game_count"])
		if !shouldLogProgress(current, total, 25) {
			return ""
		}
		title := readString(payload["game_title"])
		if title != "" {
			return readMetadataProviderLabel(payload) + " " + itoa(current) + " / " + itoa(total) + " - " + title
		}
		return readMetadataProviderLabel(payload) + " " + itoa(current) + " / " + itoa(total)
	case "scan_metadata_plugin_complete":
		if matched := readInt(payload["matched"]); matched > 0 || payload["matched"] != nil {
			return readMetadataProviderLabel(payload) + " matched " + itoa(matched) + "/" + itoa(readInt(payload["total"])) + "."
		}
		return readMetadataProviderLabel(payload) + " filled " + itoa(readInt(payload["filled"])) + " metadata gaps."
	case "scan_metadata_plugin_error":
		return readMetadataProviderLabel(payload) + " error: " + readStringOr(payload["error"], "unknown error") + "."
	case "scan_metadata_consensus_complete":
		return "Consensus complete: " + itoa(readInt(payload["identified"])) + " identified, " + itoa(readInt(payload["unidentified"])) + " unidentified."
	case "scan_metadata_finished":
		if readString(payload["status"]) == "degraded" {
			return "Metadata complete with " + itoa(readInt(payload["error_count"])) + " provider failures: " + itoa(readInt(payload["identified"])) + " identified, " + itoa(readInt(payload["unidentified"])) + " unidentified."
		}
		return "Metadata complete: " + itoa(readInt(payload["identified"])) + " identified, " + itoa(readInt(payload["unidentified"])) + " unidentified."
	case "scan_persist_started":
		return "Persisting results."
	case "scan_integration_complete":
		return "Integration complete: " + readLabelOrID(payload) + " (" + itoa(readInt(payload["games_found"])) + " games)."
	case "scan_integration_skipped":
		reason := readString(payload["reason"])
		if reason != "" {
			return "Integration skipped: " + readLabelOrID(payload) + " (" + reason + ")."
		}
		return "Integration skipped: " + readLabelOrID(payload) + "."
	case "scan_cancel_requested":
		return "Scan cancellation requested."
	case "scan_cancelled":
		return "Scan cancelled."
	case "scan_complete":
		if readBool(payload["metadata_only"]) {
			return "Metadata refresh complete."
		}
		return "Scan complete."
	case "scan_error":
		if readBool(payload["metadata_only"]) {
			return "Metadata refresh failed: " + readStringOr(payload["error"], "unknown error") + "."
		}
		return "Scan failed: " + readStringOr(payload["error"], "unknown error") + "."
	default:
		return ""
	}
}

func readIntegrationSnapshots(value any) []core.ScanJobIntegrationStatus {
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]core.ScanJobIntegrationStatus, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := readString(m["integration_id"])
		if id == "" {
			continue
		}
		out = append(out, core.ScanJobIntegrationStatus{
			IntegrationID: id,
			Label:         readString(m["label"]),
			PluginID:      readString(m["plugin_id"]),
			Status:        "pending",
			Phase:         "queued",
		})
	}
	return out
}

func readMetadataProviderSnapshots(value any) []core.ScanJobMetadataProviderStatus {
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]core.ScanJobMetadataProviderStatus, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		provider := core.ScanJobMetadataProviderStatus{
			IntegrationID: readString(m["integration_id"]),
			Label:         readString(m["label"]),
			PluginID:      readString(m["plugin_id"]),
			Status:        readStringOr(m["status"], "pending"),
			Phase:         readString(m["phase"]),
			Reason:        readString(m["reason"]),
			Error:         readString(m["error"]),
		}
		if progress, ok := m["progress"].(map[string]any); ok {
			provider.Progress = &core.ScanJobProgress{
				Current:       readInt(progress["current"]),
				Total:         readInt(progress["total"]),
				Unit:          readString(progress["unit"]),
				Indeterminate: readBool(progress["indeterminate"]),
			}
		}
		out = append(out, provider)
	}
	return out
}

func cloneScanJobStatus(in *core.ScanJobStatus) *core.ScanJobStatus {
	if in == nil {
		return nil
	}
	out := *in
	out.IntegrationIDs = append([]string(nil), in.IntegrationIDs...)
	out.Integrations = make([]core.ScanJobIntegrationStatus, len(in.Integrations))
	for i, integration := range in.Integrations {
		out.Integrations[i] = integration
		out.Integrations[i].SourceProgress = cloneScanJobProgress(integration.SourceProgress)
		out.Integrations[i].MetadataProgress = cloneScanJobProgress(integration.MetadataProgress)
		out.Integrations[i].MetadataProviders = cloneMetadataProviderStatuses(integration.MetadataProviders)
	}
	out.RecentEvents = make([]core.ScanJobRecentEvent, len(in.RecentEvents))
	for i, event := range in.RecentEvents {
		out.RecentEvents[i] = event
		out.RecentEvents[i].Data = cloneMap(event.Data)
	}
	return &out
}

func cloneScanJobProgress(in *core.ScanJobProgress) *core.ScanJobProgress {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneMetadataProviderStatuses(in []core.ScanJobMetadataProviderStatus) []core.ScanJobMetadataProviderStatus {
	if len(in) == 0 {
		return nil
	}
	out := make([]core.ScanJobMetadataProviderStatus, len(in))
	for i, provider := range in {
		out[i] = provider
		out[i].Progress = cloneScanJobProgress(provider.Progress)
	}
	return out
}

func scanPhaseForEvent(eventType string, payload map[string]any) string {
	switch eventType {
	case "scan_started":
		if readBool(payload["metadata_only"]) {
			return "metadata refresh"
		}
		return "scan started"
	case "scan_integration_started":
		return "integration started"
	case "scan_source_list_started":
		return "listing source content"
	case "scan_source_list_complete":
		return "source listing complete"
	case "scan_scanner_started", "scan_scanner_progress":
		return "scanning files"
	case "scan_scanner_complete":
		return "game detection complete"
	case "scan_metadata_started":
		return "metadata enrichment"
	case "scan_metadata_phase":
		if phase := readString(payload["phase"]); phase != "" {
			return "metadata " + phase
		}
		return "metadata"
	case "scan_metadata_plugin_started", "scan_metadata_game_progress", "scan_metadata_plugin_complete", "scan_metadata_plugin_error":
		phase := readString(payload["phase"])
		providerLabel := readMetadataProviderLabel(payload)
		if phase != "" && providerLabel != "" {
			return "metadata via " + providerLabel + ": " + phase
		}
		if phase != "" {
			return "metadata: " + phase
		}
		return "metadata plugin"
	case "scan_metadata_consensus_complete":
		return "metadata consensus complete"
	case "scan_metadata_finished":
		return "metadata complete"
	case "scan_persist_started":
		return "persisting results"
	case "scan_integration_complete":
		return "integration complete"
	case "scan_integration_skipped":
		return "integration skipped"
	case "scan_cancel_requested":
		return "cancelling"
	case "scan_cancelled":
		return "cancelled"
	case "scan_complete":
		if readBool(payload["metadata_only"]) {
			return "metadata refresh complete"
		}
		return "scan complete"
	case "scan_error":
		if readBool(payload["metadata_only"]) {
			return "metadata refresh error"
		}
		return "scan error"
	default:
		return eventType
	}
}

func scanJobTerminal(status string) bool {
	return status == "completed" || status == "failed" || status == "cancelled"
}

func scanIntegrationTerminal(status string) bool {
	return status == "completed" || status == "skipped" || status == "failed" || status == "cancelled"
}

func shouldLogProgress(current, total, step int) bool {
	if current <= 0 {
		return false
	}
	if total <= 0 {
		return true
	}
	if current == 1 || current == total {
		return true
	}
	return step > 0 && current%step == 0
}

func metadataPhaseLabel(phase string) string {
	switch phase {
	case "identify":
		return "Identifying"
	case "consensus":
		return "Building consensus"
	case "fill":
		return "Filling gaps"
	default:
		if phase == "" {
			return "working"
		}
		return phase
	}
}

func readMetadataProviderLabel(payload map[string]any) string {
	if label := readString(payload["metadata_label"]); label != "" {
		return label
	}
	if pluginID := readString(payload["plugin_id"]); pluginID != "" {
		return pluginID
	}
	return "Provider"
}

func readLabelOrID(payload map[string]any) string {
	if label := readString(payload["label"]); label != "" {
		return label
	}
	if current := readString(payload["current_integration"]); current != "" {
		return current
	}
	return readStringOr(payload["integration_id"], "unknown")
}

func readStringOr(value any, fallback string) string {
	if s := readString(value); s != "" {
		return s
	}
	return fallback
}

func appendIfMissing(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func eventUsesSourcePluginID(eventType string) bool {
	switch eventType {
	case "scan_integration_started", "scan_source_list_started", "scan_integration_complete", "scan_integration_skipped":
		return true
	default:
		return false
	}
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneValue(v)
	}
	return out
}

func cloneSlice(in []any) []any {
	out := make([]any, len(in))
	for i, value := range in {
		out[i] = cloneValue(value)
	}
	return out
}

func cloneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneMap(v)
	case []any:
		return cloneSlice(v)
	default:
		return v
	}
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func readString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func readInt(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

func readBool(value any) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return false
}
