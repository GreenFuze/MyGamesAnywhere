package clientapp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/singleinstance"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/google/uuid"
)

const (
	ownershipCatalogSchemaVersion       = 2
	ownershipCatalogLegacySchemaVersion = 1
)

type InstallationOwnershipState string

const (
	OwnershipInstalling      InstallationOwnershipState = "installing"
	OwnershipOwned           InstallationOwnershipState = "owned"
	OwnershipReleased        InstallationOwnershipState = "released"
	OwnershipLegacyUnclaimed InstallationOwnershipState = "legacy_unclaimed"
	OwnershipInterrupted     InstallationOwnershipState = "interrupted"
)

type InstallationOwnershipRecord struct {
	LocalInstallationID string                              `json:"local_installation_id"`
	OwnerBindingID      string                              `json:"owner_binding_id,omitempty"`
	State               InstallationOwnershipState          `json:"state"`
	InstallKind         string                              `json:"install_kind"`
	InstallRoot         string                              `json:"install_root"`
	InstallPath         string                              `json:"install_path"`
	ProductIdentity     string                              `json:"product_identity,omitempty"`
	GameID              string                              `json:"game_id,omitempty"`
	SourceGameID        string                              `json:"source_game_id,omitempty"`
	Title               string                              `json:"title,omitempty"`
	CreatedAt           time.Time                           `json:"created_at"`
	UpdatedAt           time.Time                           `json:"updated_at"`
	ReleasedAt          *time.Time                          `json:"released_at,omitempty"`
	PreviousOwners      []string                            `json:"previous_owner_binding_ids,omitempty"`
	NativeProducts      []devicev1.NativeProductObservation `json:"native_products,omitempty"`
	UseGrants           []InstallationUseGrant              `json:"use_grants,omitempty"`
}

type InstallationUseGrant struct {
	BindingID string    `json:"binding_id"`
	GrantedAt time.Time `json:"granted_at"`
}

func (r InstallationOwnershipRecord) validate() error {
	if _, err := uuid.Parse(r.LocalInstallationID); err != nil {
		return errors.New("local_installation_id must be a UUID")
	}
	switch r.State {
	case OwnershipInstalling, OwnershipOwned, OwnershipInterrupted:
		if _, err := uuid.Parse(r.OwnerBindingID); err != nil {
			return errors.New("owned installation requires an owner binding UUID")
		}
	case OwnershipReleased, OwnershipLegacyUnclaimed:
		if strings.TrimSpace(r.OwnerBindingID) != "" {
			return errors.New("unowned installation must not have owner_binding_id")
		}
	default:
		return fmt.Errorf("unsupported ownership state %q", r.State)
	}
	if strings.TrimSpace(r.InstallKind) == "" || !filepath.IsAbs(r.InstallRoot) || !filepath.IsAbs(r.InstallPath) {
		return errors.New("install kind and absolute root/path are required")
	}
	inside, err := pathWithinRoot(r.InstallRoot, r.InstallPath)
	if err != nil || !inside || sameLocalPath(r.InstallRoot, r.InstallPath) {
		return errors.New("install_path must be a non-root child of install_root")
	}
	if r.CreatedAt.IsZero() || r.UpdatedAt.IsZero() {
		return errors.New("ownership timestamps are required")
	}
	seenProducts := map[string]bool{}
	for index, product := range r.NativeProducts {
		if err := product.Validate(); err != nil {
			return fmt.Errorf("native product %d: %w", index, err)
		}
		key := product.Provider + ":" + strings.ToLower(product.ProductID)
		if seenProducts[key] {
			return fmt.Errorf("duplicate native product %q", product.ProductID)
		}
		seenProducts[key] = true
	}
	seenGrants := map[string]bool{}
	for index, grant := range r.UseGrants {
		if _, err := uuid.Parse(grant.BindingID); err != nil || grant.GrantedAt.IsZero() {
			return fmt.Errorf("use grant %d is invalid", index)
		}
		key := strings.ToLower(grant.BindingID)
		if seenGrants[key] || strings.EqualFold(grant.BindingID, r.OwnerBindingID) {
			return fmt.Errorf("use grant %d is duplicate or grants the owner", index)
		}
		seenGrants[key] = true
	}
	return nil
}

type ownershipCatalogDocument struct {
	SchemaVersion int                           `json:"schema_version"`
	Installations []InstallationOwnershipRecord `json:"installations"`
}

