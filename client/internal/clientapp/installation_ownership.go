package clientapp

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var unsafeRootLabel = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// InstallationOwnership binds an installer instance to the local identity of
// the server agent that created it. The server cannot provide this value.
type InstallationOwnership struct {
	bindingID    string
	rootLabel    string
	bindingCount int
	catalog      *OwnershipCatalog
	coordinator  *InstallationCoordinator
}

func NewInstallationOwnership(bindingID, serverURL string, bindingCount int, catalog *OwnershipCatalog, coordinator *InstallationCoordinator) (*InstallationOwnership, error) {
	if _, err := uuid.Parse(bindingID); err != nil {
		return nil, errors.New("binding ID must be a UUID")
	}
	if bindingCount < 1 {
		return nil, errors.New("binding count must be positive")
	}
	if catalog == nil || coordinator == nil {
		return nil, errors.New("ownership catalog and operation coordinator are required")
	}
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || parsed.Hostname() == "" {
		return nil, errors.New("server URL is required")
	}
	label := unsafeRootLabel.ReplaceAllString(strings.ToLower(parsed.Hostname()), "-")
	label = strings.Trim(label, "-._")
	if label == "" {
		label = "server"
	}
	short := strings.ReplaceAll(strings.ToLower(bindingID), "-", "")
	if len(short) > 8 {
		short = short[:8]
	}
	return &InstallationOwnership{bindingID: bindingID, rootLabel: label + "-" + short, bindingCount: bindingCount, catalog: catalog, coordinator: coordinator}, nil
}

func (o *InstallationOwnership) NamespacedRoot(base string) string {
	if o == nil {
		return base
	}
	return filepath.Join(base, "MGA", o.rootLabel)
}

type OwnedInstallOperation struct {
	owner   *InstallationOwnership
	record  InstallationOwnershipRecord
	release func()
	done    bool
}

func (o *InstallationOwnership) BeginInstall(kind, gameID, sourceGameID, title, root, path, product string) (*OwnedInstallOperation, error) {
	if o == nil {
		return nil, errors.New("installation ownership is unavailable")
	}
	release, err := o.coordinator.Reserve(o.bindingID, path, product)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	record := InstallationOwnershipRecord{LocalInstallationID: uuid.NewString(), OwnerBindingID: o.bindingID, State: OwnershipInstalling, InstallKind: kind, InstallRoot: root, InstallPath: path, ProductIdentity: product, GameID: gameID, SourceGameID: sourceGameID, Title: title, CreatedAt: now, UpdatedAt: now}
	if err := o.catalog.BeginInstall(record); err != nil {
		release()
		return nil, err
	}
	return &OwnedInstallOperation{owner: o, record: record, release: release}, nil
}

func (op *OwnedInstallOperation) LocalInstallationID() string {
	if op == nil {
		return ""
	}
	return op.record.LocalInstallationID
}
func (op *OwnedInstallOperation) OwnerBindingID() string {
	if op == nil {
		return ""
	}
	return op.record.OwnerBindingID
}

func (op *OwnedInstallOperation) Complete() error {
	if op == nil || op.done {
		return errors.New("installation operation is already closed")
	}
	if err := op.owner.catalog.CompleteInstall(op.record.LocalInstallationID, op.record.OwnerBindingID); err != nil {
		return err
	}
	op.done = true
	op.release()
	return nil
}

func (op *OwnedInstallOperation) Abort() error {
	if op == nil || op.done {
		return nil
	}
	err := op.owner.catalog.AbortInstall(op.record.LocalInstallationID, op.record.OwnerBindingID)
	op.done = true
	op.release()
	return err
}

// LeavePending releases the in-process lease while retaining the durable
// installing record. It is used when filesystem commit succeeded but catalog
// finalization failed, so a restart cannot broaden mutation authority.
func (op *OwnedInstallOperation) LeavePending() {
	if op != nil && !op.done {
		op.done = true
		op.release()
	}
}

type OwnedMutation struct {
	owner   *InstallationOwnership
	record  InstallationOwnershipRecord
	release func()
	done    bool
}

func (o *InstallationOwnership) AuthorizeMutation(localID, manifestBindingID, path string) (*OwnedMutation, error) {
	if o == nil {
		return nil, nil
	}
	if strings.TrimSpace(localID) == "" || strings.TrimSpace(manifestBindingID) == "" {
		return nil, errors.New("legacy installation ownership is ambiguous; release or adopt it locally before changing it")
	}
	if !strings.EqualFold(manifestBindingID, o.bindingID) {
		return nil, errors.New("installation is managed by another MGA server")
	}
	record, found := o.catalog.FindByPath(path)
	if !found {
		return nil, errors.New("installation ownership record is missing; repair ownership locally before changing it")
	}
	if !strings.EqualFold(record.LocalInstallationID, localID) || !strings.EqualFold(record.OwnerBindingID, o.bindingID) || record.State != OwnershipOwned {
		return nil, errors.New("installation ownership does not match this MGA server")
	}
	release, err := o.coordinator.Reserve(o.bindingID, path, record.ProductIdentity)
	if err != nil {
		return nil, err
	}
	return &OwnedMutation{owner: o, record: record, release: release}, nil
}

func (m *OwnedMutation) Removed() error {
	if m == nil {
		return nil
	}
	if m.done {
		return errors.New("installation mutation is already closed")
	}
	if err := m.owner.catalog.RemoveOwned(m.record.LocalInstallationID, m.record.OwnerBindingID); err != nil {
		return err
	}
	m.done = true
	m.release()
	return nil
}

func (m *OwnedMutation) Close() {
	if m != nil && !m.done {
		m.done = true
		m.release()
	}
}

func (o *InstallationOwnership) Describe() string {
	if o == nil {
		return "legacy"
	}
	return fmt.Sprintf("%s (%s)", o.rootLabel, o.bindingID)
}
