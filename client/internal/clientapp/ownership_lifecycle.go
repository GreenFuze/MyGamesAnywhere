package clientapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/desktop"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func (s *Service) ConfirmAndReleaseInstallation(ctx context.Context, options ReleaseInstallationOptions) error {
	record, _, err := s.resolveOwnedInstallation(options.LocalInstallationID, options.ServerURL)
	if err != nil {
		return err
	}
	approved, err := desktop.ConfirmInstallationRelease(ctx, record.Title, record.InstallPath, options.ServerURL)
	if err != nil {
		return err
	}
	if !approved {
		return errors.New("installation release was canceled")
	}
	return s.ReleaseInstallation(options)
}

func (s *Service) ConfirmAndAdoptInstallation(ctx context.Context, options AdoptInstallationOptions) error {
	record, found := s.ownership.FindByID(strings.TrimSpace(options.LocalInstallationID))
	if !found {
		return errors.New("installation ownership record not found")
	}
	approved, err := desktop.ConfirmInstallationAdoption(ctx, record.Title, record.InstallPath, options.ServerURL)
	if err != nil {
		return err
	}
	if !approved {
		return errors.New("installation adoption was canceled")
	}
	return s.AdoptInstallation(options)
}

type ReleaseInstallationOptions struct {
	LocalInstallationID string
	ServerURL           string
}

type AdoptInstallationOptions struct {
	LocalInstallationID string
	ServerURL           string
}

func (s *Service) Installations() ([]InstallationOwnershipRecord, error) {
	if s == nil || s.ownership == nil {
		return nil, errors.New("installation ownership catalog is unavailable")
	}
	return s.ownership.List(), nil
}

func (s *Service) ReleaseInstallation(options ReleaseInstallationOptions) error {
	record, binding, err := s.resolveOwnedInstallation(options.LocalInstallationID, options.ServerURL)
	if err != nil {
		return err
	}
	if err := rewriteManifestOwnership(record, "", OwnershipReleased); err != nil {
		return err
	}
	if err := s.ownership.Release(record.LocalInstallationID, binding.BindingID); err != nil {
		_ = rewriteManifestOwnership(record, binding.BindingID, OwnershipOwned)
		return fmt.Errorf("release installation ownership: %w", err)
	}
	s.Logf("released installation %s from binding %s without deleting files", record.LocalInstallationID, binding.BindingID)
	return nil
}

func (s *Service) AdoptInstallation(options AdoptInstallationOptions) error {
	localID := strings.TrimSpace(options.LocalInstallationID)
	record, found := s.ownership.FindByID(localID)
	if !found {
		return errors.New("installation ownership record not found")
	}
	if record.State != OwnershipReleased {
		return fmt.Errorf("installation is %s, not released", record.State)
	}
	document, err := s.loadBindings()
	if err != nil {
		return err
	}
	targetURL, err := validateServerURL(options.ServerURL)
	if err != nil {
		return err
	}
	binding, found := findBinding(document.Bindings, targetURL)
	if !found {
		return fmt.Errorf("MGA Client is not paired with %s", targetURL)
	}
	if err := s.ownership.Adopt(localID, binding.BindingID); err != nil {
		return err
	}
	if err := rewriteManifestOwnership(record, binding.BindingID, OwnershipOwned); err != nil {
		_ = s.ownership.Release(localID, binding.BindingID)
		return fmt.Errorf("update installation manifest after adoption: %w", err)
	}
	s.Logf("adopted installation %s into binding %s", localID, binding.BindingID)
	return nil
}

func (s *Service) releaseAllOwnedByServer(serverURL string) error {
	document, err := s.loadBindings()
	if err != nil {
		return err
	}
	normalized, err := validateServerURL(serverURL)
	if err != nil {
		return err
	}
	binding, found := findBinding(document.Bindings, normalized)
	if !found {
		return fmt.Errorf("MGA Client is not paired with %s", normalized)
	}
	for _, record := range s.ownership.List() {
		if record.State == OwnershipOwned && strings.EqualFold(record.OwnerBindingID, binding.BindingID) {
			if err := s.ReleaseInstallation(ReleaseInstallationOptions{LocalInstallationID: record.LocalInstallationID, ServerURL: binding.ServerURL}); err != nil {
				return fmt.Errorf("release %s: %w", record.Title, err)
			}
		}
	}
	return nil
}

