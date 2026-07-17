package v1

import (
	"testing"
	"time"
)

func TestInventoryValidationAcceptsLegacyAndComponentSnapshots(t *testing.T) {
	legacy := DeviceInventory{SchemaVersion: InventorySchemaVersionLegacy, CapturedAt: time.Now(), Runtimes: []RuntimeInventory{{ID: "scummvm", Name: "ScummVM"}}}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy inventory rejected: %v", err)
	}
	current := DeviceInventory{
		SchemaVersion: InventorySchemaVersion, CapturedAt: time.Now(),
		PackageManagers: []PackageManagerInventory{{ID: "winget", Name: "Windows Package Manager"}},
		Runtimes:        []RuntimeInventory{{ID: "retroarch", Name: "RetroArch", CoreProbeState: "complete", FirmwareProbeState: "unknown", Components: []RuntimeComponentInventory{{Kind: "core", ID: "snes9x", Name: "Snes9x"}}}},
	}
	if err := current.Validate(); err != nil {
		t.Fatalf("component inventory rejected: %v", err)
	}
}

func TestInventoryValidationRejectsSchemaOneExtensionAndDuplicateComponent(t *testing.T) {
	legacy := DeviceInventory{SchemaVersion: InventorySchemaVersionLegacy, CapturedAt: time.Now(), PackageManagers: []PackageManagerInventory{{ID: "winget", Name: "Winget"}}}
	if err := legacy.Validate(); err == nil {
		t.Fatal("schema 1 extension was accepted")
	}
	duplicate := DeviceInventory{SchemaVersion: InventorySchemaVersion, CapturedAt: time.Now(), Runtimes: []RuntimeInventory{{
		ID: "retroarch", Name: "RetroArch", Components: []RuntimeComponentInventory{{Kind: "core", ID: "snes9x", Name: "Snes9x"}, {Kind: "core", ID: "snes9x", Name: "Snes9x"}},
	}}}
	if err := duplicate.Validate(); err == nil {
		t.Fatal("duplicate runtime component was accepted")
	}
}
