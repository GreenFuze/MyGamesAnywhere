package v1

import (
	"testing"
	"time"
)

func TestDeviceInventoryValidationAndNormalization(t *testing.T) {
	t.Parallel()
	inventory := DeviceInventory{
		SchemaVersion: InventorySchemaVersion,
		CapturedAt:    time.Now(),
		Storage: []StorageInventory{
			{ID: "d", Root: `D:\`, TotalBytes: 200, FreeBytes: 100},
			{ID: "c", Root: `C:\`, TotalBytes: 100, FreeBytes: 25},
		},
		Runtimes: []RuntimeInventory{
			{ID: "steam", Name: "Steam"},
			{ID: "retroarch", Name: "RetroArch"},
		},
	}
	if err := inventory.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	normalized := inventory.Normalize()
	if normalized.Storage[0].ID != "c" || normalized.Runtimes[0].ID != "retroarch" {
		t.Fatalf("Normalize() = %#v", normalized)
	}
}

func TestDeviceInventoryRejectsUnsafeOrInconsistentFacts(t *testing.T) {
	t.Parallel()
	tests := []DeviceInventory{
		{},
		{SchemaVersion: InventorySchemaVersion, CapturedAt: time.Now(), Storage: []StorageInventory{{ID: "c", Root: `C:\`, TotalBytes: 10, FreeBytes: 11}}},
		{SchemaVersion: InventorySchemaVersion, CapturedAt: time.Now(), Runtimes: []RuntimeInventory{{ID: "steam", Name: "Steam"}, {ID: "steam", Name: "Steam"}}},
	}
	for _, inventory := range tests {
		if err := inventory.Validate(); err == nil {
			t.Fatalf("Validate(%#v) error = nil", inventory)
		}
	}
}
