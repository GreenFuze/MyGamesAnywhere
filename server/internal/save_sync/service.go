package save_sync

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/google/uuid"
)

var slotIDs = []string{
	"autosave",
	"slot-1",
	"slot-2",
	"slot-3",
	"slot-4",
	"slot-5",
	"state-1",
	"state-2",
	"state-3",
	"state-4",
	"state-5",
	"state-6",
	"state-7",
	"state-8",
	"state-9",
	"save-ram",
}

type PluginHost = plugins.PluginHost

type service struct {
	integrationRepo core.IntegrationRepository
	gameStore       core.GameStore
	pluginHost      PluginHost
	logger          core.Logger
	eventBus        *events.EventBus

	cacheRoot string

	mu                sync.RWMutex
	jobs              map[string]*core.SaveSyncMigrationStatus
	prefetchJobs      map[string]*core.SaveSyncPrefetchStatus
	uploadWorkers     map[string]*slotUploadWorker
	cachePathReplacer *regexp.Regexp
}

type slotUploadWorker struct {
	running bool
	pending bool
}

type saveSyncStoredManifest struct {
	Version         int                         `json:"version"`
	CanonicalGameID string                      `json:"canonical_game_id"`
	SourceGameID    string                      `json:"source_game_id"`
	Runtime         string                      `json:"runtime"`
	SlotID          string                      `json:"slot_id"`
	UpdatedAt       time.Time                   `json:"updated_at"`
	FileCount       int                         `json:"file_count"`
	TotalSize       int64                       `json:"total_size"`
	Files           []core.SaveSyncSnapshotFile `json:"files"`
}

type saveSyncPluginFile struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
}

