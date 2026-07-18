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
		SaveAdapters:    []SaveAdapterInventory{{ID: "retroarch", Name: "RetroArch", ProbeState: "complete", SaveKinds: []string{"save_ram", "save_state"}}},
	}
	if err := current.Validate(); err != nil {
		t.Fatalf("component inventory rejected: %v", err)
	}
}

func TestInventoryValidationAcceptsSchemaTwoWithoutSaveAdapters(t *testing.T) {
	previous := DeviceInventory{
		SchemaVersion: InventorySchemaVersionPrevious, CapturedAt: time.Now(),
		PackageManagers: []PackageManagerInventory{{ID: "winget", Name: "Windows Package Manager"}},
	}
	if err := previous.Validate(); err != nil {
		t.Fatalf("schema 2 inventory rejected: %v", err)
	}
	previous.SaveAdapters = []SaveAdapterInventory{{ID: "scummvm", Name: "ScummVM", ProbeState: "complete", SaveKinds: []string{"save_file"}}}
	if err := previous.Validate(); err == nil {
		t.Fatal("schema 2 save adapters were accepted")
	}
}

func TestInventoryManagedInstallationObservationIsBoundedAndSanitized(t *testing.T) {
	inventory := DeviceInventory{SchemaVersion: InventorySchemaVersion, CapturedAt: time.Now(), ManagedInstallations: []ManagedInstallationObservation{{LocalInstallationID: "local-1", State: "managed_elsewhere", InstallKind: "managed_archive", Title: "Game"}}}
	if err := inventory.Validate(); err != nil {
		t.Fatal(err)
	}
	inventory.ManagedInstallations[0].InstallPath = `C:\Games\secret`
	if err := inventory.Validate(); err == nil {
		t.Fatal("other server path was accepted")
	}
}

func TestInventoryValidationRejectsInvalidSaveAdapters(t *testing.T) {
	for name, adapters := range map[string][]SaveAdapterInventory{
		"duplicate": {{ID: "scummvm", Name: "ScummVM", ProbeState: "complete"}, {ID: "scummvm", Name: "ScummVM", ProbeState: "partial"}},
		"state":     {{ID: "scummvm", Name: "ScummVM", ProbeState: "ready"}},
		"kind":      {{ID: "scummvm", Name: "ScummVM", ProbeState: "complete", SaveKinds: []string{"memory_card"}}},
	} {
		t.Run(name, func(t *testing.T) {
			inventory := DeviceInventory{SchemaVersion: InventorySchemaVersion, CapturedAt: time.Now(), SaveAdapters: adapters}
			if err := inventory.Validate(); err == nil {
				t.Fatal("invalid save adapters were accepted")
			}
		})
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