func (d ownershipCatalogDocument) validate() error {
	if d.SchemaVersion != ownershipCatalogSchemaVersion && d.SchemaVersion != ownershipCatalogLegacySchemaVersion {
		return fmt.Errorf("unsupported ownership catalog schema %d", d.SchemaVersion)
	}
	ids := make(map[string]struct{}, len(d.Installations))
	paths := make(map[string]struct{}, len(d.Installations))
	for index, record := range d.Installations {
		if d.SchemaVersion == ownershipCatalogLegacySchemaVersion && (len(record.NativeProducts) != 0 || len(record.UseGrants) != 0) {
			return fmt.Errorf("installation %d uses schema-2 fields in a schema-1 catalog", index)
		}
		if err := record.validate(); err != nil {
			return fmt.Errorf("installation %d: %w", index, err)
		}
		id := strings.ToLower(record.LocalInstallationID)
		if _, exists := ids[id]; exists {
			return fmt.Errorf("duplicate local installation ID %q", record.LocalInstallationID)
		}
		ids[id] = struct{}{}
		path := localPathKey(record.InstallPath)
		if _, exists := paths[path]; exists {
			return fmt.Errorf("duplicate installation path %q", record.InstallPath)
		}
		paths[path] = struct{}{}
	}
	return nil
}

// OwnershipCatalog is the per-OS-user authority for MGA-managed filesystem ownership.
type OwnershipCatalog struct {
	mu       sync.Mutex
	path     string
	doc      ownershipCatalogDocument
	lockName string
}

func OpenOwnershipCatalog(path string) (*OwnershipCatalog, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("ownership catalog path is required")
	}
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(path))))
	catalog := &OwnershipCatalog{path: path, lockName: "MGAOwnership-" + hex.EncodeToString(digest[:16]), doc: ownershipCatalogDocument{SchemaVersion: ownershipCatalogSchemaVersion, Installations: []InstallationOwnershipRecord{}}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return catalog, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ownership catalog: %w", err)
	}
	document, err := decodeOwnershipCatalog(data)
	if err != nil {
		return nil, fmt.Errorf("decode ownership catalog: %w", err)
	}
	migrated := document.SchemaVersion == ownershipCatalogLegacySchemaVersion
	if migrated {
		document.SchemaVersion = ownershipCatalogSchemaVersion
	}
	catalog.doc = document
	if migrated {
		if err := catalog.saveLocked(); err != nil {
			return nil, fmt.Errorf("migrate ownership catalog to schema %d: %w", ownershipCatalogSchemaVersion, err)
		}
	}
	return catalog, nil
}

func decodeOwnershipCatalog(data []byte) (ownershipCatalogDocument, error) {
	var document ownershipCatalogDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return ownershipCatalogDocument{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ownershipCatalogDocument{}, errors.New("ownership catalog contains trailing JSON")
	}
	if err := document.validate(); err != nil {
		return ownershipCatalogDocument{}, err
	}
	return document, nil
}

func (c *OwnershipCatalog) List() []InstallationOwnershipRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := append([]InstallationOwnershipRecord(nil), c.doc.Installations...)
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i].Title) < strings.ToLower(result[j].Title) })
	return result
}

func (c *OwnershipCatalog) FindByPath(path string) (InstallationOwnershipRecord, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.findByPathLocked(path)
}

func (c *OwnershipCatalog) FindByID(localID string) (InstallationOwnershipRecord, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, record := range c.doc.Installations {
		if strings.EqualFold(record.LocalInstallationID, strings.TrimSpace(localID)) {
			return record, true
		}
	}
	return InstallationOwnershipRecord{}, false
}

func (c *OwnershipCatalog) GrantUse(localID, bindingID string, grantedAt time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, err := c.prepareMutationLocked()
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := uuid.Parse(bindingID); err != nil || grantedAt.IsZero() {
		return errors.New("use grant requires a binding UUID and timestamp")
	}
	for index := range c.doc.Installations {
		record := &c.doc.Installations[index]
		if !strings.EqualFold(record.LocalInstallationID, localID) {
			continue
		}
		if record.State != OwnershipOwned && record.State != OwnershipReleased {
			return fmt.Errorf("installation is %s and cannot be shared", record.State)
		}
		if strings.EqualFold(record.OwnerBindingID, bindingID) {
			return errors.New("the owning server already has access to this installation")
		}
		for _, grant := range record.UseGrants {
			if strings.EqualFold(grant.BindingID, bindingID) {
				return nil
			}
		}
		record.UseGrants = append(record.UseGrants, InstallationUseGrant{BindingID: bindingID, GrantedAt: grantedAt.UTC()})
		record.UpdatedAt = time.Now().UTC()
		return c.saveLocked()
	}
	return errors.New("installation ownership record not found")
}