type saveSyncCacheStatus struct {
	SyncState          string    `json:"sync_state"`
	LastSyncError      string    `json:"last_sync_error,omitempty"`
	RemoteManifestHash string    `json:"remote_manifest_hash,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func NewService(
	integrationRepo core.IntegrationRepository,
	gameStore core.GameStore,
	pluginHost PluginHost,
	logger core.Logger,
	eventBus *events.EventBus,
) core.SaveSyncService {
	return &service{
		integrationRepo:   integrationRepo,
		gameStore:         gameStore,
		pluginHost:        pluginHost,
		logger:            logger,
		eventBus:          eventBus,
		cacheRoot:         defaultSaveSyncCacheRoot(),
		jobs:              make(map[string]*core.SaveSyncMigrationStatus),
		prefetchJobs:      make(map[string]*core.SaveSyncPrefetchStatus),
		uploadWorkers:     make(map[string]*slotUploadWorker),
		cachePathReplacer: regexp.MustCompile(`[^A-Za-z0-9._-]+`),
	}
}

func (s *service) ListSlots(ctx context.Context, req core.SaveSyncListRequest) ([]core.SaveSyncSlotSummary, error) {
	if err := s.validateSlotListRequest(ctx, req); err != nil {
		return nil, err
	}
	if _, _, err := s.resolveIntegrationForSaveSync(ctx, req.IntegrationID); err != nil {
		return nil, err
	}

	summaries := make([]core.SaveSyncSlotSummary, 0, len(slotIDs))
	for _, slotID := range slotIDs {
		ref := core.SaveSyncSlotRef{
			CanonicalGameID: req.CanonicalGameID,
			SourceGameID:    req.SourceGameID,
			Runtime:         req.Runtime,
			SlotID:          slotID,
			IntegrationID:   req.IntegrationID,
		}
		summary, err := s.readSlotSummaryFromCache(ref)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func (s *service) GetSlot(ctx context.Context, req core.SaveSyncSlotRef) (*core.SaveSyncSnapshot, error) {
	if err := s.validateSlotRef(ctx, req); err != nil {
		return nil, err
	}
	if _, _, err := s.resolveIntegrationForSaveSync(ctx, req.IntegrationID); err != nil {
		return nil, err
	}
	return s.readSlotSnapshotFromCache(req)
}

func (s *service) PutSlot(ctx context.Context, req core.SaveSyncPutRequest) (*core.SaveSyncPutResult, error) {
	if err := s.validatePutRequest(ctx, req); err != nil {
		return nil, err
	}
	if _, _, err := s.resolveIntegrationForSaveSync(ctx, req.IntegrationID); err != nil {
		return nil, err
	}

	currentSummary, err := s.readSlotSummaryFromCache(req.SaveSyncSlotRef)
	if err != nil {
		return nil, err
	}
	if currentSummary.Exists && !req.Force && req.BaseManifestHash != currentSummary.ManifestHash {
		return &core.SaveSyncPutResult{
			OK: false,
			Summary: core.SaveSyncSlotSummary{
				SlotID:       req.SlotID,
				Exists:       true,
				ManifestHash: currentSummary.ManifestHash,
				UpdatedAt:    currentSummary.UpdatedAt,
				FileCount:    currentSummary.FileCount,
				TotalSize:    currentSummary.TotalSize,
				Cached:       currentSummary.Cached,
				SyncState:    currentSummary.SyncState,
			},
			Conflict: &core.SaveSyncConflict{
				SlotID:             req.SlotID,
				Message:            "cached slot changed since the last known manifest",
				RemoteManifestHash: currentSummary.ManifestHash,
				RemoteUpdatedAt:    currentSummary.UpdatedAt,
				RemoteFileCount:    currentSummary.FileCount,
				RemoteTotalSize:    currentSummary.TotalSize,
			},
		}, nil
	}

	archiveBytes, err := base64.StdEncoding.DecodeString(req.Snapshot.ArchiveBase64)
	if err != nil {
		return nil, fmt.Errorf("decode snapshot archive: %w", err)
	}
	if err := validateArchive(archiveBytes, req.Snapshot.Files); err != nil {
		return nil, err
	}

	files := append([]core.SaveSyncSnapshotFile(nil), req.Snapshot.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	updatedAt := time.Now().UTC()
	manifest := saveSyncStoredManifest{
		Version:         1,
		CanonicalGameID: req.CanonicalGameID,
		SourceGameID:    req.SourceGameID,
		Runtime:         req.Runtime,
		SlotID:          req.SlotID,
		UpdatedAt:       updatedAt,
		FileCount:       len(files),
		TotalSize:       sumSnapshotSize(files),
		Files:           files,
	}
	manifestBytes, manifestHash, err := marshalManifest(manifest)
	if err != nil {
		return nil, err
	}

	if err := s.writeSlotCache(req.SaveSyncSlotRef, manifestBytes, archiveBytes, saveSyncCacheStatus{
		SyncState: "uploading",
		UpdatedAt: updatedAt,
	}); err != nil {
		return nil, err
	}
	s.enqueueUpload(req.SaveSyncSlotRef)

	return &core.SaveSyncPutResult{
		OK: true,
		Summary: core.SaveSyncSlotSummary{
			SlotID:        req.SlotID,
			Exists:        true,
			ManifestHash:  manifestHash,
			UpdatedAt:     updatedAt.Format(time.RFC3339),
			FileCount:     manifest.FileCount,
			TotalSize:     manifest.TotalSize,
			Cached:        true,
			SyncState:     "uploading",
			UploadPending: true,
		},
	}, nil
}

func (s *service) StartMigration(ctx context.Context, req core.SaveSyncMigrationRequest) (*core.SaveSyncMigrationStatus, error) {
	if req.SourceIntegrationID == "" || req.TargetIntegrationID == "" {
		return nil, fmt.Errorf("source_integration_id and target_integration_id are required")
	}
	if req.SourceIntegrationID == req.TargetIntegrationID {
		return nil, fmt.Errorf("source and target integrations must differ")
	}
	if req.Scope != core.SaveSyncMigrationScopeAll && req.Scope != core.SaveSyncMigrationScopeGame {
		return nil, fmt.Errorf("unsupported migration scope")
	}
	if req.Scope == core.SaveSyncMigrationScopeGame && strings.TrimSpace(req.CanonicalGameID) == "" {
		return nil, fmt.Errorf("canonical_game_id is required for game migrations")
	}
	if _, _, err := s.resolveIntegrationForSaveSync(ctx, req.SourceIntegrationID); err != nil {
		return nil, err
	}
	if _, _, err := s.resolveIntegrationForSaveSync(ctx, req.TargetIntegrationID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	status := &core.SaveSyncMigrationStatus{
		JobID:               uuid.New().String(),
		Status:              "started",
		Scope:               req.Scope,
		SourceIntegrationID: req.SourceIntegrationID,
		TargetIntegrationID: req.TargetIntegrationID,
		CanonicalGameID:     req.CanonicalGameID,
		StartedAt:           now.Format(time.RFC3339),
	}

	s.mu.Lock()
	s.jobs[status.JobID] = cloneMigrationStatus(status)
	s.mu.Unlock()
	s.publishMigrationEvent("save_sync_migration_started", status)

	go s.runMigration(req, status.JobID)
	return cloneMigrationStatus(status), nil
}

func (s *service) GetMigrationStatus(_ context.Context, jobID string) (*core.SaveSyncMigrationStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status, ok := s.jobs[jobID]
	if !ok {
		return nil, nil
	}
	return cloneMigrationStatus(status), nil
}

func (s *service) StartPrefetch(ctx context.Context, req core.SaveSyncPrefetchRequest) (*core.SaveSyncPrefetchStatus, error) {
	if err := s.validateSlotListRequest(ctx, core.SaveSyncListRequest{
		CanonicalGameID: req.CanonicalGameID,
		SourceGameID:    req.SourceGameID,
		Runtime:         req.Runtime,
		IntegrationID:   req.IntegrationID,
	}); err != nil {
		return nil, err
	}
	if _, _, err := s.resolveIntegrationForSaveSync(ctx, req.IntegrationID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	status := &core.SaveSyncPrefetchStatus{
		JobID:           uuid.New().String(),
		Status:          "started",
		Message:         "prefetch queued",
		CanonicalGameID: req.CanonicalGameID,
		SourceGameID:    req.SourceGameID,
		Runtime:         req.Runtime,
		IntegrationID:   req.IntegrationID,
		ProgressTotal:   len(emulatorJSSlotIDs()),
		StartedAt:       now.Format(time.RFC3339),
	}

	s.mu.Lock()
	s.prefetchJobs[status.JobID] = clonePrefetchStatus(status)
	s.mu.Unlock()
	s.publishPrefetchEvent("save_sync_prefetch_started", status)

	go s.runPrefetch(req, status.JobID)
	return clonePrefetchStatus(status), nil
}

func (s *service) GetPrefetchStatus(_ context.Context, jobID string) (*core.SaveSyncPrefetchStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status, ok := s.prefetchJobs[jobID]
	if !ok {
		return nil, nil
	}
	return clonePrefetchStatus(status), nil
}

func (s *service) runPrefetch(req core.SaveSyncPrefetchRequest, jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, integration, err := s.resolveIntegrationForSaveSync(ctx, req.IntegrationID)
	if err != nil {
		s.finishPrefetch(jobID, err)
		return
	}
	listReq := core.SaveSyncListRequest{
		CanonicalGameID: req.CanonicalGameID,
		SourceGameID:    req.SourceGameID,
		Runtime:         req.Runtime,
		IntegrationID:   req.IntegrationID,
	}
	existingManifests, err := s.listExistingManifestPaths(ctx, integration, listReq)
	if err != nil {
		s.finishPrefetch(jobID, err)
		return
	}

	s.updatePrefetch(jobID, func(status *core.SaveSyncPrefetchStatus) {
		status.Status = "running"
		status.Message = "prefetching save slots"
	})

	for _, slotID := range emulatorJSSlotIDs() {
		ref := core.SaveSyncSlotRef{
			CanonicalGameID: req.CanonicalGameID,
			SourceGameID:    req.SourceGameID,
			Runtime:         req.Runtime,
			SlotID:          slotID,
			IntegrationID:   req.IntegrationID,
		}
		if !existingManifests[slotManifestPath(ref)] {
			_ = s.removeSlotCachePayload(ref)
			_ = s.writeSlotCacheStatus(ref, saveSyncCacheStatus{SyncState: "missing", UpdatedAt: time.Now().UTC()})
			s.updatePrefetch(jobID, func(status *core.SaveSyncPrefetchStatus) {
				status.ProgressCurrent++
				status.SlotsMissing++
				status.Message = "save slot " + slotID + " is empty"
			})
			continue
		}

		manifest, manifestHash, err := s.fetchStoredManifest(ctx, integration, ref)
		if err != nil {
			s.updatePrefetch(jobID, func(status *core.SaveSyncPrefetchStatus) {
				status.SlotsFailed++
				status.Message = "failed to prefetch " + slotID
			})
			s.finishPrefetch(jobID, err)
			return
		}
		if manifest == nil {
			_ = s.removeSlotCachePayload(ref)
			_ = s.writeSlotCacheStatus(ref, saveSyncCacheStatus{SyncState: "missing", UpdatedAt: time.Now().UTC()})
			s.updatePrefetch(jobID, func(status *core.SaveSyncPrefetchStatus) {
				status.ProgressCurrent++
				status.SlotsMissing++
				status.Message = "save slot " + slotID + " is empty"
			})
			continue
		}

		archiveBytes, err := s.fetchArchiveBytes(ctx, integration, ref)
		if err != nil {
			s.updatePrefetch(jobID, func(status *core.SaveSyncPrefetchStatus) {
				status.SlotsFailed++
				status.Message = "failed to prefetch " + slotID
			})
			s.finishPrefetch(jobID, err)
			return
		}
		if archiveBytes == nil {
			_ = s.removeSlotCachePayload(ref)
			_ = s.writeSlotCacheStatus(ref, saveSyncCacheStatus{SyncState: "missing", UpdatedAt: time.Now().UTC()})
			s.updatePrefetch(jobID, func(status *core.SaveSyncPrefetchStatus) {
				status.ProgressCurrent++
				status.SlotsMissing++
				status.Message = "save slot " + slotID + " is empty"
			})
			continue
		}
		manifestBytes, _, err := marshalManifest(*manifest)
		if err != nil {
			s.finishPrefetch(jobID, err)
			return
		}
		if err := s.writeSlotCache(ref, manifestBytes, archiveBytes, saveSyncCacheStatus{
			SyncState:          "synced",
			RemoteManifestHash: manifestHash,
			UpdatedAt:          manifest.UpdatedAt,
		}); err != nil {
			s.finishPrefetch(jobID, err)
			return
		}
		s.updatePrefetch(jobID, func(status *core.SaveSyncPrefetchStatus) {
			status.ProgressCurrent++
			status.SlotsCached++
			status.Message = "prefetched " + slotID
		})
	}

	s.updatePrefetch(jobID, func(status *core.SaveSyncPrefetchStatus) {
		status.Status = "completed"
		status.Message = "save slots prefetched"
		status.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	})
}

func (s *service) runMigration(req core.SaveSyncMigrationRequest, jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	targets, err := s.enumerateMigrationTargets(ctx, req)
	if err != nil {
		s.finishMigration(jobID, err)
		return
	}
	s.updateMigration(jobID, func(status *core.SaveSyncMigrationStatus) {
		status.ItemsTotal = len(targets)
	})

	for _, target := range targets {
		snapshot, err := s.GetSlot(ctx, target.Source)
		if err != nil {
			s.finishMigration(jobID, err)
			return
		}
		if snapshot == nil {
			s.updateMigration(jobID, func(status *core.SaveSyncMigrationStatus) {
				status.ItemsCompleted++
				status.SlotsSkipped++
			})
			continue
		}

		_, err = s.PutSlot(ctx, core.SaveSyncPutRequest{
			SaveSyncSlotRef: core.SaveSyncSlotRef{
				CanonicalGameID: target.Target.CanonicalGameID,
				SourceGameID:    target.Target.SourceGameID,
				Runtime:         target.Target.Runtime,
				SlotID:          target.Target.SlotID,
				IntegrationID:   target.Target.IntegrationID,
			},
			Force:    true,
			Snapshot: *snapshot,
		})
		if err != nil {
			s.finishMigration(jobID, err)
			return
		}

		if req.DeleteSourceAfterSuccess {
			if err := s.deleteSlot(ctx, target.Source); err != nil {
				s.finishMigration(jobID, err)
				return
			}
		}

		s.updateMigration(jobID, func(status *core.SaveSyncMigrationStatus) {
			status.ItemsCompleted++
			status.SlotsMigrated++
		})
	}

	s.updateMigration(jobID, func(status *core.SaveSyncMigrationStatus) {
		status.Status = "completed"
		status.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	})
}

type migrationTarget struct {
	Source core.SaveSyncSlotRef
	Target core.SaveSyncSlotRef
}

func (s *service) enumerateMigrationTargets(ctx context.Context, req core.SaveSyncMigrationRequest) ([]migrationTarget, error) {
	var games []*core.CanonicalGame
	if req.Scope == core.SaveSyncMigrationScopeGame {
		game, err := s.gameStore.GetCanonicalGameByID(ctx, req.CanonicalGameID)
		if err != nil {
			return nil, fmt.Errorf("load canonical game: %w", err)
		}
		if game == nil {
			return nil, fmt.Errorf("canonical game not found")
		}
		games = []*core.CanonicalGame{game}
	} else {
		var err error
		games, err = s.gameStore.GetCanonicalGames(ctx)
		if err != nil {
			return nil, fmt.Errorf("list canonical games: %w", err)
		}
	}

	targets := make([]migrationTarget, 0)
	for _, game := range games {
		if game == nil {
			continue
		}
		for _, sourceGame := range game.SourceGames {
			if sourceGame == nil || sourceGame.Status != "found" {
				continue
			}
			runtime, ok := core.BrowserPlayRuntimeForSourceGame(sourceGame.Platform, game.Platform)
			if !ok {
				continue
			}
			for _, slotID := range slotIDs {
				targets = append(targets, migrationTarget{
					Source: core.SaveSyncSlotRef{
						CanonicalGameID: game.ID,
						SourceGameID:    sourceGame.ID,
						Runtime:         runtime,
						SlotID:          slotID,
						IntegrationID:   req.SourceIntegrationID,
					},
					Target: core.SaveSyncSlotRef{
						CanonicalGameID: game.ID,
						SourceGameID:    sourceGame.ID,
						Runtime:         runtime,
						SlotID:          slotID,
						IntegrationID:   req.TargetIntegrationID,
					},
				})
			}
		}
	}
	return targets, nil
}

func (s *service) finishMigration(jobID string, err error) {
	s.updateMigration(jobID, func(status *core.SaveSyncMigrationStatus) {
		status.Status = "failed"
		status.Error = err.Error()
		status.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	})
}

func (s *service) updateMigration(jobID string, mutate func(*core.SaveSyncMigrationStatus)) {
	s.mu.Lock()
	status, ok := s.jobs[jobID]
	if ok {
		mutate(status)
	}
	updated := cloneMigrationStatus(status)
	s.mu.Unlock()
	if !ok || updated == nil {
		return
	}

	switch updated.Status {
	case "completed":
		s.publishMigrationEvent("save_sync_migration_completed", updated)
	case "failed":
		s.publishMigrationEvent("save_sync_migration_failed", updated)
	default:
		s.publishMigrationEvent("save_sync_migration_progress", updated)
	}
}

func (s *service) publishMigrationEvent(eventType string, status *core.SaveSyncMigrationStatus) {
	if s.eventBus == nil || status == nil {
		return
	}
	events.PublishJSON(s.eventBus, eventType, map[string]any{
		"job_id":                status.JobID,
		"status":                status.Status,
		"scope":                 status.Scope,
		"source_integration_id": status.SourceIntegrationID,
		"target_integration_id": status.TargetIntegrationID,
		"canonical_game_id":     status.CanonicalGameID,
		"started_at":            status.StartedAt,
		"finished_at":           status.FinishedAt,
		"items_total":           status.ItemsTotal,
		"items_completed":       status.ItemsCompleted,
		"slots_migrated":        status.SlotsMigrated,
		"slots_skipped":         status.SlotsSkipped,
		"error":                 status.Error,
	})
}

func cloneMigrationStatus(in *core.SaveSyncMigrationStatus) *core.SaveSyncMigrationStatus {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (s *service) finishPrefetch(jobID string, err error) {
	s.updatePrefetch(jobID, func(status *core.SaveSyncPrefetchStatus) {
		status.Status = "failed"
		status.Error = err.Error()
		status.Message = err.Error()
		status.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	})
}

func (s *service) updatePrefetch(jobID string, mutate func(*core.SaveSyncPrefetchStatus)) {
	s.mu.Lock()
	status, ok := s.prefetchJobs[jobID]
	if ok {
		mutate(status)
	}
	updated := clonePrefetchStatus(status)
	s.mu.Unlock()
	if !ok || updated == nil {
		return
	}

	switch updated.Status {
	case "completed":
		s.publishPrefetchEvent("save_sync_prefetch_completed", updated)
	case "failed":
		s.publishPrefetchEvent("save_sync_prefetch_failed", updated)
	default:
		s.publishPrefetchEvent("save_sync_prefetch_progress", updated)
	}
}

func (s *service) publishPrefetchEvent(eventType string, status *core.SaveSyncPrefetchStatus) {
	if s.eventBus == nil || status == nil {
		return
	}
	events.PublishJSON(s.eventBus, eventType, map[string]any{
		"job_id":            status.JobID,
		"status":            status.Status,
		"message":           status.Message,
		"error":             status.Error,
		"canonical_game_id": status.CanonicalGameID,
		"source_game_id":    status.SourceGameID,
		"runtime":           status.Runtime,
		"integration_id":    status.IntegrationID,
		"progress_current":  status.ProgressCurrent,
		"progress_total":    status.ProgressTotal,
		"slots_cached":      status.SlotsCached,
		"slots_missing":     status.SlotsMissing,
		"slots_failed":      status.SlotsFailed,
		"started_at":        status.StartedAt,
		"finished_at":       status.FinishedAt,
	})
}

func clonePrefetchStatus(in *core.SaveSyncPrefetchStatus) *core.SaveSyncPrefetchStatus {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (s *service) validatePutRequest(ctx context.Context, req core.SaveSyncPutRequest) error {
	if err := s.validateSlotRef(ctx, req.SaveSyncSlotRef); err != nil {
		return err
	}
	if req.Snapshot.ArchiveBase64 == "" {
		return fmt.Errorf("snapshot archive_base64 is required")
	}
	if req.Snapshot.CanonicalGameID != "" && req.Snapshot.CanonicalGameID != req.CanonicalGameID {
		return fmt.Errorf("snapshot canonical_game_id mismatch")
	}
	if req.Snapshot.SourceGameID != "" && req.Snapshot.SourceGameID != req.SourceGameID {
		return fmt.Errorf("snapshot source_game_id mismatch")
	}
	if req.Snapshot.Runtime != "" && req.Snapshot.Runtime != req.Runtime {
		return fmt.Errorf("snapshot runtime mismatch")
	}
	if req.Snapshot.SlotID != "" && req.Snapshot.SlotID != req.SlotID {
		return fmt.Errorf("snapshot slot_id mismatch")
	}
	return nil
}

func (s *service) readSlotSummaryFromCache(ref core.SaveSyncSlotRef) (core.SaveSyncSlotSummary, error) {
	status, _ := s.readSlotCacheStatus(ref)
	manifest, manifestHash, err := s.readSlotManifestFromCache(ref)
	if err != nil {
		return core.SaveSyncSlotSummary{}, err
	}
	if manifest == nil {
		summary := core.SaveSyncSlotSummary{SlotID: ref.SlotID, Exists: false}
		if status != nil {
			summary.Cached = true
			summary.SyncState = status.SyncState
			summary.LastSyncError = status.LastSyncError
			summary.UploadPending = status.SyncState == "uploading"
		}
		return summary, nil
	}
	summary := core.SaveSyncSlotSummary{
		SlotID:       ref.SlotID,
		Exists:       true,
		ManifestHash: manifestHash,
		UpdatedAt:    manifest.UpdatedAt.Format(time.RFC3339),
		FileCount:    manifest.FileCount,
		TotalSize:    manifest.TotalSize,
		Cached:       true,
		SyncState:    "synced",
	}
	if status != nil {
		summary.SyncState = status.SyncState
		summary.LastSyncError = status.LastSyncError
		summary.UploadPending = status.SyncState == "uploading"
	}
	return summary, nil
}

func (s *service) readSlotSnapshotFromCache(ref core.SaveSyncSlotRef) (*core.SaveSyncSnapshot, error) {
	manifest, manifestHash, err := s.readSlotManifestFromCache(ref)
	if err != nil || manifest == nil {
		return nil, err
	}
	archiveBytes, err := os.ReadFile(s.cacheArchivePath(ref))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read cached slot archive: %w", err)
	}
	return &core.SaveSyncSnapshot{
		ManifestHash:    manifestHash,
		CanonicalGameID: manifest.CanonicalGameID,
		SourceGameID:    manifest.SourceGameID,
		Runtime:         manifest.Runtime,
		SlotID:          manifest.SlotID,
		UpdatedAt:       manifest.UpdatedAt,
		TotalSize:       manifest.TotalSize,
		FileCount:       manifest.FileCount,
		Files:           manifest.Files,
		ArchiveBase64:   base64.StdEncoding.EncodeToString(archiveBytes),
	}, nil
}

func (s *service) readSlotManifestFromCache(ref core.SaveSyncSlotRef) (*saveSyncStoredManifest, string, error) {
	data, err := os.ReadFile(s.cacheManifestPath(ref))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("read cached slot manifest: %w", err)
	}
	var manifest saveSyncStoredManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, "", fmt.Errorf("parse cached slot manifest: %w", err)
	}
	return &manifest, hashBytes(data), nil
}

func (s *service) readSlotCacheStatus(ref core.SaveSyncSlotRef) (*saveSyncCacheStatus, error) {
	data, err := os.ReadFile(s.cacheStatusPath(ref))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read save-sync cache status: %w", err)
	}
	var status saveSyncCacheStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("parse save-sync cache status: %w", err)
	}
	return &status, nil
}

func (s *service) writeSlotCache(ref core.SaveSyncSlotRef, manifestBytes, archiveBytes []byte, status saveSyncCacheStatus) error {
	dir := s.cacheSlotDir(ref)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create save-sync cache dir: %w", err)
	}
	if err := atomicWriteFile(s.cacheArchivePath(ref), archiveBytes, 0o644); err != nil {
		return fmt.Errorf("write cached slot archive: %w", err)
	}
	if err := atomicWriteFile(s.cacheManifestPath(ref), manifestBytes, 0o644); err != nil {
		return fmt.Errorf("write cached slot manifest: %w", err)
	}
	return s.writeSlotCacheStatus(ref, status)
}

func (s *service) writeSlotCacheStatus(ref core.SaveSyncSlotRef, status saveSyncCacheStatus) error {
	if status.UpdatedAt.IsZero() {
		status.UpdatedAt = time.Now().UTC()
	}
	dir := s.cacheSlotDir(ref)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create save-sync cache dir: %w", err)
	}
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("marshal save-sync cache status: %w", err)
	}
	return atomicWriteFile(s.cacheStatusPath(ref), data, 0o644)
}

func (s *service) removeSlotCachePayload(ref core.SaveSyncSlotRef) error {
	for _, filePath := range []string{s.cacheManifestPath(ref), s.cacheArchivePath(ref)} {
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (s *service) enqueueUpload(ref core.SaveSyncSlotRef) {
	key := s.slotQueueKey(ref)
	s.mu.Lock()
	worker := s.uploadWorkers[key]
	if worker == nil {
		worker = &slotUploadWorker{}
		s.uploadWorkers[key] = worker
	}
	worker.pending = true
	if worker.running {
		s.mu.Unlock()
		return
	}
	worker.running = true
	s.mu.Unlock()

	go s.runUploadWorker(key, ref)
}

func (s *service) runUploadWorker(key string, ref core.SaveSyncSlotRef) {
	for {
		s.mu.Lock()
		worker := s.uploadWorkers[key]
		if worker == nil || !worker.pending {
			if worker != nil {
				worker.running = false
			}
			s.mu.Unlock()
			return
		}
		worker.pending = false
		s.mu.Unlock()

		if err := s.uploadSlotFromCache(ref); err != nil {
			s.logger.Error("save-sync upload failed", err, "slot_id", ref.SlotID, "source_game_id", ref.SourceGameID)
			_ = s.markSlotUploadFailed(ref, err)
		}
	}
}

func (s *service) uploadSlotFromCache(ref core.SaveSyncSlotRef) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, integration, err := s.resolveIntegrationForSaveSync(ctx, ref.IntegrationID)
	if err != nil {
		return err
	}
	manifestBytes, err := os.ReadFile(s.cacheManifestPath(ref))
	if err != nil {
		return fmt.Errorf("read cached manifest for upload: %w", err)
	}
	archiveBytes, err := os.ReadFile(s.cacheArchivePath(ref))
	if err != nil {
		return fmt.Errorf("read cached archive for upload: %w", err)
	}
	if err := s.putObject(ctx, integration, slotArchivePath(ref), archiveBytes, "application/zip"); err != nil {
		return err
	}
	if err := s.putObject(ctx, integration, slotManifestPath(ref), manifestBytes, "application/json"); err != nil {
		return err
	}
	return s.writeSlotCacheStatus(ref, saveSyncCacheStatus{
		SyncState:          "synced",
		RemoteManifestHash: hashBytes(manifestBytes),
		UpdatedAt:          time.Now().UTC(),
	})
}

func (s *service) markSlotUploadFailed(ref core.SaveSyncSlotRef, err error) error {
	status, _ := s.readSlotCacheStatus(ref)
	if status == nil {
		status = &saveSyncCacheStatus{}
	}
	status.SyncState = "failed"
	status.LastSyncError = err.Error()
	status.UpdatedAt = time.Now().UTC()
	return s.writeSlotCacheStatus(ref, *status)
}

func (s *service) fetchArchiveBytes(ctx context.Context, integration *core.Integration, ref core.SaveSyncSlotRef) ([]byte, error) {
	var archiveResult struct {
		Status     string `json:"status"`
		DataBase64 string `json:"data_base64,omitempty"`
	}
	if err := s.pluginHost.Call(ctx, integration.PluginID, "save_sync.get", map[string]any{
		"config": integrationConfig(integration),
		"path":   slotArchivePath(ref),
	}, &archiveResult); err != nil {
		return nil, fmt.Errorf("load slot archive: %w", err)
	}
	if archiveResult.Status == "not_found" {
		return nil, nil
	}
	if archiveResult.Status != "" && archiveResult.Status != "ok" {
		return nil, fmt.Errorf("load slot archive failed: %s", archiveResult.Status)
	}
	archiveBytes, err := base64.StdEncoding.DecodeString(archiveResult.DataBase64)
	if err != nil {
		return nil, fmt.Errorf("decode slot archive: %w", err)
	}
	return archiveBytes, nil
}

func (s *service) validateSlotListRequest(ctx context.Context, req core.SaveSyncListRequest) error {
	return s.validateSlotRef(ctx, core.SaveSyncSlotRef{
		CanonicalGameID: req.CanonicalGameID,
		SourceGameID:    req.SourceGameID,
		Runtime:         req.Runtime,
		SlotID:          "autosave",
		IntegrationID:   req.IntegrationID,
	})
}

func (s *service) validateSlotRef(ctx context.Context, req core.SaveSyncSlotRef) error {
	if req.CanonicalGameID == "" || req.SourceGameID == "" || req.IntegrationID == "" {
		return fmt.Errorf("canonical_game_id, source_game_id, and integration_id are required")
	}
	if _, ok := browserRuntimeAllowed(req.Runtime); !ok {
		return fmt.Errorf("unsupported runtime")
	}
	if req.SlotID != "" && !isKnownSlotID(req.SlotID) {
		return fmt.Errorf("unsupported slot_id")
	}

	game, err := s.gameStore.GetCanonicalGameByID(ctx, req.CanonicalGameID)
	if err != nil {
		return fmt.Errorf("load game: %w", err)
	}
	if game == nil {
		return fmt.Errorf("game not found")
	}
	return validateSlotRefAgainstGame(game, req)
}

func validateSlotRefAgainstGame(game *core.CanonicalGame, req core.SaveSyncSlotRef) error {
	if game == nil {
		return fmt.Errorf("game not found")
	}
	for _, sourceGame := range game.SourceGames {
		if sourceGame != nil && sourceGame.ID == req.SourceGameID {
			expectedRuntime, ok := core.BrowserPlayRuntimeForSourceGame(sourceGame.Platform, game.Platform)
			if !ok || expectedRuntime != req.Runtime {
				return fmt.Errorf("runtime does not match source game platform")
			}
			return nil
		}
	}
	return fmt.Errorf("source game does not belong to canonical game")
}

func (s *service) readSlotSummary(ctx context.Context, ref core.SaveSyncSlotRef) (core.SaveSyncSlotSummary, error) {
	_, integration, err := s.resolveIntegrationForSaveSync(ctx, ref.IntegrationID)
	if err != nil {
		return core.SaveSyncSlotSummary{}, err
	}
	return s.readSlotSummaryFromIntegration(ctx, integration, ref)
}

func (s *service) readSlotSummaryFromIntegration(ctx context.Context, integration *core.Integration, ref core.SaveSyncSlotRef) (core.SaveSyncSlotSummary, error) {
	manifest, manifestHash, err := s.fetchStoredManifest(ctx, integration, ref)
	if err != nil {
		return core.SaveSyncSlotSummary{}, err
	}
	if manifest == nil {
		return core.SaveSyncSlotSummary{SlotID: ref.SlotID, Exists: false}, nil
	}
	return core.SaveSyncSlotSummary{
		SlotID:       ref.SlotID,
		Exists:       true,
		ManifestHash: manifestHash,
		UpdatedAt:    manifest.UpdatedAt.Format(time.RFC3339),
		FileCount:    manifest.FileCount,
		TotalSize:    manifest.TotalSize,
	}, nil
}

func (s *service) listExistingManifestPaths(ctx context.Context, integration *core.Integration, req core.SaveSyncListRequest) (map[string]bool, error) {
	prefix := path.Join(
		"integrations",
		req.IntegrationID,
		"games",
		req.CanonicalGameID,
		req.SourceGameID,
		req.Runtime,
	)
	var result struct {
		Status string `json:"status"`
		Files  []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := s.pluginHost.Call(ctx, integration.PluginID, "save_sync.list", map[string]any{
		"config": integrationConfig(integration),
		"prefix": prefix,
	}, &result); err != nil {
		return nil, fmt.Errorf("list slot manifests: %w", err)
	}
	if result.Status != "" && result.Status != "ok" {
		return nil, fmt.Errorf("list slot manifests failed: %s", result.Status)
	}

	paths := make(map[string]bool, len(result.Files))
	for _, file := range result.Files {
		normalized := path.Clean(strings.TrimPrefix(strings.ReplaceAll(file.Path, "\\", "/"), "/"))
		if path.Base(normalized) == "manifest.json" {
			paths[normalized] = true
			paths[path.Join(prefix, normalized)] = true
		}
	}
	return paths, nil
}

func (s *service) fetchStoredManifest(ctx context.Context, integration *core.Integration, ref core.SaveSyncSlotRef) (*saveSyncStoredManifest, string, error) {
	var result struct {
		Status     string `json:"status"`
		DataBase64 string `json:"data_base64,omitempty"`
	}
	if err := s.pluginHost.Call(ctx, integration.PluginID, "save_sync.get", map[string]any{
		"config": integrationConfig(integration),
		"path":   slotManifestPath(ref),
	}, &result); err != nil {
		return nil, "", fmt.Errorf("load slot manifest: %w", err)
	}
	if result.Status == "not_found" {
		return nil, "", nil
	}
	if result.Status != "" && result.Status != "ok" {
		return nil, "", fmt.Errorf("load slot manifest failed: %s", result.Status)
	}

	manifestBytes, err := base64.StdEncoding.DecodeString(result.DataBase64)
	if err != nil {
		return nil, "", fmt.Errorf("decode slot manifest: %w", err)
	}
	var manifest saveSyncStoredManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, "", fmt.Errorf("parse slot manifest: %w", err)
	}
	manifestHash := hashBytes(manifestBytes)
	return &manifest, manifestHash, nil
}

func (s *service) putObject(ctx context.Context, integration *core.Integration, objectPath string, data []byte, contentType string) error {
	var result struct {
		Status string `json:"status"`
	}
	if err := s.pluginHost.Call(ctx, integration.PluginID, "save_sync.put", map[string]any{
		"config":       integrationConfig(integration),
		"path":         objectPath,
		"data_base64":  base64.StdEncoding.EncodeToString(data),
		"content_type": contentType,
	}, &result); err != nil {
		return fmt.Errorf("save object %s: %w", objectPath, err)
	}
	if result.Status != "" && result.Status != "ok" {
		return fmt.Errorf("save object %s failed: %s", objectPath, result.Status)
	}
	return nil
}

func (s *service) deleteSlot(ctx context.Context, ref core.SaveSyncSlotRef) error {
	_, integration, err := s.resolveIntegrationForSaveSync(ctx, ref.IntegrationID)
	if err != nil {
		return err
	}
	for _, objectPath := range []string{slotManifestPath(ref), slotArchivePath(ref)} {
		var result struct {
			Status string `json:"status"`
		}
		if err := s.pluginHost.Call(ctx, integration.PluginID, "save_sync.delete", map[string]any{
			"config": integrationConfig(integration),
			"path":   objectPath,
		}, &result); err != nil {
			return fmt.Errorf("delete %s: %w", objectPath, err)
		}
		if result.Status != "" && result.Status != "ok" && result.Status != "not_found" {
			return fmt.Errorf("delete %s failed: %s", objectPath, result.Status)
		}
	}
	return nil
}

func (s *service) resolveIntegrationForSaveSync(ctx context.Context, integrationID string) (map[string]any, *core.Integration, error) {
	integration, err := s.integrationRepo.GetByID(ctx, integrationID)
	if err != nil || integration == nil {
		return nil, nil, fmt.Errorf("integration not found")
	}
	plugin, ok := s.pluginHost.GetPlugin(integration.PluginID)
	if !ok {
		return nil, nil, fmt.Errorf("plugin not found: %s", integration.PluginID)
	}
	if !pluginProvides(plugin.Manifest.Provides, "save_sync.get") || !pluginProvides(plugin.Manifest.Provides, "save_sync.put") {
		return nil, nil, fmt.Errorf("integration does not support save sync")
	}
	return integrationConfig(integration), integration, nil
}

func integrationConfig(integration *core.Integration) map[string]any {
	cfg := map[string]any{}
	if integration == nil || integration.ConfigJSON == "" {
		return cfg
	}
	_ = json.Unmarshal([]byte(integration.ConfigJSON), &cfg)
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg
}

func browserRuntimeAllowed(runtime string) (string, bool) {
	switch runtime {
	case "emulatorjs", "jsdos", "scummvm":
		return runtime, true
	default:
		return "", false
	}
}

func slotBasePath(ref core.SaveSyncSlotRef) string {
	return path.Join(
		"integrations",
		ref.IntegrationID,
		"games",
		ref.CanonicalGameID,
		ref.SourceGameID,
		ref.Runtime,
		ref.SlotID,
	)
}

func slotManifestPath(ref core.SaveSyncSlotRef) string {
	return path.Join(slotBasePath(ref), "manifest.json")
}

func slotArchivePath(ref core.SaveSyncSlotRef) string {
	return path.Join(slotBasePath(ref), "snapshot.zip")
}

func (s *service) cacheSlotDir(ref core.SaveSyncSlotRef) string {
	return filepath.Join(
		s.cacheRoot,
		s.safeCacheSegment(ref.IntegrationID),
		s.safeCacheSegment(ref.CanonicalGameID),
		s.safeCacheSegment(ref.SourceGameID),
		s.safeCacheSegment(ref.Runtime),
		s.safeCacheSegment(ref.SlotID),
	)
}

func (s *service) cacheManifestPath(ref core.SaveSyncSlotRef) string {
	return filepath.Join(s.cacheSlotDir(ref), "manifest.json")
}

func (s *service) cacheArchivePath(ref core.SaveSyncSlotRef) string {
	return filepath.Join(s.cacheSlotDir(ref), "snapshot.zip")
}

func (s *service) cacheStatusPath(ref core.SaveSyncSlotRef) string {
	return filepath.Join(s.cacheSlotDir(ref), "status.json")
}

func (s *service) safeCacheSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	return s.cachePathReplacer.ReplaceAllString(value, "_")
}

func (s *service) slotQueueKey(ref core.SaveSyncSlotRef) string {
	return strings.Join([]string{
		ref.IntegrationID,
		ref.CanonicalGameID,
		ref.SourceGameID,
		ref.Runtime,
		ref.SlotID,
	}, "\x00")
}

func defaultSaveSyncCacheRoot() string {
	if dir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "MyGamesAnywhere", "save-sync")
	}
	return filepath.Join(".", "save-sync-cache")
}

func atomicWriteFile(dest string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp-" + uuid.New().String()
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	var removeErr error
	for attempt := 0; attempt < 5; attempt++ {
		removeErr = os.Remove(dest)
		if removeErr == nil || os.IsNotExist(removeErr) {
			removeErr = nil
			break
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	if removeErr != nil {
		_ = os.Remove(tmp)
		return removeErr
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func emulatorJSSlotIDs() []string {
	return []string{
		"state-1",
		"state-2",
		"state-3",
		"state-4",
		"state-5",
		"state-6",
		"state-7",
		"state-8",
		"state-9",
		"save-ram",
	}
}

func pluginProvides(provides []string, value string) bool {
	for _, item := range provides {
		if item == value {
			return true
		}
	}
	return false
}

func marshalManifest(manifest saveSyncStoredManifest) ([]byte, string, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", fmt.Errorf("marshal slot manifest: %w", err)
	}
	return data, hashBytes(data), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sumSnapshotSize(files []core.SaveSyncSnapshotFile) int64 {
	var total int64
	for _, file := range files {
		total += file.Size
	}
	return total
}

func validateArchive(archiveBytes []byte, files []core.SaveSyncSnapshotFile) error {
	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return fmt.Errorf("invalid snapshot archive: %w", err)
	}

	expected := make(map[string]core.SaveSyncSnapshotFile, len(files))
	for _, file := range files {
		expected[file.Path] = file
	}
	if len(expected) != len(files) {
		return fmt.Errorf("snapshot file list contains duplicate paths")
	}

	seen := make(map[string]bool, len(reader.File))
	for _, zf := range reader.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		normalizedPath := path.Clean(strings.TrimPrefix(strings.ReplaceAll(zf.Name, "\\", "/"), "/"))
		if strings.HasPrefix(normalizedPath, "../") || normalizedPath == ".." {
			return fmt.Errorf("snapshot archive contains invalid path")
		}
		meta, ok := expected[normalizedPath]
		if !ok {
			return fmt.Errorf("snapshot archive contains undeclared file %q", normalizedPath)
		}
		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("open archived file %q: %w", normalizedPath, err)
		}
		bytes, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("read archived file %q: %w", normalizedPath, err)
		}
		if int64(len(bytes)) != meta.Size {
			return fmt.Errorf("snapshot file size mismatch for %q", normalizedPath)
		}
		if hashBytes(bytes) != meta.Hash {
			return fmt.Errorf("snapshot file hash mismatch for %q", normalizedPath)
		}
		seen[normalizedPath] = true
	}

	if len(seen) != len(expected) {
		return fmt.Errorf("snapshot archive is missing declared files")
	}
	return nil
}

func isKnownSlotID(slotID string) bool {
	for _, candidate := range slotIDs {
		if candidate == slotID {
			return true
		}
	}
	return false
}
