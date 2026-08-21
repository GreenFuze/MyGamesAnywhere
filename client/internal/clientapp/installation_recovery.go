package clientapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/desktop"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

var ErrInstallationRecoveryDeclined = errors.New("installation recovery was canceled")

// InstallationRecoveryManager owns the bounded repair/release operations for
// one server binding. It never accepts an unowned catalog record.
type InstallationRecoveryManager struct {
	ownership *InstallationOwnership
	serverURL string
	validator *LocalInstallationValidator
}

func NewInstallationRecoveryManager(ownership *InstallationOwnership, serverURL string, programs RegisteredProgramInspector) (*InstallationRecoveryManager, error) {
	if ownership == nil {
		return nil, errors.New("installation ownership is required")
	}
	if strings.TrimSpace(serverURL) == "" {
		return nil, errors.New("MGA Server URL is required")
	}
	validator, err := NewLocalInstallationValidator(programs)
	if err != nil {
		return nil, err
	}
	return &InstallationRecoveryManager{ownership: ownership, serverURL: serverURL, validator: validator}, nil
}

func (m *InstallationRecoveryManager) Recover(ctx context.Context, request devicev1.InstallationRecoveryRequest, report CommandProgressReporter) (devicev1.InstallationRecoveryResult, error) {
	var result devicev1.InstallationRecoveryResult
	if m == nil || m.ownership == nil || m.validator == nil {
		return result, errors.New("installation recovery is unavailable")
	}
	if err := request.Validate(); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	record, err := m.resolveOwnedRecord(request)
	if err != nil {
		return result, err
	}
	switch request.Action {
	case devicev1.InstallationRecoveryRepair:
		return m.repair(ctx, record, request, report)
	case devicev1.InstallationRecoveryReinstall, devicev1.InstallationRecoveryForget:
		return m.release(ctx, record, request, report)
	default:
		return result, fmt.Errorf("unsupported recovery action %q", request.Action)
	}
}

func (m *InstallationRecoveryManager) resolveOwnedRecord(request devicev1.InstallationRecoveryRequest) (InstallationOwnershipRecord, error) {
	record, found := m.ownership.catalog.FindByPath(request.InstallPath)
	if !found {
		return InstallationOwnershipRecord{}, errors.New("installation ownership record is missing")
	}
	if request.LocalInstallationID != "" && !strings.EqualFold(record.LocalInstallationID, request.LocalInstallationID) {
		return InstallationOwnershipRecord{}, errors.New("installation local identity does not match")
	}
	if record.State != OwnershipOwned || !strings.EqualFold(record.OwnerBindingID, m.ownership.bindingID) {
		return InstallationOwnershipRecord{}, errors.New("installation is managed by another MGA server")
	}
	if record.GameID != request.GameID || record.SourceGameID != request.SourceGameID ||
		record.InstallKind != request.InstallKind || !sameLocalPath(record.InstallRoot, request.InstallRoot) ||
		!sameLocalPath(record.InstallPath, request.InstallPath) {
		return InstallationOwnershipRecord{}, errors.New("installation recovery identity does not match the local catalog")
	}
	return record, nil
}

