package v1

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	InventorySchemaVersion       uint16 = 2
	InventorySchemaVersionLegacy uint16 = 1
	maxInventoryRuntimes                = 64
	maxRuntimeComponents                = 256
)

// DeviceInventory is a bounded snapshot of facts needed to evaluate play and
// installation options. It intentionally excludes full environment dumps and
// arbitrary installed-software lists.
type DeviceInventory struct {
	SchemaVersion   uint16                    `json:"schema_version"`
	CapturedAt      time.Time                 `json:"captured_at"`
	Storage         []StorageInventory        `json:"storage"`
	Runtimes        []RuntimeInventory        `json:"runtimes"`
	PackageManagers []PackageManagerInventory `json:"package_managers,omitempty"`
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

func (i DeviceInventory) Validate() error {
	if i.SchemaVersion != InventorySchemaVersionLegacy && i.SchemaVersion != InventorySchemaVersion {
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
	return i
}