func (c *OwnershipCatalog) HasUseGrant(localID, bindingID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, record := range c.doc.Installations {
		if !strings.EqualFold(record.LocalInstallationID, localID) {
			continue
		}
		for _, grant := range record.UseGrants {
			if strings.EqualFold(grant.BindingID, bindingID) {
				return true
			}
		}
	}
	return false
}

func (c *OwnershipCatalog) ReplaceNativeProducts(localID string, products []devicev1.NativeProductObservation) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, err := c.prepareMutationLocked()
	if err != nil {
		return err
	}
	defer lock.Close()
	products = append([]devicev1.NativeProductObservation(nil), products...)
	sort.Slice(products, func(left, right int) bool { return products[left].ProductID < products[right].ProductID })
	for index := range c.doc.Installations {
		record := &c.doc.Installations[index]
		if !strings.EqualFold(record.LocalInstallationID, localID) {
			continue
		}
		current, _ := json.Marshal(record.NativeProducts)
		next, _ := json.Marshal(products)
		if bytes.Equal(current, next) {
			return nil
		}
		record.NativeProducts = products
		record.UpdatedAt = time.Now().UTC()
		return c.saveLocked()
	}
	return errors.New("installation ownership record not found")
}

func (c *OwnershipCatalog) BeginInstall(record InstallationOwnershipRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, err := c.prepareMutationLocked()
	if err != nil {
		return err
	}
	defer lock.Close()
	if record.State != OwnershipInstalling {
		return errors.New("new ownership record must be installing")
	}
	if _, found := c.findByPathLocked(record.InstallPath); found {
		return fmt.Errorf("installation_conflict: destination is already managed: %s", record.InstallPath)
	}
	for _, existing := range c.doc.Installations {
		if record.ProductIdentity != "" && strings.EqualFold(existing.ProductIdentity, record.ProductIdentity) {
			return fmt.Errorf("installation_conflict: product is already managed at %s", existing.InstallPath)
		}
	}
	if err := record.validate(); err != nil {
		return err
	}
	c.doc.Installations = append(c.doc.Installations, record)
	return c.saveLocked()
}

func (c *OwnershipCatalog) CompleteInstall(localID, bindingID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, err := c.prepareMutationLocked()
	if err != nil {
		return err
	}
	defer lock.Close()
	index, err := c.ownedIndexLocked(localID, bindingID, OwnershipInstalling)
	if err != nil {
		return err
	}
	c.doc.Installations[index].State = OwnershipOwned
	c.doc.Installations[index].UpdatedAt = time.Now().UTC()
	return c.saveLocked()
}

func (c *OwnershipCatalog) AbortInstall(localID, bindingID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, err := c.prepareMutationLocked()
	if err != nil {
		return err
	}
	defer lock.Close()
	index, err := c.ownedIndexLocked(localID, bindingID, OwnershipInstalling)
	if err != nil {
		return err
	}
	c.doc.Installations = append(c.doc.Installations[:index], c.doc.Installations[index+1:]...)
	return c.saveLocked()
}

func (c *OwnershipCatalog) RemoveOwned(localID, bindingID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, err := c.prepareMutationLocked()
	if err != nil {
		return err
	}
	defer lock.Close()
	index, err := c.ownedIndexLocked(localID, bindingID, OwnershipOwned)
	if err != nil {
		return err
	}
	c.doc.Installations = append(c.doc.Installations[:index], c.doc.Installations[index+1:]...)
	return c.saveLocked()
}

func (c *OwnershipCatalog) Release(localID, bindingID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, err := c.prepareMutationLocked()
	if err != nil {
		return err
	}
	defer lock.Close()
	index := -1
	for i, record := range c.doc.Installations {
		if !strings.EqualFold(record.LocalInstallationID, localID) {
			continue
		}
		if !strings.EqualFold(record.OwnerBindingID, bindingID) {
			return errors.New("installation is owned by another MGA server")
		}
		if record.State != OwnershipOwned {
			return fmt.Errorf("installation is %s, not releasable", record.State)
		}
		index = i
		break
	}
	if index < 0 {
		return errors.New("installation ownership record not found")
	}
	now := time.Now().UTC()
	record := &c.doc.Installations[index]
	record.PreviousOwners = appendUnique(record.PreviousOwners, record.OwnerBindingID)
	record.OwnerBindingID = ""
	record.State = OwnershipReleased
	record.ReleasedAt = &now
	record.UpdatedAt = now
	return c.saveLocked()
}