func (m *InstallationRecoveryManager) repair(ctx context.Context, record InstallationOwnershipRecord, request devicev1.InstallationRecoveryRequest, report CommandProgressReporter) (devicev1.InstallationRecoveryResult, error) {
	result := recoveryResult(record, request)
	if err := reportProgress(report, "checking", "Checking the managed game folder", 10, "", 0); err != nil {
		return result, err
	}
	if err := requireSafeInstallationDirectory(record); err != nil {
		return result, err
	}
	release, err := m.ownership.coordinator.Reserve(m.ownership.bindingID, record.InstallPath, record.ProductIdentity)
	if err != nil {
		return result, err
	}
	defer release()

	candidates, selected, err := discoverLaunchTargets(record.InstallPath, record.Title)
	if err != nil {
		return result, err
	}
	if selected == "" {
		if len(candidates) == 0 {
			return result, errors.New("repair found no safe game executable")
		}
		return result, errors.New("repair found more than one possible game executable; choose one manually")
	}
	if err := reportProgress(report, "repairing", "Updating how MGA starts the game", 55, "", 0); err != nil {
		return result, err
	}
	rollback, err := m.writeRepairedManifest(record, request, selected, candidates)
	if err != nil {
		return result, err
	}
	checked, err := m.validator.validateOne(devicev1.InstallationValidationRequestItem{
		GameID: request.GameID, SourceGameID: request.SourceGameID, InstallKind: request.InstallKind,
		InstallRoot: request.InstallRoot, InstallPath: request.InstallPath, LaunchTarget: selected,
		UninstallTarget: func() string {
			if request.InstallKind != devicev1.InstallKindGogInno {
				return ""
			}
			manifest, readErr := readGogInnoManifest(request.InstallPath)
			if readErr != nil {
				return ""
			}
			return manifest.UninstallTarget
		}(),
	})
	if err != nil || checked.State != devicev1.InstallStateInstalled {
		_ = rollback()
		if err != nil {
			return result, fmt.Errorf("verify repaired installation: %w", err)
		}
		return result, fmt.Errorf("repair did not restore a healthy installation: %s", checked.ReasonCode)
	}
	result.PathPresent = true
	result.LaunchTarget = selected
	result.LaunchCandidates = candidates
	if err := reportProgress(report, "complete", "Game repaired", 100, "", 0); err != nil {
		return result, err
	}
	return result, result.Validate()
}

func (m *InstallationRecoveryManager) writeRepairedManifest(record InstallationOwnershipRecord, request devicev1.InstallationRecoveryRequest, selected string, candidates []string) (func() error, error) {
	switch record.InstallKind {
	case devicev1.InstallKindManagedArchive:
		manifest, err := readInstallManifest(record.InstallPath)
		if err != nil {
			return nil, err
		}
		if !validOwnedArchiveManifest(manifest, record, request) {
			return nil, errors.New("archive manifest does not match the owned installation")
		}
		original := manifest
		manifest.LaunchTarget, manifest.LaunchCandidates = selected, candidates
		if err := writeJSONAtomic(filepath.Join(record.InstallPath, installManifestName), manifest); err != nil {
			return nil, err
		}
		return func() error { return writeJSONAtomic(filepath.Join(record.InstallPath, installManifestName), original) }, nil
	case devicev1.InstallKindGogInno:
		manifest, err := readGogInnoManifest(record.InstallPath)
		if err != nil {
			return nil, err
		}
		if !validOwnedGogManifest(manifest, record, request) {
			return nil, errors.New("GOG manifest does not match the owned installation")
		}
		original := manifest
		manifest.LaunchTarget, manifest.LaunchCandidates = selected, candidates
		if err := writeJSONAtomic(filepath.Join(record.InstallPath, installManifestName), manifest); err != nil {
			return nil, err
		}
		return func() error { return writeJSONAtomic(filepath.Join(record.InstallPath, installManifestName), original) }, nil
	default:
		return nil, fmt.Errorf("installation kind %q cannot be repaired", record.InstallKind)
	}
}

