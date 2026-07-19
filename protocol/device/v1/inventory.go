package v1

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	InventorySchemaVersion                   uint16 = 6
	InventorySchemaVersionWithNativeProducts uint16 = 5
	InventorySchemaVersionWithInstallations  uint16 = 4
	InventorySchemaVersionWithSaveAdapters   uint16 = 3
	InventorySchemaVersionPrevious           uint16 = 2
	InventorySchemaVersionLegacy             uint16 = 1
	maxInventoryRuntimes                            = 64
	maxRuntimeComponents                            = 256
	maxSaveAdapters                                 = 16
	maxManagedInstallations                         = 256
	maxSaveDomains                                  = 1024
)

// DeviceInventory is a bounded snapshot of facts needed to evaluate play and
// installation options. It intentionally excludes full environment dumps and
// arbitrary installed-software lists.
type DeviceInventory struct {
	SchemaVersion        uint16                           `json:"schema_version"`
	CapturedAt           time.Time                        `json:"captured_at"`
	Storage              []StorageInventory               `json:"storage"`
	Runtimes             []RuntimeInventory               `json:"runtimes"`
	PackageManagers      []PackageManagerInventory        `json:"package_managers,omitempty"`
	SaveAdapters         []SaveAdapterInventory           `json:"save_adapters,omitempty"`
	ManagedInstallations []ManagedInstallationObservation `json:"managed_installations,omitempty"`
	SaveDomains          []SaveDomainObservation          `json:"save_domains,omitempty"`
}

type StorageInventory struct {
	ID         string `json:"id"`
	Root       string `json:"root"`
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
}

type RuntimeInventory struct {
	ID                 string                      `json:"id"`
	Name               string                      `json:"name"`
	Version            string                      `json:"version,omitempty"`
	Path               string                      `json:"path,omitempty"`
	CoreProbeState     string                      `json:"core_probe_state,omitempty"`
	FirmwareProbeState string                      `json:"firmware_probe_state,omitempty"`
	Components         []RuntimeComponentInventory `json:"components,omitempty"`
}

type PackageManagerInventory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RuntimeComponentInventory struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type SaveAdapterInventory struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	ProbeState     string   `json:"probe_state"`
	SaveKinds      []string `json:"save_kinds"`
	RouteOverrides bool     `json:"route_overrides,omitempty"`
}

type ManagedInstallationObservation struct {
	LocalInstallationID string                     `json:"local_installation_id"`
	State               string                     `json:"state"`
	InstallKind         string                     `json:"install_kind"`
	Title               string                     `json:"title"`
	InstallPath         string                     `json:"install_path,omitempty"`
	CanManage           bool                       `json:"can_manage,omitempty"`
	CanAdopt            bool                       `json:"can_adopt,omitempty"`
	UseGranted          bool                       `json:"use_granted,omitempty"`
	NativeProducts      []NativeProductObservation `json:"native_products,omitempty"`
}

