package clientapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const saveDomainCatalogSchemaVersion uint16 = 1

const (
	SaveDomainObserved               = "observed"
	SaveDomainOwned                  = "owned"
	SaveDomainReleased               = "released"
	SaveDomainReconciliationRequired = "reconciliation_required"
)

// LocalSaveDomainRecord is client-local authority evidence. ResolvedPaths is
// deliberately persisted only on the client and must never be placed in
// device inventory or a server command result.
type LocalSaveDomainRecord struct {
	LocalSaveDomainID       string    `json:"local_save_domain_id"`
	AdapterID               string    `json:"adapter_id"`
	RouteFingerprint        string    `json:"route_fingerprint"`
	EvidenceFingerprint     string    `json:"evidence_fingerprint"`
	ResolvedPaths           []string  `json:"resolved_paths"`
	WriterBindingID         string    `json:"writer_binding_id,omitempty"`
	LastWriterBindingID     string    `json:"last_writer_binding_id,omitempty"`
	PendingWriterBindingID  string    `json:"pending_writer_binding_id,omitempty"`
	ScummVMGameID           string    `json:"scummvm_game_id,omitempty"`
	State                   string    `json:"state"`
	LastSnapshotFingerprint string    `json:"last_snapshot_fingerprint,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

func (c *SaveDomainCatalog) SetScummVMGameID(localID, gameID string) error {
	gameID = strings.ToLower(strings.TrimSpace(gameID))
	if !scummVMGameIDPattern.MatchString(gameID) {
		return errors.New("ScummVM game ID must include its engine")
	}
	return c.change(localID, func(domain *LocalSaveDomainRecord) error {
		if domain.AdapterID != "scummvm" || domain.State == SaveDomainOwned {
			return errors.New("ScummVM route identity can only be set before authority is granted")
		}
		domain.ScummVMGameID = gameID
		return nil
	})
}

func (r LocalSaveDomainRecord) validate() error {
	if strings.TrimSpace(r.LocalSaveDomainID) == "" || strings.TrimSpace(r.AdapterID) == "" || strings.TrimSpace(r.RouteFingerprint) == "" || strings.TrimSpace(r.EvidenceFingerprint) == "" {
		return errors.New("save domain ID, adapter, route fingerprint, and evidence fingerprint are required")
	}
	if len(r.ResolvedPaths) == 0 || len(r.ResolvedPaths) > 32 {
		return errors.New("save domain must contain between 1 and 32 resolved paths")
	}
	seen := map[string]bool{}
	for _, path := range r.ResolvedPaths {
		absolute, err := filepath.Abs(strings.TrimSpace(path))
		if err != nil || absolute != filepath.Clean(path) {
			return fmt.Errorf("save domain contains invalid absolute path %q", path)
		}
		key := strings.ToLower(absolute)
		if seen[key] {
			return fmt.Errorf("save domain contains duplicate path %q", path)
		}
		seen[key] = true
	}
	switch r.State {
	case SaveDomainObserved, SaveDomainOwned, SaveDomainReleased, SaveDomainReconciliationRequired:
	default:
		return fmt.Errorf("unsupported save domain state %q", r.State)
	}
	if r.State == SaveDomainOwned && strings.TrimSpace(r.WriterBindingID) == "" {
		return errors.New("owned save domain requires a writer binding")
	}
	if r.State != SaveDomainOwned && r.WriterBindingID != "" {
		return errors.New("only an owned save domain may name a writer binding")
	}
	if r.State != SaveDomainReconciliationRequired && r.PendingWriterBindingID != "" {
		return errors.New("only reconciliation-required save domain may name a pending writer")
	}
	if r.ScummVMGameID != "" && (r.AdapterID != "scummvm" || !scummVMGameIDPattern.MatchString(r.ScummVMGameID)) {
		return errors.New("save domain contains an invalid ScummVM game ID")
	}
	if r.AdapterID == "scummvm" && r.State == SaveDomainOwned && r.ScummVMGameID == "" {
		return errors.New("owned ScummVM save domain requires an exact engine-qualified game ID")
	}
	if r.CreatedAt.IsZero() || r.UpdatedAt.IsZero() || r.UpdatedAt.Before(r.CreatedAt) {
		return errors.New("save domain timestamps are invalid")
	}
	return nil
}

type saveDomainCatalogDocument struct {
	SchemaVersion uint16                  `json:"schema_version"`
	Domains       []LocalSaveDomainRecord `json:"domains"`
}

func (d saveDomainCatalogDocument) validate() error {
	if d.SchemaVersion != saveDomainCatalogSchemaVersion {
		return fmt.Errorf("unsupported save domain catalog schema %d", d.SchemaVersion)
	}
	if len(d.Domains) > 1024 {
		return errors.New("save domain catalog contains too many records")
	}
	ids := map[string]bool{}
	routes := map[string]bool{}
	for _, domain := range d.Domains {
		if err := domain.validate(); err != nil {
			return err
		}
		if ids[domain.LocalSaveDomainID] || routes[domain.AdapterID+"\x00"+domain.RouteFingerprint] {
			return errors.New("save domain catalog contains duplicate identity")
		}
		ids[domain.LocalSaveDomainID] = true
		routes[domain.AdapterID+"\x00"+domain.RouteFingerprint] = true
	}
	return nil
}

// SaveDomainCatalog is the per-OS-user authority for local save writers.
type SaveDomainCatalog struct {
	mu   sync.RWMutex
	path string
	doc  saveDomainCatalogDocument
	now  func() time.Time
}

func OpenSaveDomainCatalog(path string) (*SaveDomainCatalog, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("save domain catalog path is required")
	}
	catalog := &SaveDomainCatalog{path: path, doc: saveDomainCatalogDocument{SchemaVersion: saveDomainCatalogSchemaVersion, Domains: []LocalSaveDomainRecord{}}, now: time.Now}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return catalog, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read save domain catalog: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document saveDomainCatalogDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode save domain catalog: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("save domain catalog contains trailing JSON")
	}
	if err := document.validate(); err != nil {
		return nil, err
	}
	catalog.doc = document
	return catalog, nil
}

func (c *SaveDomainCatalog) List() []LocalSaveDomainRecord {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneSaveDomains(c.doc.Domains)
}

func (c *SaveDomainCatalog) FindByID(localID string) (LocalSaveDomainRecord, bool) {
	if c == nil {
		return LocalSaveDomainRecord{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, domain := range c.doc.Domains {
		if domain.LocalSaveDomainID == strings.TrimSpace(localID) {
			return cloneSaveDomain(domain), true
		}
	}
	return LocalSaveDomainRecord{}, false
}

func (c *SaveDomainCatalog) FindByRoute(adapterID, routeFingerprint string) (LocalSaveDomainRecord, bool) {
	if c == nil {
		return LocalSaveDomainRecord{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, domain := range c.doc.Domains {
		if domain.AdapterID == strings.TrimSpace(adapterID) && domain.RouteFingerprint == strings.TrimSpace(routeFingerprint) {
			return cloneSaveDomain(domain), true
		}
	}
	return LocalSaveDomainRecord{}, false
}

func (c *SaveDomainCatalog) Resolve(adapterID, routeFingerprint, evidenceFingerprint string, paths []string) (LocalSaveDomainRecord, error) {
	if c == nil || c.now == nil {
		return LocalSaveDomainRecord{}, errors.New("save domain catalog is unavailable")
	}
	adapterID = strings.TrimSpace(adapterID)
	routeFingerprint = strings.TrimSpace(routeFingerprint)
	evidenceFingerprint = strings.TrimSpace(evidenceFingerprint)
	normalizedPaths, err := normalizeSaveDomainPaths(paths)
	if err != nil {
		return LocalSaveDomainRecord{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for index := range c.doc.Domains {
		domain := &c.doc.Domains[index]
		if domain.AdapterID != adapterID || domain.RouteFingerprint != routeFingerprint {
			continue
		}
		if domain.EvidenceFingerprint != evidenceFingerprint || !equalFoldedPaths(domain.ResolvedPaths, normalizedPaths) {
			before := cloneSaveDomain(*domain)
			if domain.WriterBindingID != "" {
				domain.LastWriterBindingID = domain.WriterBindingID
				domain.PendingWriterBindingID = domain.WriterBindingID
			}
			domain.EvidenceFingerprint = evidenceFingerprint
			domain.ResolvedPaths = normalizedPaths
			domain.WriterBindingID = ""
			domain.State = SaveDomainReconciliationRequired
			domain.UpdatedAt = c.now().UTC()
			if err := c.persistLocked(); err != nil {
				*domain = before
				return LocalSaveDomainRecord{}, err
			}
		}
		return cloneSaveDomain(*domain), nil
	}
	now := c.now().UTC()
	domain := LocalSaveDomainRecord{LocalSaveDomainID: uuid.NewString(), AdapterID: adapterID, RouteFingerprint: routeFingerprint, EvidenceFingerprint: evidenceFingerprint, ResolvedPaths: normalizedPaths, State: SaveDomainObserved, CreatedAt: now, UpdatedAt: now}
	if err := domain.validate(); err != nil {
		return LocalSaveDomainRecord{}, err
	}
	c.doc.Domains = append(c.doc.Domains, domain)
	if err := c.persistLocked(); err != nil {
		c.doc.Domains = c.doc.Domains[:len(c.doc.Domains)-1]
		return LocalSaveDomainRecord{}, err
	}
	return cloneSaveDomain(domain), nil
}

func (c *SaveDomainCatalog) RecordSnapshot(localID, bindingID, fingerprint string) error {
	fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))
	if err := validateSnapshotFingerprint(fingerprint); err != nil {
		return err
	}
	return c.change(localID, func(domain *LocalSaveDomainRecord) error {
		if domain.State != SaveDomainOwned || !strings.EqualFold(domain.WriterBindingID, strings.TrimSpace(bindingID)) {
			return errors.New("only the current writer can record a save snapshot")
		}
		domain.LastSnapshotFingerprint = fingerprint
		return nil
	})
}

func (c *SaveDomainCatalog) Claim(localID, bindingID string) error {
	return c.change(localID, func(domain *LocalSaveDomainRecord) error {
		bindingID = strings.TrimSpace(bindingID)
		if bindingID == "" {
			return errors.New("writer binding is required")
		}
		if domain.State == SaveDomainOwned && !strings.EqualFold(domain.WriterBindingID, bindingID) {
			return errors.New("save domain is already managed by another MGA server")
		}
		if domain.State == SaveDomainReconciliationRequired {
			return errors.New("save domain requires reconciliation before it can be claimed")
		}
		if domain.State == SaveDomainReleased && domain.LastWriterBindingID != "" && !strings.EqualFold(domain.LastWriterBindingID, bindingID) {
			domain.State = SaveDomainReconciliationRequired
			domain.PendingWriterBindingID = bindingID
			return nil
		}
		domain.State = SaveDomainOwned
		domain.WriterBindingID = bindingID
		domain.LastWriterBindingID = bindingID
		domain.PendingWriterBindingID = ""
		return nil
	})
}

func (c *SaveDomainCatalog) Release(localID, bindingID string) error {
	return c.change(localID, func(domain *LocalSaveDomainRecord) error {
		if domain.State != SaveDomainOwned || !strings.EqualFold(domain.WriterBindingID, strings.TrimSpace(bindingID)) {
			return errors.New("only the current writer can release this save domain")
		}
		domain.State = SaveDomainReleased
		domain.LastWriterBindingID = domain.WriterBindingID
		domain.WriterBindingID = ""
		return nil
	})
}

func (c *SaveDomainCatalog) CompleteReconciliation(localID, bindingID, fingerprint string) error {
	fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))
	if err := validateSnapshotFingerprint(fingerprint); err != nil {
		return err
	}
	return c.change(localID, func(domain *LocalSaveDomainRecord) error {
		bindingID = strings.TrimSpace(bindingID)
		if domain.State != SaveDomainReconciliationRequired || !strings.EqualFold(domain.PendingWriterBindingID, bindingID) {
			return errors.New("this binding does not own the pending save reconciliation")
		}
		domain.State = SaveDomainOwned
		domain.WriterBindingID = bindingID
		domain.LastWriterBindingID = bindingID
		domain.PendingWriterBindingID = ""
		domain.LastSnapshotFingerprint = fingerprint
		return nil
	})
}

func validateSnapshotFingerprint(fingerprint string) error {
	if len(fingerprint) != 64 {
		return errors.New("snapshot fingerprint must contain 64 hexadecimal characters")
	}
	for _, character := range fingerprint {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return errors.New("snapshot fingerprint must contain 64 hexadecimal characters")
		}
	}
	return nil
}

func (c *SaveDomainCatalog) change(localID string, mutate func(*LocalSaveDomainRecord) error) error {
	if c == nil || c.now == nil || mutate == nil {
		return errors.New("save domain catalog is unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for index := range c.doc.Domains {
		if c.doc.Domains[index].LocalSaveDomainID != strings.TrimSpace(localID) {
			continue
		}
		before := cloneSaveDomain(c.doc.Domains[index])
		if err := mutate(&c.doc.Domains[index]); err != nil {
			return err
		}
		c.doc.Domains[index].UpdatedAt = c.now().UTC()
		if err := c.doc.Domains[index].validate(); err != nil {
			c.doc.Domains[index] = before
			return err
		}
		if err := c.persistLocked(); err != nil {
			c.doc.Domains[index] = before
			return err
		}
		return nil
	}
	return errors.New("save domain record not found")
}

func (c *SaveDomainCatalog) persistLocked() error {
	if err := c.doc.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c.doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	temporary := c.path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, c.path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace save domain catalog: %w", err)
	}
	return nil
}

func normalizeSaveDomainPaths(paths []string) ([]string, error) {
	result := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, value := range paths {
		absolute, err := filepath.Abs(strings.TrimSpace(value))
		if err != nil || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("resolve save domain path %q", value)
		}
		absolute = filepath.Clean(absolute)
		key := strings.ToLower(absolute)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, absolute)
	}
	if len(result) == 0 || len(result) > 32 {
		return nil, errors.New("save domain requires between 1 and 32 unique paths")
	}
	return result, nil
}

func cloneSaveDomains(domains []LocalSaveDomainRecord) []LocalSaveDomainRecord {
	result := make([]LocalSaveDomainRecord, len(domains))
	for index := range domains {
		result[index] = cloneSaveDomain(domains[index])
	}
	return result
}

func cloneSaveDomain(domain LocalSaveDomainRecord) LocalSaveDomainRecord {
	domain.ResolvedPaths = append([]string(nil), domain.ResolvedPaths...)
	return domain
}

func equalFoldedPaths(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !strings.EqualFold(left[index], right[index]) {
			return false
		}
	}
	return true
}