func (m *InstallationRecoveryManager) release(ctx context.Context, record InstallationOwnershipRecord, request devicev1.InstallationRecoveryRequest, report CommandProgressReporter) (devicev1.InstallationRecoveryResult, error) {
	result := recoveryResult(record, request)
	info, err := os.Lstat(record.InstallPath)
	pathPresent := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, fmt.Errorf("inspect installation path: %w", err)
	}
	if request.Action == devicev1.InstallationRecoveryReinstall && pathPresent {
		return result, errors.New("reinstall refused because the managed game folder exists")
	}
	if pathPresent {
		if !info.IsDir() {
			return result, errors.New("managed installation path is not a directory")
		}
		if err := requireSafeInstallationDirectory(record); err != nil {
			return result, err
		}
	}
	approved, err := desktop.ConfirmInstallationRecoveryRelease(ctx, record.Title, record.InstallPath, m.serverURL, request.Action, pathPresent)
	if err != nil {
		return result, err
	}
	if !approved {
		return result, ErrInstallationRecoveryDeclined
	}
	releaseLease, err := m.ownership.coordinator.Reserve(m.ownership.bindingID, record.InstallPath, record.ProductIdentity)
	if err != nil {
		return result, err
	}
	defer releaseLease()
	if err := reportProgress(report, "releasing", "Releasing this installation from MGA", 60, "", 0); err != nil {
		return result, err
	}
	if pathPresent {
		if err := rewriteManifestOwnership(record, "", OwnershipReleased); err != nil {
			return result, err
		}
	}
	if err := m.ownership.catalog.Release(record.LocalInstallationID, m.ownership.bindingID); err != nil {
		if pathPresent {
			_ = rewriteManifestOwnership(record, m.ownership.bindingID, OwnershipOwned)
		}
		return result, fmt.Errorf("release installation ownership: %w", err)
	}
	result.Released = true
	result.PathPresent = pathPresent
	if err := reportProgress(report, "complete", "Installation released", 100, "", 0); err != nil {
		return result, err
	}
	return result, result.Validate()
}

func recoveryResult(record InstallationOwnershipRecord, request devicev1.InstallationRecoveryRequest) devicev1.InstallationRecoveryResult {
	return devicev1.InstallationRecoveryResult{
		Action: request.Action, GameID: request.GameID, SourceGameID: request.SourceGameID,
		LocalInstallationID: record.LocalInstallationID,
	}
}

func requireSafeInstallationDirectory(record InstallationOwnershipRecord) error {
	inside, err := pathWithinRoot(record.InstallRoot, record.InstallPath)
	if err != nil || !inside || sameLocalPath(record.InstallRoot, record.InstallPath) {
		return errors.New("installation path is outside its recorded MGA root")
	}
	info, err := os.Lstat(record.InstallPath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("installation path is not a directory")
	}
	reparse, err := isFilesystemReparsePoint(record.InstallPath)
	if err != nil {
		return err
	}
	if reparse {
		return errors.New("installation path is an unsafe reparse point")
	}
	return nil
}

func validOwnedArchiveManifest(manifest installManifest, record InstallationOwnershipRecord, request devicev1.InstallationRecoveryRequest) bool {
	return (manifest.SchemaVersion == devicev1.InstallManifestSchemaVersion || manifest.SchemaVersion == devicev1.LegacyInstallManifestSchemaVersion) &&
		manifest.GameID == request.GameID && manifest.SourceGameID == request.SourceGameID &&
		sameLocalPath(manifest.InstallRoot, request.InstallRoot) &&
		strings.EqualFold(manifest.LocalInstallationID, record.LocalInstallationID) &&
		strings.EqualFold(manifest.OwnerBindingID, record.OwnerBindingID)
}

func validOwnedGogManifest(manifest gogInnoManifest, record InstallationOwnershipRecord, request devicev1.InstallationRecoveryRequest) bool {
	return (manifest.SchemaVersion == devicev1.ExecutableInstallManifestSchemaVersion || manifest.SchemaVersion == devicev1.PreviousExecutableInstallManifestSchemaVersion || manifest.SchemaVersion == devicev1.LegacyExecutableInstallManifestSchemaVersion) &&
		manifest.GameID == request.GameID && manifest.SourceGameID == request.SourceGameID &&
		manifest.InstallerFamily == devicev1.GogInnoInstallerFamily &&
		sameLocalPath(manifest.InstallRoot, request.InstallRoot) && sameLocalPath(manifest.InstallPath, request.InstallPath) &&
		strings.EqualFold(manifest.LocalInstallationID, record.LocalInstallationID) &&
		strings.EqualFold(manifest.OwnerBindingID, record.OwnerBindingID)
}
