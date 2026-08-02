package clientapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestInstallationRecoveryRepairsOneUnambiguousLaunchTarget(t *testing.T) {
	root := t.TempDir()
	installPath := filepath.Join(root, "Plasma Pong")
	if err := os.MkdirAll(installPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installPath, "Plasma Pong.exe"), []byte("game"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := OpenOwnershipCatalog(filepath.Join(t.TempDir(), "ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	ownership, err := NewInstallationOwnership(testBindingOne, "http://mga:8900", 1, catalog, NewInstallationCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	record := InstallationOwnershipRecord{
		LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, State: OwnershipInstalling,
		InstallKind: devicev1.InstallKindManagedArchive, InstallRoot: root, InstallPath: installPath,
		GameID: "game", SourceGameID: "source", Title: "Plasma Pong", CreatedAt: now, UpdatedAt: now,
	}
	if err := catalog.BeginInstall(record); err != nil {
		t.Fatal(err)
	}
	if err := catalog.CompleteInstall(testInstallID, testBindingOne); err != nil {
		t.Fatal(err)
	}
	if err := writeInstallManifest(installPath, installManifest{
		SchemaVersion: devicev1.InstallManifestSchemaVersion, LocalInstallationID: testInstallID,
		OwnerBindingID: testBindingOne, OwnershipState: string(OwnershipOwned),
		GameID: "game", SourceGameID: "source", InstallRoot: root,
		LaunchTarget: "Old.exe", LaunchCandidates: []string{"Old.exe"}, InstalledAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	manager, err := NewInstallationRecoveryManager(ownership, "http://mga:8900", fakeRegisteredPrograms{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Recover(context.Background(), devicev1.InstallationRecoveryRequest{
		Action: devicev1.InstallationRecoveryRepair, GameID: "game", SourceGameID: "source",
		LocalInstallationID: testInstallID, InstallKind: devicev1.InstallKindManagedArchive,
		InstallRoot: root, InstallPath: installPath, InstallState: devicev1.InstallStateNeedsRepair,
		ReasonCode: devicev1.ValidationReasonLaunchTargetMissing,
	}, nil)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if result.LaunchTarget != "Plasma Pong.exe" || result.Released {
		t.Fatalf("Recover() result = %+v", result)
	}
	manifest, err := readInstallManifest(installPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.LaunchTarget != "Plasma Pong.exe" {
		t.Fatalf("manifest launch target = %q", manifest.LaunchTarget)
	}
}

func TestInstallationRecoveryFailsClosedForAmbiguousExecutables(t *testing.T) {
	root := t.TempDir()
	installPath := filepath.Join(root, "Game")
	if err := os.MkdirAll(installPath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Alpha.exe", "Beta.exe"} {
		if err := os.WriteFile(filepath.Join(installPath, name), []byte("game"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalog, _ := OpenOwnershipCatalog(filepath.Join(t.TempDir(), "ownership.json"))
	ownership, _ := NewInstallationOwnership(testBindingOne, "http://mga:8900", 1, catalog, NewInstallationCoordinator())
	now := time.Now().UTC()
	_ = catalog.BeginInstall(InstallationOwnershipRecord{
		LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, State: OwnershipInstalling,
		InstallKind: devicev1.InstallKindManagedArchive, InstallRoot: root, InstallPath: installPath,
		GameID: "game", SourceGameID: "source", Title: "Game", CreatedAt: now, UpdatedAt: now,
	})
	_ = catalog.CompleteInstall(testInstallID, testBindingOne)
	_ = writeInstallManifest(installPath, installManifest{
		SchemaVersion: devicev1.InstallManifestSchemaVersion, LocalInstallationID: testInstallID,
		OwnerBindingID: testBindingOne, GameID: "game", SourceGameID: "source", InstallRoot: root,
		LaunchTarget: "Old.exe", LaunchCandidates: []string{"Old.exe"}, InstalledAt: now,
	})
	manager, _ := NewInstallationRecoveryManager(ownership, "http://mga:8900", fakeRegisteredPrograms{})
	_, err := manager.Recover(context.Background(), devicev1.InstallationRecoveryRequest{
		Action: devicev1.InstallationRecoveryRepair, GameID: "game", SourceGameID: "source",
		InstallKind: devicev1.InstallKindManagedArchive, InstallRoot: root, InstallPath: installPath,
		InstallState: devicev1.InstallStateNeedsRepair, ReasonCode: devicev1.ValidationReasonLaunchTargetMissing,
	}, nil)
	if err == nil {
		t.Fatal("Recover() accepted an ambiguous executable set")
	}
}
