package save_sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/savehistory"
	"github.com/google/uuid"
)

type saveHistoryManager struct {
	repository savehistory.Repository
	root       string
	now        func() time.Time
}

func (m *saveHistoryManager) ArchiveCurrent(ctx context.Context, ref core.SaveSyncSlotRef, cache *service) error {
	if m == nil || m.repository == nil || cache == nil || m.now == nil {
		return fmt.Errorf("save history is unavailable")
	}
	manifestBytes, err := os.ReadFile(cache.cacheManifestPath(ref))
	if err != nil {
		return fmt.Errorf("read current save manifest: %w", err)
	}
	archiveBytes, err := os.ReadFile(cache.cacheArchivePath(ref))
	if err != nil {
		return fmt.Errorf("read current save archive: %w", err)
	}
	var manifest saveSyncStoredManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("parse current save manifest: %w", err)
	}
	if manifest.SaveDomainID == "" {
		manifest.SaveDomainID = ref.SaveDomainID
	}
	manifest.OriginLabel = safeSaveEvidenceLabel(manifest.OriginLabel, "Earlier MGA save")
	manifest.RouteLabel = safeSaveEvidenceLabel(manifest.RouteLabel, browserRouteLabel(manifest.Runtime))

	payloadKey := uuid.NewString()
	payloadDir := filepath.Join(m.root, payloadKey)
	if err := os.MkdirAll(payloadDir, 0o700); err != nil {
		return fmt.Errorf("create save history staging: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(payloadDir)
		}
	}()
	if err := atomicWriteFile(filepath.Join(payloadDir, "manifest.json"), manifestBytes, 0o600); err != nil {
		return err
	}
	if err := atomicWriteFile(filepath.Join(payloadDir, "save.zip"), archiveBytes, 0o600); err != nil {
		return err
	}
	policy, err := m.repository.GetPolicy(ctx, ref.OwnerProfileID, ref.SaveDomainID)
	if err != nil {
		return err
	}
	version := savehistory.Version{
		ID: uuid.NewString(), ProfileID: ref.OwnerProfileID, DomainID: ref.SaveDomainID,
		CanonicalGameID: ref.CanonicalGameID, SourceGameID: ref.SourceGameID, Runtime: ref.Runtime,
		SlotID: ref.SlotID, IntegrationID: ref.IntegrationID, ManifestHash: hashBytes(manifestBytes),
		OriginLabel: manifest.OriginLabel, RouteLabel: manifest.RouteLabel, AcceptedAt: manifest.UpdatedAt,
		ReportedAt: manifest.ReportedAt, FileCount: manifest.FileCount, TotalSize: manifest.TotalSize, PayloadKey: payloadKey,
	}
	pruned, err := m.repository.RecordVersion(ctx, version, policy)
	if err != nil {
		return err
	}
	cleanup = false
	for _, item := range pruned {
		_ = os.RemoveAll(filepath.Join(m.root, item.PayloadKey))
	}
	return nil
}

func (s *service) GetSaveDomainHistory(ctx context.Context, domainID string) (*core.SaveDomainHistory, error) {
	profile, err := requireSaveSyncProfile(ctx)
	if err != nil {
		return nil, err
	}
	if s.history == nil {
		return nil, fmt.Errorf("save history is unavailable")
	}
	policy, err := s.history.repository.GetPolicy(ctx, profile.ID, domainID)
	if err != nil {
		return nil, err
	}
	versions, err := s.history.repository.ListVersions(ctx, profile.ID, domainID)
	if err != nil {
		return nil, err
	}
	return historyResponse(policy, versions), nil
}

func (s *service) SetSaveDomainHistoryPolicy(ctx context.Context, request core.SaveDomainHistoryPolicy) (*core.SaveDomainHistory, error) {
	profile, err := requireSaveSyncProfile(ctx)
	if err != nil {
		return nil, err
	}
	if s.history == nil {
		return nil, fmt.Errorf("save history is unavailable")
	}
	policy := savehistory.Policy{
		ProfileID: profile.ID, DomainID: request.DomainID,
		RetainVersions: request.RetainVersions, RetainDays: request.RetainDays,
	}
	if err := s.history.repository.UpsertPolicy(ctx, policy); err != nil {
		return nil, err
	}
	return s.GetSaveDomainHistory(ctx, request.DomainID)
}