// NativeProductObservation is bounded evidence for a product already known to
// MGA. It intentionally omits registry keys, uninstall commands, and unrelated
// installed applications.
type NativeProductObservation struct {
	Provider     string   `json:"provider"`
	ProductID    string   `json:"product_id"`
	DisplayName  string   `json:"display_name"`
	Version      string   `json:"version,omitempty"`
	Publisher    string   `json:"publisher,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// SaveDomainObservation is a sanitized per-binding view of a client-local save
// domain. It deliberately omits local paths, filenames, and the writer's
// binding/server identity.
type SaveDomainObservation struct {
	LocalSaveDomainID string `json:"local_save_domain_id"`
	AdapterID         string `json:"adapter_id"`
	State             string `json:"state"`
	CanWrite          bool   `json:"can_write,omitempty"`
	CanClaim          bool   `json:"can_claim,omitempty"`
}

func (i DeviceInventory) Validate() error {
	if i.SchemaVersion != InventorySchemaVersionLegacy && i.SchemaVersion != InventorySchemaVersionPrevious && i.SchemaVersion != InventorySchemaVersionWithSaveAdapters && i.SchemaVersion != InventorySchemaVersionWithInstallations && i.SchemaVersion != InventorySchemaVersionWithNativeProducts && i.SchemaVersion != InventorySchemaVersion {
		return fmt.Errorf("unsupported inventory schema version %d", i.SchemaVersion)
	}
	if i.CapturedAt.IsZero() {
		return errors.New("inventory captured_at is required")
	}
	seenStorage := map[string]bool{}
	for _, storage := range i.Storage {
		if err := storage.Validate(); err != nil {
			return err
		}
		if seenStorage[storage.ID] {
			return fmt.Errorf("duplicate storage id %q", storage.ID)
		}
		seenStorage[storage.ID] = true
	}
	seenRuntimes := map[string]bool{}
	if len(i.Runtimes) > maxInventoryRuntimes {
		return errors.New("inventory contains too many runtimes")
	}
	for _, runtime := range i.Runtimes {
		if err := runtime.Validate(); err != nil {
			return err
		}
		if i.SchemaVersion == InventorySchemaVersionLegacy && (runtime.CoreProbeState != "" || runtime.FirmwareProbeState != "" || len(runtime.Components) != 0) {
			return fmt.Errorf("inventory schema 1 runtime %s cannot contain component facts", runtime.ID)
		}
		if seenRuntimes[runtime.ID] {
			return fmt.Errorf("duplicate runtime id %q", runtime.ID)
		}
		seenRuntimes[runtime.ID] = true
	}
	seenManagers := map[string]bool{}
	for _, manager := range i.PackageManagers {
		if err := manager.Validate(); err != nil {
			return err
		}
		if seenManagers[manager.ID] {
			return fmt.Errorf("duplicate package manager id %q", manager.ID)
		}
		seenManagers[manager.ID] = true
	}
	if i.SchemaVersion == InventorySchemaVersionLegacy && len(i.PackageManagers) != 0 {
		return errors.New("inventory schema 1 cannot contain package managers")
	}
	if i.SchemaVersion < 3 && len(i.SaveAdapters) != 0 {
		return fmt.Errorf("inventory schema %d cannot contain save adapters", i.SchemaVersion)
	}
	if len(i.SaveAdapters) > maxSaveAdapters {
		return errors.New("inventory contains too many save adapters")
	}
	seenAdapters := map[string]bool{}
	for _, adapter := range i.SaveAdapters {
		if err := adapter.Validate(); err != nil {
			return err
		}
		if seenAdapters[adapter.ID] {
			return fmt.Errorf("duplicate save adapter id %q", adapter.ID)
		}
		seenAdapters[adapter.ID] = true
	}
	if i.SchemaVersion < InventorySchemaVersionWithInstallations && len(i.ManagedInstallations) != 0 {
		return fmt.Errorf("inventory schema %d cannot contain managed installations", i.SchemaVersion)
	}
	if len(i.ManagedInstallations) > maxManagedInstallations {
		return errors.New("inventory contains too many managed installations")
	}
	seenInstallations := map[string]bool{}
	for _, installation := range i.ManagedInstallations {
		if err := installation.Validate(); err != nil {
			return err
		}
		if seenInstallations[installation.LocalInstallationID] {
			return fmt.Errorf("duplicate local installation id %q", installation.LocalInstallationID)
		}
		seenInstallations[installation.LocalInstallationID] = true
		if i.SchemaVersion < InventorySchemaVersionWithNativeProducts && len(installation.NativeProducts) != 0 {
			return fmt.Errorf("inventory schema %d cannot contain native product observations", i.SchemaVersion)
		}
	}
	if i.SchemaVersion < InventorySchemaVersion && len(i.SaveDomains) != 0 {
		return fmt.Errorf("inventory schema %d cannot contain save domain observations", i.SchemaVersion)
	}
	if len(i.SaveDomains) > maxSaveDomains {
		return errors.New("inventory contains too many save domains")
	}
	seenSaveDomains := map[string]bool{}
	for _, domain := range i.SaveDomains {
		if err := domain.Validate(); err != nil {
			return err
		}
		if seenSaveDomains[domain.LocalSaveDomainID] {
			return fmt.Errorf("duplicate save domain id %q", domain.LocalSaveDomainID)
		}
		seenSaveDomains[domain.LocalSaveDomainID] = true
	}
	return nil
}

func (d SaveDomainObservation) Validate() error {
	if strings.TrimSpace(d.LocalSaveDomainID) == "" || strings.TrimSpace(d.AdapterID) == "" {
		return errors.New("save domain ID and adapter are required")
	}
	switch d.State {
	case "observed", "owned_here", "owned_elsewhere", "released", "reconciliation_required":
	default:
		return fmt.Errorf("unsupported save domain observation state %q", d.State)
	}
	if d.CanWrite != (d.State == "owned_here") {
		return errors.New("only a save domain owned here may be writable")
	}
	if d.CanClaim && d.State != "released" && d.State != "observed" {
		return errors.New("only an observed or released save domain may be claimable")
	}
	return nil
}

func (i ManagedInstallationObservation) Validate() error {
	if strings.TrimSpace(i.LocalInstallationID) == "" || strings.TrimSpace(i.InstallKind) == "" || strings.TrimSpace(i.Title) == "" {
		return errors.New("managed installation ID, kind, and title are required")
	}
	switch i.State {
	case "managed_here", "managed_elsewhere", "released", "installing_here", "installing_elsewhere", "legacy_unclaimed", "interrupted":
	default:
		return fmt.Errorf("unsupported managed installation state %q", i.State)
	}
	if i.State == "managed_elsewhere" || i.State == "installing_elsewhere" {
		if i.InstallPath != "" || i.CanManage || i.CanAdopt {
			return errors.New("another server's installation must not expose path or authority")
		}
	}
	if i.CanManage && i.State != "managed_here" {
		return errors.New("only a locally owned installation can be managed")
	}
	if i.CanAdopt && i.State != "released" {
		return errors.New("only a released installation can be adopted")
	}
	if i.UseGranted && i.State != "managed_elsewhere" && i.State != "released" {
		return errors.New("use grant is only reported for another or released installation")
	}
	if len(i.NativeProducts) > 16 {
		return errors.New("managed installation contains too many native products")
	}
	seenProducts := map[string]bool{}
	for index, product := range i.NativeProducts {
		if err := product.Validate(); err != nil {
			return fmt.Errorf("native product %d: %w", index, err)
		}
		key := product.Provider + ":" + strings.ToLower(product.ProductID)
		if seenProducts[key] {
			return fmt.Errorf("duplicate native product %q", product.ProductID)
		}
		seenProducts[key] = true
	}
	return nil
}

func (p NativeProductObservation) Validate() error {
	if p.Provider != "windows_uninstall" {
		return fmt.Errorf("unsupported native product provider %q", p.Provider)
	}
	if strings.TrimSpace(p.ProductID) == "" || len(p.ProductID) > 128 {
		return errors.New("native product ID is required and must be bounded")
	}
	if strings.TrimSpace(p.DisplayName) == "" || len(p.DisplayName) > 256 || len(p.Version) > 128 || len(p.Publisher) > 256 {
		return errors.New("native product display fields are missing or too long")
	}
	seen := map[string]bool{}
	for _, capability := range p.Capabilities {
		if capability != "uninstall" {
			return fmt.Errorf("unsupported native product capability %q", capability)
		}
		if seen[capability] {
			return fmt.Errorf("duplicate native product capability %q", capability)
		}
		seen[capability] = true
	}
	return nil
}

func (s StorageInventory) Validate() error {
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.Root) == "" {
		return errors.New("storage id and root are required")
	}
	if s.TotalBytes == 0 {
		return fmt.Errorf("storage %s total_bytes must be greater than zero", s.ID)
	}
	if s.FreeBytes > s.TotalBytes {
		return fmt.Errorf("storage %s free_bytes exceeds total_bytes", s.ID)
	}
	return nil
}

func (r RuntimeInventory) Validate() error {
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Name) == "" {
		return errors.New("runtime id and name are required")
	}
	if len(r.Components) > maxRuntimeComponents {
		return fmt.Errorf("runtime %s contains too many components", r.ID)
	}
	if err := validateProbeState(r.CoreProbeState); err != nil {
		return fmt.Errorf("runtime %s core probe: %w", r.ID, err)
	}
	if err := validateProbeState(r.FirmwareProbeState); err != nil {
		return fmt.Errorf("runtime %s firmware probe: %w", r.ID, err)
	}
	seen := map[string]bool{}
	for _, component := range r.Components {
		if err := component.Validate(); err != nil {
			return fmt.Errorf("runtime %s: %w", r.ID, err)
		}
		key := component.Kind + ":" + component.ID
		if seen[key] {
			return fmt.Errorf("runtime %s has duplicate component %q", r.ID, key)
		}
		seen[key] = true
	}
	return nil
}

func (p PackageManagerInventory) Validate() error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Name) == "" {
		return errors.New("package manager id and name are required")
	}
	return nil
}

func (c RuntimeComponentInventory) Validate() error {
	if c.Kind != "core" && c.Kind != "firmware" {
		return fmt.Errorf("unsupported runtime component kind %q", c.Kind)
	}
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Name) == "" {
		return errors.New("runtime component id and name are required")
	}
	return nil
}

func (a SaveAdapterInventory) Validate() error {
	if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.Name) == "" {
		return errors.New("save adapter id and name are required")
	}
	if a.ProbeState == "" {
		return fmt.Errorf("save adapter %s probe state is required", a.ID)
	}
	if err := validateProbeState(a.ProbeState); err != nil {
		return fmt.Errorf("save adapter %s probe: %w", a.ID, err)
	}
	seenKinds := map[string]bool{}
	for _, kind := range a.SaveKinds {
		if kind != "save_file" && kind != "save_ram" && kind != "save_state" {
			return fmt.Errorf("save adapter %s has unsupported save kind %q", a.ID, kind)
		}
		if seenKinds[kind] {
			return fmt.Errorf("save adapter %s has duplicate save kind %q", a.ID, kind)
		}
		seenKinds[kind] = true
	}
	return nil
}

func validateProbeState(state string) error {
	switch state {
	case "", "unknown", "complete", "partial", "unsupported":
		return nil
	default:
		return fmt.Errorf("unsupported state %q", state)
	}
}

// Normalize returns a stable snapshot for persistence and comparison.
func (i DeviceInventory) Normalize() DeviceInventory {
	i.CapturedAt = i.CapturedAt.UTC()
	i.Storage = append([]StorageInventory(nil), i.Storage...)
	i.Runtimes = append([]RuntimeInventory(nil), i.Runtimes...)
	i.PackageManagers = append([]PackageManagerInventory(nil), i.PackageManagers...)
	i.SaveAdapters = append([]SaveAdapterInventory(nil), i.SaveAdapters...)
	i.ManagedInstallations = append([]ManagedInstallationObservation(nil), i.ManagedInstallations...)
	i.SaveDomains = append([]SaveDomainObservation(nil), i.SaveDomains...)
	for index := range i.ManagedInstallations {
		i.ManagedInstallations[index].NativeProducts = append([]NativeProductObservation(nil), i.ManagedInstallations[index].NativeProducts...)
		sort.Slice(i.ManagedInstallations[index].NativeProducts, func(left, right int) bool {
			return i.ManagedInstallations[index].NativeProducts[left].ProductID < i.ManagedInstallations[index].NativeProducts[right].ProductID
		})
	}
	for index := range i.Runtimes {
		i.Runtimes[index].Components = append([]RuntimeComponentInventory(nil), i.Runtimes[index].Components...)
		sort.Slice(i.Runtimes[index].Components, func(left, right int) bool {
			if i.Runtimes[index].Components[left].Kind != i.Runtimes[index].Components[right].Kind {
				return i.Runtimes[index].Components[left].Kind < i.Runtimes[index].Components[right].Kind
			}
			return i.Runtimes[index].Components[left].ID < i.Runtimes[index].Components[right].ID
		})
	}
	sort.Slice(i.Storage, func(left, right int) bool { return i.Storage[left].ID < i.Storage[right].ID })
	sort.Slice(i.Runtimes, func(left, right int) bool { return i.Runtimes[left].ID < i.Runtimes[right].ID })
	sort.Slice(i.PackageManagers, func(left, right int) bool { return i.PackageManagers[left].ID < i.PackageManagers[right].ID })
	for index := range i.SaveAdapters {
		sort.Strings(i.SaveAdapters[index].SaveKinds)
	}
	sort.Slice(i.SaveAdapters, func(left, right int) bool { return i.SaveAdapters[left].ID < i.SaveAdapters[right].ID })
	sort.Slice(i.ManagedInstallations, func(left, right int) bool {
		return i.ManagedInstallations[left].LocalInstallationID < i.ManagedInstallations[right].LocalInstallationID
	})
	sort.Slice(i.SaveDomains, func(left, right int) bool {
		return i.SaveDomains[left].LocalSaveDomainID < i.SaveDomains[right].LocalSaveDomainID
	})
	return i
}
