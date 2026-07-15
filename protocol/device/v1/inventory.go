package v1

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const InventorySchemaVersion uint16 = 1

// DeviceInventory is a bounded snapshot of facts needed to evaluate play and
// installation options. It intentionally excludes full environment dumps and
// arbitrary installed-software lists.
type DeviceInventory struct {
	SchemaVersion uint16             `json:"schema_version"`
	CapturedAt    time.Time          `json:"captured_at"`
	Storage       []StorageInventory `json:"storage"`
	Runtimes      []RuntimeInventory `json:"runtimes"`
}

type StorageInventory struct {
	ID         string `json:"id"`
	Root       string `json:"root"`
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
}

type RuntimeInventory struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
}

func (i DeviceInventory) Validate() error {
	if i.SchemaVersion != InventorySchemaVersion {
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
	for _, runtime := range i.Runtimes {
		if err := runtime.Validate(); err != nil {
			return err
		}
		if seenRuntimes[runtime.ID] {
			return fmt.Errorf("duplicate runtime id %q", runtime.ID)
		}
		seenRuntimes[runtime.ID] = true
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
	return nil
}

// Normalize returns a stable snapshot for persistence and comparison.
func (i DeviceInventory) Normalize() DeviceInventory {
	i.CapturedAt = i.CapturedAt.UTC()
	i.Storage = append([]StorageInventory(nil), i.Storage...)
	i.Runtimes = append([]RuntimeInventory(nil), i.Runtimes...)
	sort.Slice(i.Storage, func(left, right int) bool { return i.Storage[left].ID < i.Storage[right].ID })
	sort.Slice(i.Runtimes, func(left, right int) bool { return i.Runtimes[left].ID < i.Runtimes[right].ID })
	return i
}