func (s *service) RecoverSaveDomainVersion(ctx context.Context, versionID string) (*core.SaveSyncPutResult, error) {
	profile, err := requireSaveSyncProfile(ctx)
	if err != nil {
		return nil, err
	}
	if s.history == nil {
		return nil, fmt.Errorf("save history is unavailable")
	}
	version, err := s.history.repository.GetVersion(ctx, profile.ID, versionID)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, fmt.Errorf("retained save version not found")
	}
	ref := core.SaveSyncSlotRef{
		OwnerProfileID: profile.ID, CanonicalGameID: version.CanonicalGameID, SourceGameID: version.SourceGameID,
		Runtime: version.Runtime, SlotID: version.SlotID, IntegrationID: version.IntegrationID,
		SaveDomainID: version.DomainID, OriginLabel: "Recovered MGA save", RouteLabel: version.RouteLabel,
	}
	if err := s.validateSlotRef(ctx, ref); err != nil {
		return nil, err
	}
	if _, _, err := s.resolveIntegrationForSaveSync(ctx, ref.IntegrationID); err != nil {
		return nil, err
	}
	manifestBytes, archiveBytes, err := s.history.ReadPayload(version.PayloadKey)
	if err != nil {
		return nil, err
	}
	if hashBytes(manifestBytes) != version.ManifestHash {
		return nil, fmt.Errorf("retained save manifest no longer matches its evidence")
	}
	var retained saveSyncStoredManifest
	if err := json.Unmarshal(manifestBytes, &retained); err != nil {
		return nil, fmt.Errorf("parse retained save manifest: %w", err)
	}
	if err := validateArchive(archiveBytes, retained.Files); err != nil {
		return nil, fmt.Errorf("validate retained save archive: %w", err)
	}
	unlock := s.lockSlot(ref)
	defer unlock()
	current, err := s.readSlotSummaryFromCache(ref)
	if err != nil {
		return nil, err
	}
	if current.Exists {
		if err := s.history.ArchiveCurrent(ctx, ref, s); err != nil {
			return nil, fmt.Errorf("retain current save before recovery: %w", err)
		}
	}
	now := time.Now().UTC()
	retained.Version = 2
	retained.UpdatedAt = now
	retained.SaveDomainID = version.DomainID
	retained.OriginLabel = "Recovered MGA save"
	retained.RouteLabel = version.RouteLabel
	manifestBytes, manifestHash, err := marshalManifest(retained)
	if err != nil {
		return nil, err
	}
	if err := s.replaceSlotCacheWithRollback(ref, manifestBytes, archiveBytes, saveSyncCacheStatus{SyncState: "uploading", UpdatedAt: now}); err != nil {
		return nil, err
	}
	s.enqueueUpload(ref, profile)
	return &core.SaveSyncPutResult{OK: true, Summary: core.SaveSyncSlotSummary{
		SlotID: ref.SlotID, Exists: true, ManifestHash: manifestHash, UpdatedAt: now.Format(time.RFC3339),
		FileCount: retained.FileCount, TotalSize: retained.TotalSize, Cached: true, SyncState: "uploading",
		UploadPending: true, OriginLabel: retained.OriginLabel, RouteLabel: retained.RouteLabel,
	}}, nil
}

func (m *saveHistoryManager) ReadPayload(payloadKey string) ([]byte, []byte, error) {
	dir := filepath.Join(m.root, payloadKey)
	manifest, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("read retained save manifest: %w", err)
	}
	archive, err := os.ReadFile(filepath.Join(dir, "save.zip"))
	if err != nil {
		return nil, nil, fmt.Errorf("read retained save archive: %w", err)
	}
	return manifest, archive, nil
}

func (s *service) replaceSlotCacheWithRollback(ref core.SaveSyncSlotRef, manifest, archive []byte, status saveSyncCacheStatus) error {
	oldManifest, manifestErr := os.ReadFile(s.cacheManifestPath(ref))
	oldArchive, archiveErr := os.ReadFile(s.cacheArchivePath(ref))
	oldStatus, statusErr := os.ReadFile(s.cacheStatusPath(ref))
	if err := s.writeSlotCache(ref, manifest, archive, status); err == nil {
		return nil
	} else {
		if manifestErr == nil {
			_ = atomicWriteFile(s.cacheManifestPath(ref), oldManifest, 0o644)
		}
		if archiveErr == nil {
			_ = atomicWriteFile(s.cacheArchivePath(ref), oldArchive, 0o644)
		}
		if statusErr == nil {
			_ = atomicWriteFile(s.cacheStatusPath(ref), oldStatus, 0o644)
		}
		return fmt.Errorf("activate retained save snapshot: %w", err)
	}
}

func historyResponse(policy savehistory.Policy, versions []savehistory.Version) *core.SaveDomainHistory {
	result := &core.SaveDomainHistory{
		Policy:   core.SaveDomainHistoryPolicy{DomainID: policy.DomainID, RetainVersions: policy.RetainVersions, RetainDays: policy.RetainDays},
		Versions: make([]core.SaveDomainHistoryVersion, len(versions)),
	}
	for index, version := range versions {
		result.Versions[index] = core.SaveDomainHistoryVersion{
			ID: version.ID, DomainID: version.DomainID, ManifestHash: version.ManifestHash,
			OriginLabel: version.OriginLabel, RouteLabel: version.RouteLabel, AcceptedAt: version.AcceptedAt,
			ReportedAt: version.ReportedAt, FileCount: version.FileCount, TotalSize: version.TotalSize,
		}
	}
	return result
}
