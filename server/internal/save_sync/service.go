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
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/google/uuid"
)

var slotIDs = []string{"autosave", "slot-1", "slot-2", "slot-3", "slot-4", "slot-5"}

type PluginHost = plugins.PluginHost

type service struct {
	integrationRepo core.IntegrationRepository
	gameStore       core.GameStore
	pluginHost      PluginHost
	logger          core.Logger
	eventBus        *events.EventBus

	mu   sync.RWMutex
	jobs map[string]*core.SaveSyncMigrationStatus
}

type saveSyncStoredManifest struct {
	Version         int                       `json:"version"`
	CanonicalGameID string                    `json:"canonical_game_id"`
	SourceGameID    string                    `json:"source_game_id"`
	Runtime         string                    `json:"runtime"`
	SlotID          string                    `json:"slot_id"`
	UpdatedAt       time.Time                 `json:"updated_at"`
	FileCount       int                       `json:"file_count"`
	TotalSize       int64                     `json:"total_size"`
	Files           []core.SaveSyncSnapshotFile `json:"files"`
}

type saveSyncPluginFile struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
}

func NewService(
	integrationRepo core.IntegrationRepository,
	gameStore core.GameStore,
	pluginHost PluginHost,
	logger core.Logger,
	eventBus *events.EventBus,
) core.SaveSyncService {
	return &service{
		integrationRepo: integrationRepo,
		gameStore:       gameStore,
		pluginHost:      pluginHost,
		logger:          logger,
		eventBus:        eventBus,
		jobs:            make(map[string]*core.SaveSyncMigrationStatus),
	}
}

func (s *service) ListSlots(ctx context.Context, req core.SaveSyncListRequest) ([]core.SaveSyncSlotSummary, error) {
	if err := s.validateSlotListRequest(ctx, req); err != nil {
		return nil, err
	}

	summaries := make([]core.SaveSyncSlotSummary, 0, len(slotIDs))
	for _, slotID := range slotIDs {
		summary, err := s.readSlotSummary(ctx, core.SaveSyncSlotRef{
			CanonicalGameID: req.CanonicalGameID,
			SourceGameID:    req.SourceGameID,
			Runtime:         req.Runtime,
			SlotID:          slotID,
			IntegrationID:   req.IntegrationID,
		})
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
	_, integration, err := s.resolveIntegrationForSaveSync(ctx, req.IntegrationID)
	if err != nil {
		return nil, err
	}

	manifest, manifestHash, err := s.fetchStoredManifest(ctx, integration, req)
	if err != nil || manifest == nil {
		return nil, err
	}

	archivePath := slotArchivePath(req)
	var archiveResult struct {
		Status     string `json:"status"`
		DataBase64 string `json:"data_base64,omitempty"`
	}
	if err := s.pluginHost.Call(ctx, integration.PluginID, "save_sync.get", map[string]any{
		"config": integrationConfig(integration),
		"path":   archivePath,
	}, &archiveResult); err != nil {
		return nil, fmt.Errorf("load slot archive: %w", err)
	}
	if archiveResult.Status == "not_found" {
		return nil, nil
	}
	if archiveResult.Status != "" && archiveResult.Status != "ok" {
		return nil, fmt.Errorf("load slot archive failed: %s", archiveResult.Status)
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
		ArchiveBase64:   archiveResult.DataBase64,
	}, nil
}

func (s *service) PutSlot(ctx context.Context, req core.SaveSyncPutRequest) (*core.SaveSyncPutResult, error) {
	if err := s.validatePutRequest(ctx, req); err != nil {
		return nil, err
	}
	_, integration, err := s.resolveIntegrationForSaveSync(ctx, req.IntegrationID)
	if err != nil {
		return nil, err
	}

	currentManifest, currentHash, err := s.fetchStoredManifest(ctx, integration, req.SaveSyncSlotRef)
	if err != nil {
		return nil, err
	}
	if currentManifest != nil && !req.Force && req.BaseManifestHash != currentHash {
		return &core.SaveSyncPutResult{
			OK: false,
			Summary: core.SaveSyncSlotSummary{
				SlotID: req.SlotID,
				Exists: true,
			},
			Conflict: &core.SaveSyncConflict{
				SlotID:             req.SlotID,
				Message:            "remote slot changed since the last known manifest",
				RemoteManifestHash: currentHash,
				RemoteUpdatedAt:    currentManifest.UpdatedAt.Format(time.RFC3339),
				RemoteFileCount:    currentManifest.FileCount,
				RemoteTotalSize:    currentManifest.TotalSize,
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

	if err := s.putObject(ctx, integration, slotArchivePath(req.SaveSyncSlotRef), archiveBytes, "application/zip"); err != nil {
		return nil, err
	}
	if err := s.putObject(ctx, integration, slotManifestPath(req.SaveSyncSlotRef), manifestBytes, "application/json"); err != nil {
		return nil, err
	}

	return &core.SaveSyncPutResult{
		OK: true,
		Summary: core.SaveSyncSlotSummary{
			SlotID:       req.SlotID,
			Exists:       true,
			ManifestHash: manifestHash,
			UpdatedAt:    updatedAt.Format(time.RFC3339),
			FileCount:    manifest.FileCount,
			TotalSize:    manifest.TotalSize,
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
			runtime, ok := browserRuntimeForPlatform(sourceGame.Platform)
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
		"job_id":                 status.JobID,
		"status":                 status.Status,
		"scope":                  status.Scope,
		"source_integration_id":  status.SourceIntegrationID,
		"target_integration_id":  status.TargetIntegrationID,
		"canonical_game_id":      status.CanonicalGameID,
		"started_at":             status.StartedAt,
		"finished_at":            status.FinishedAt,
		"items_total":            status.ItemsTotal,
		"items_completed":        status.ItemsCompleted,
		"slots_migrated":         status.SlotsMigrated,
		"slots_skipped":          status.SlotsSkipped,
		"error":                  status.Error,
	})
}

func cloneMigrationStatus(in *core.SaveSyncMigrationStatus) *core.SaveSyncMigrationStatus {
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
	for _, sourceGame := range game.SourceGames {
		if sourceGame != nil && sourceGame.ID == req.SourceGameID && sourceGame.Status == "found" {
			expectedRuntime, ok := browserRuntimeForPlatform(sourceGame.Platform)
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

func browserRuntimeForPlatform(platform core.Platform) (string, bool) {
	switch platform {
	case core.PlatformNES, core.PlatformSNES, core.PlatformGB, core.PlatformGBC, core.PlatformGBA,
		core.PlatformGenesis, core.PlatformSegaMasterSystem, core.PlatformGameGear, core.PlatformSegaCD,
		core.PlatformSega32X, core.PlatformPS1, core.PlatformArcade:
		return "emulatorjs", true
	case core.PlatformMSDOS:
		return "jsdos", true
	case core.PlatformScummVM:
		return "scummvm", true
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