func (s *Service) resolveOwnedInstallation(localID, serverURL string) (InstallationOwnershipRecord, clientBinding, error) {
	record, found := s.ownership.FindByID(strings.TrimSpace(localID))
	if !found {
		return InstallationOwnershipRecord{}, clientBinding{}, errors.New("installation ownership record not found")
	}
	if record.State != OwnershipOwned {
		return InstallationOwnershipRecord{}, clientBinding{}, fmt.Errorf("installation is %s, not owned", record.State)
	}
	document, err := s.loadBindings()
	if err != nil {
		return InstallationOwnershipRecord{}, clientBinding{}, err
	}
	requested := strings.TrimSpace(serverURL)
	for _, binding := range document.Bindings {
		if !strings.EqualFold(binding.BindingID, record.OwnerBindingID) {
			continue
		}
		if requested != "" {
			normalized, normalizeErr := validateServerURL(requested)
			if normalizeErr != nil {
				return InstallationOwnershipRecord{}, clientBinding{}, normalizeErr
			}
			if !samePairedServerURL(normalized, binding.ServerURL) {
				return InstallationOwnershipRecord{}, clientBinding{}, errors.New("installation is owned by a different MGA server")
			}
		}
		return record, clientBinding{BindingID: binding.BindingID, ServerURL: binding.ServerURL}, nil
	}
	if requested == "" {
		return record, clientBinding{BindingID: record.OwnerBindingID}, nil
	}
	return InstallationOwnershipRecord{}, clientBinding{}, errors.New("owning MGA server is disconnected")
}

type clientBinding struct{ BindingID, ServerURL string }

func rewriteManifestOwnership(record InstallationOwnershipRecord, ownerBindingID string, state InstallationOwnershipState) error {
	switch record.InstallKind {
	case "managed_archive":
		manifest, err := readInstallManifest(record.InstallPath)
		if err != nil {
			return err
		}
		if !strings.EqualFold(manifest.LocalInstallationID, record.LocalInstallationID) {
			return errors.New("archive manifest local installation ID does not match the catalog")
		}
		manifest.OwnerBindingID = ownerBindingID
		manifest.OwnershipState = string(state)
		return writeJSONAtomic(filepath.Join(record.InstallPath, installManifestName), manifest)
	case "gog_inno":
		manifest, err := readGogInnoManifest(record.InstallPath)
		if err != nil {
			return err
		}
		if !strings.EqualFold(manifest.LocalInstallationID, record.LocalInstallationID) {
			return errors.New("native manifest local installation ID does not match the catalog")
		}
		manifest.OwnerBindingID = ownerBindingID
		manifest.OwnershipState = string(state)
		return writeJSONAtomic(filepath.Join(record.InstallPath, installManifestName), manifest)
	default:
		return fmt.Errorf("installation kind %q does not support ownership transfer", record.InstallKind)
	}
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".ownership.tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func ensureInstallationManifestOwnership(owner *InstallationOwnership, installPath string, manifest installManifest) (installManifest, error) {
	if owner == nil || (manifest.LocalInstallationID != "" && manifest.OwnerBindingID != "") {
		return manifest, nil
	}
	if owner.bindingCount != 1 {
		return installManifest{}, errors.New("legacy installation ownership is ambiguous because this client has multiple MGA server bindings; release or adopt it locally")
	}
	kind, productIdentity := devicev1.InstallKindManagedArchive, ""
	if manifest.InstallerFamily != "" {
		kind = manifest.InstallerFamily
		if manifest.PrimarySHA256 != "" {
			productIdentity = manifest.InstallerFamily + ":sha256:" + strings.ToLower(manifest.PrimarySHA256)
		}
	}
	if existing, found := owner.catalog.FindByPath(installPath); found {
		if existing.State != OwnershipOwned || !strings.EqualFold(existing.OwnerBindingID, owner.bindingID) {
			return installManifest{}, errors.New("legacy installation has conflicting local ownership")
		}
		manifest.LocalInstallationID, manifest.OwnerBindingID, manifest.OwnershipState = existing.LocalInstallationID, existing.OwnerBindingID, string(OwnershipOwned)
		return manifest, nil
	}
	operation, err := owner.BeginInstall(kind, manifest.GameID, manifest.SourceGameID, filepath.Base(installPath), manifest.InstallRoot, installPath, productIdentity)
	if err != nil {
		return installManifest{}, err
	}
	manifest.LocalInstallationID, manifest.OwnerBindingID, manifest.OwnershipState = operation.LocalInstallationID(), operation.OwnerBindingID(), string(OwnershipOwned)
	if kind == devicev1.InstallKindGogInno {
		full, readErr := readGogInnoManifest(installPath)
		if readErr != nil {
			_ = operation.Abort()
			return installManifest{}, readErr
		}
		full.SchemaVersion = devicev1.ExecutableInstallManifestSchemaVersion
		full.LocalInstallationID, full.OwnerBindingID, full.OwnershipState = manifest.LocalInstallationID, manifest.OwnerBindingID, manifest.OwnershipState
		if err := writeJSONAtomic(filepath.Join(installPath, installManifestName), full); err != nil {
			_ = operation.Abort()
			return installManifest{}, err
		}
	} else {
		manifest.SchemaVersion = devicev1.InstallManifestSchemaVersion
		if err := writeJSONAtomic(filepath.Join(installPath, installManifestName), manifest); err != nil {
			_ = operation.Abort()
			return installManifest{}, err
		}
	}
	if err := operation.Complete(); err != nil {
		operation.LeavePending()
		return installManifest{}, fmt.Errorf("claim legacy installation ownership: %w", err)
	}
	return manifest, nil
}
