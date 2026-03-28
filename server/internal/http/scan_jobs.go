package http

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
	"github.com/google/uuid"
)

type scanRunner interface {
	RunScan(ctx context.Context, integrationIDs []string) ([]*core.CanonicalGame, error)
	RunMetadataRefresh(ctx context.Context, integrationIDs []string) ([]*core.CanonicalGame, error)
}

type scanJobManager struct {
	runner scanRunner
	bus    *events.EventBus
	logger core.Logger

	mu          sync.RWMutex
	activeJobID string
	jobs        map[string]*core.ScanJobStatus
}

func newScanJobManager(runner scanRunner, bus *events.EventBus, logger core.Logger) *scanJobManager {
	return &scanJobManager{
		runner: runner,
		bus:    bus,
		logger: logger,
		jobs:   make(map[string]*core.ScanJobStatus),
	}
}

func (m *scanJobManager) Start(req ScanRequest) (*core.ScanJobStatus, bool, error) {
	m.mu.Lock()
	if current := m.activeRunningJobLocked(); current != nil {
		out := cloneScanJobStatus(current)
		m.mu.Unlock()
		return out, true, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	job := &core.ScanJobStatus{
		JobID:          uuid.NewString(),
		Status:         "queued",
		MetadataOnly:   req.MetadataOnly,
		IntegrationIDs: append([]string(nil), req.GameSources...),
		StartedAt:      now,
		CurrentPhase:   "queued",
	}
	m.jobs[job.JobID] = job
	m.activeJobID = job.JobID
	out := cloneScanJobStatus(job)
	m.mu.Unlock()

	go m.run(job.JobID, req)
	return out, false, nil
}

func (m *scanJobManager) Get(jobID string) *core.ScanJobStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneScanJobStatus(m.jobs[jobID])
}

func (m *scanJobManager) run(jobID string, req ScanRequest) {
	ctx, cancel := context.WithTimeout(scan.WithScanJobID(context.Background(), jobID), 30*time.Minute)
	defer cancel()

	done := make(chan struct{})
	var sub <-chan events.Event
	if m.bus != nil {
		sub = m.bus.Subscribe()
		go m.watch(jobID, sub, done)
	}

	m.update(jobID, func(job *core.ScanJobStatus) {
		job.Status = "running"
		job.CurrentPhase = "starting"
	})

	var err error
	if req.MetadataOnly {
		_, err = m.runner.RunMetadataRefresh(ctx, req.GameSources)
	} else {
		_, err = m.runner.RunScan(ctx, req.GameSources)
	}

	close(done)
	if m.bus != nil && sub != nil {
		m.bus.Unsubscribe(sub)
	}

	if err != nil {
		m.update(jobID, func(job *core.ScanJobStatus) {
			job.Status = "failed"
			job.Error = err.Error()
			job.CurrentPhase = "failed"
			job.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		})
	} else {
		m.update(jobID, func(job *core.ScanJobStatus) {
			job.Status = "completed"
			job.CurrentPhase = ""
			job.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		})
	}

	m.mu.Lock()
	if m.activeJobID == jobID {
		m.activeJobID = ""
	}
	m.mu.Unlock()
}

func (m *scanJobManager) watch(jobID string, sub <-chan events.Event, done <-chan struct{}) {
	completed := make(map[string]bool)

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
			m.update(jobID, func(job *core.ScanJobStatus) {
				job.CurrentPhase = scanPhaseForEvent(ev.Type, payload)
				if id := readString(payload["integration_id"]); id != "" {
					job.CurrentIntegrationID = id
				}
				if label := readString(payload["label"]); label != "" {
					job.CurrentIntegrationLabel = label
				}
				switch ev.Type {
				case "scan_started":
					job.IntegrationCount = readInt(payload["integration_count"])
				case "scan_integration_complete", "scan_integration_skipped":
					id := readString(payload["integration_id"])
					if id != "" && !completed[id] {
						completed[id] = true
						job.IntegrationsCompleted++
					}
				case "scan_complete":
					job.ReportID = readString(payload["report_id"])
				}
			})
		}
	}
}

func (m *scanJobManager) update(jobID string, mutate func(*core.ScanJobStatus)) {
	m.mu.Lock()
	job := m.jobs[jobID]
	if job != nil {
		mutate(job)
	}
	m.mu.Unlock()
}

func (m *scanJobManager) activeRunningJobLocked() *core.ScanJobStatus {
	if m.activeJobID == "" {
		return nil
	}
	job := m.jobs[m.activeJobID]
	if job == nil {
		m.activeJobID = ""
		return nil
	}
	if job.Status == "completed" || job.Status == "failed" {
		m.activeJobID = ""
		return nil
	}
	return job
}

func cloneScanJobStatus(in *core.ScanJobStatus) *core.ScanJobStatus {
	if in == nil {
		return nil
	}
	out := *in
	out.IntegrationIDs = append([]string(nil), in.IntegrationIDs...)
	return &out
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
	case "scan_scanner_started":
		return "detecting games"
	case "scan_scanner_complete":
		return "game detection complete"
	case "scan_metadata_started":
		return "metadata enrichment"
	case "scan_metadata_phase":
		if phase := readString(payload["phase"]); phase != "" {
			return "metadata " + phase
		}
		return "metadata"
	case "scan_metadata_plugin_started":
		phase := readString(payload["phase"])
		pluginID := readString(payload["plugin_id"])
		if phase != "" && pluginID != "" {
			return "metadata " + phase + ": " + pluginID
		}
		if phase != "" {
			return "metadata " + phase
		}
		return "metadata plugin"
	case "scan_metadata_plugin_complete":
		return "metadata plugin complete"
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
	case "scan_complete":
		return "scan complete"
	case "scan_error":
		return "scan error"
	default:
		return eventType
	}
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
