package clientapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type fakeUseExistingConfirmer struct {
	approved bool
	calls    int
}

func (f *fakeUseExistingConfirmer) Confirm(context.Context, string, string, string) (bool, error) {
	f.calls++
	return f.approved, nil
}

func TestUseExistingInstallationCreatesLaunchOnlyBindingGrant(t *testing.T) {
	catalog, err := OpenOwnershipCatalog(filepath.Join(t.TempDir(), "ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Games", "MGA", "one")
	path := filepath.Join(root, "Game")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "game.exe"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	record := InstallationOwnershipRecord{LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, State: OwnershipInstalling, InstallKind: devicev1.InstallKindManagedArchive, InstallRoot: root, InstallPath: path, Title: "Game", CreatedAt: now, UpdatedAt: now}
	if err := catalog.BeginInstall(record); err != nil {
		t.Fatal(err)
	}
	if err := catalog.CompleteInstall(testInstallID, testBindingOne); err != nil {
		t.Fatal(err)
	}
	if err := writeInstallManifest(path, installManifest{SchemaVersion: devicev1.InstallManifestSchemaVersion, LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, OwnershipState: string(OwnershipOwned), GameID: "owner-game", SourceGameID: "owner-source", InstallRoot: root, LaunchTarget: "game.exe", LaunchCandidates: []string{"game.exe"}, InstalledAt: now}); err != nil {
		t.Fatal(err)
	}
	ownership, err := NewInstallationOwnership(testBindingTwo, "http://tv2:8900", 2, catalog, NewInstallationCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	confirmer := &fakeUseExistingConfirmer{approved: true}
	user := &LocalExistingInstallationUser{ownership: ownership, serverURL: "http://tv2:8900", confirmer: confirmer, now: func() time.Time { return now }}
	result, err := user.Use(context.Background(), devicev1.UseExistingInstallationRequest{LocalInstallationID: testInstallID, GameID: "target-game", SourceGameID: "target-source", Title: "Game"})
	if err != nil {
		t.Fatal(err)
	}
	if confirmer.calls != 1 || !catalog.HasUseGrant(testInstallID, testBindingTwo) || result.GameID != "target-game" {
		t.Fatalf("confirmation/grant/result = %d %t %+v", confirmer.calls, catalog.HasUseGrant(testInstallID, testBindingTwo), result)
	}
	stored, _ := catalog.FindByID(testInstallID)
	if stored.OwnerBindingID != testBindingOne || stored.State != OwnershipOwned {
		t.Fatalf("use grant changed ownership: %+v", stored)
	}
}

func TestUseExistingInstallationDeclineDoesNotGrant(t *testing.T) {
	catalog, err := OpenOwnershipCatalog(filepath.Join(t.TempDir(), "ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Games", "MGA", "one")
	path := filepath.Join(root, "Game")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "game.exe"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := catalog.BeginInstall(InstallationOwnershipRecord{LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, State: OwnershipInstalling, InstallKind: devicev1.InstallKindManagedArchive, InstallRoot: root, InstallPath: path, Title: "Game", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.CompleteInstall(testInstallID, testBindingOne); err != nil {
		t.Fatal(err)
	}
	if err := writeInstallManifest(path, installManifest{SchemaVersion: devicev1.InstallManifestSchemaVersion, LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, InstallRoot: root, LaunchTarget: "game.exe", LaunchCandidates: []string{"game.exe"}}); err != nil {
		t.Fatal(err)
	}
	ownership, _ := NewInstallationOwnership(testBindingTwo, "http://tv2:8900", 2, catalog, NewInstallationCoordinator())
	user := &LocalExistingInstallationUser{ownership: ownership, serverURL: "http://tv2:8900", confirmer: &fakeUseExistingConfirmer{}, now: time.Now}
	_, err = user.Use(context.Background(), devicev1.UseExistingInstallationRequest{LocalInstallationID: testInstallID, GameID: "target-game", SourceGameID: "target-source", Title: "Game"})
	if err != ErrUseExistingDeclined || catalog.HasUseGrant(testInstallID, testBindingTwo) {
		t.Fatalf("decline error/grant = %v %t", err, catalog.HasUseGrant(testInstallID, testBindingTwo))
	}
}