func (c *OwnershipCatalog) RecoverInterrupted() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, err := c.prepareMutationLocked()
	if err != nil {
		return err
	}
	defer lock.Close()
	changed := false
	for index := range c.doc.Installations {
		if c.doc.Installations[index].State == OwnershipInstalling {
			c.doc.Installations[index].State = OwnershipInterrupted
			c.doc.Installations[index].UpdatedAt = time.Now().UTC()
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return c.saveLocked()
}

func (c *OwnershipCatalog) Adopt(localID, bindingID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, err := c.prepareMutationLocked()
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := uuid.Parse(bindingID); err != nil {
		return errors.New("adopting binding ID must be a UUID")
	}
	index := -1
	for i := range c.doc.Installations {
		if strings.EqualFold(c.doc.Installations[i].LocalInstallationID, localID) {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("installation ownership record not found")
	}
	record := &c.doc.Installations[index]
	if record.State != OwnershipReleased {
		return fmt.Errorf("installation is %s, not released", record.State)
	}
	record.OwnerBindingID = bindingID
	record.State = OwnershipOwned
	record.ReleasedAt = nil
	record.UpdatedAt = time.Now().UTC()
	grants := record.UseGrants[:0]
	for _, grant := range record.UseGrants {
		if !strings.EqualFold(grant.BindingID, bindingID) {
			grants = append(grants, grant)
		}
	}
	record.UseGrants = grants
	return c.saveLocked()
}

func (c *OwnershipCatalog) prepareMutationLocked() (*singleinstance.Lock, error) {
	lock, err := singleinstance.Acquire(c.lockName)
	if err != nil {
		return nil, fmt.Errorf("installation ownership catalog is busy: %w", err)
	}
	data, err := os.ReadFile(c.path)
	if errors.Is(err, os.ErrNotExist) {
		c.doc = ownershipCatalogDocument{SchemaVersion: ownershipCatalogSchemaVersion, Installations: []InstallationOwnershipRecord{}}
		return lock, nil
	}
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("refresh ownership catalog: %w", err)
	}
	document, err := decodeOwnershipCatalog(data)
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("refresh ownership catalog: %w", err)
	}
	if document.SchemaVersion == ownershipCatalogLegacySchemaVersion {
		document.SchemaVersion = ownershipCatalogSchemaVersion
	}
	c.doc = document
	return lock, nil
}

func (c *OwnershipCatalog) findByPathLocked(path string) (InstallationOwnershipRecord, bool) {
	key := localPathKey(path)
	for _, record := range c.doc.Installations {
		if localPathKey(record.InstallPath) == key {
			return record, true
		}
	}
	return InstallationOwnershipRecord{}, false
}

func (c *OwnershipCatalog) ownedIndexLocked(localID, bindingID string, state InstallationOwnershipState) (int, error) {
	for index := range c.doc.Installations {
		record := c.doc.Installations[index]
		if !strings.EqualFold(record.LocalInstallationID, localID) {
			continue
		}
		if record.State != state {
			return -1, fmt.Errorf("installation is %s, expected %s", record.State, state)
		}
		if !strings.EqualFold(record.OwnerBindingID, bindingID) {
			return -1, errors.New("installation is owned by another MGA server")
		}
		return index, nil
	}
	return -1, errors.New("installation ownership record not found")
}

func (c *OwnershipCatalog) saveLocked() error {
	if err := c.doc.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c.doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	temporary := c.path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, c.path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace ownership catalog: %w", err)
	}
	return nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

func localPathKey(path string) string {
	key := filepath.Clean(strings.TrimSpace(path))
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

func sameLocalPath(left, right string) bool { return localPathKey(left) == localPathKey(right) }

// InstallationCoordinator serializes path and native-product mutations across all bound agents.
type InstallationCoordinator struct {
	mu     sync.Mutex
	active map[string]string
}

func NewInstallationCoordinator() *InstallationCoordinator {
	return &InstallationCoordinator{active: make(map[string]string)}
}

func (c *InstallationCoordinator) Reserve(bindingID, path, product string) (func(), error) {
	if c == nil {
		return nil, errors.New("installation coordinator is unavailable")
	}
	keys := []string{"path:" + localPathKey(path)}
	if strings.TrimSpace(product) != "" {
		keys = append(keys, "product:"+strings.ToLower(strings.TrimSpace(product)))
	}
	c.mu.Lock()
	for _, key := range keys {
		if owner, exists := c.active[key]; exists {
			c.mu.Unlock()
			return nil, fmt.Errorf("installation_conflict: another MGA server operation (%s) is using this installation", owner)
		}
	}
	for _, key := range keys {
		c.active[key] = bindingID
	}
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			for _, key := range keys {
				delete(c.active, key)
			}
		})
	}, nil
}
