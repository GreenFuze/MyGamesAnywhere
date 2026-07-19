package clientapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo"
	clientconfig "github.com/GreenFuze/MyGamesAnywhere/client/internal/config"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestOwnershipCatalogMigratesSchemaOneAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ownership.json")
	root := filepath.Join(t.TempDir(), "Games", "MGA", "one")
	now := time.Now().UTC()
	legacy := ownershipCatalogDocument{SchemaVersion: ownershipCatalogLegacySchemaVersion, Installations: []InstallationOwnershipRecord{{LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, State: OwnershipOwned, InstallKind: devicev1.InstallKindManagedArchive, InstallRoot: root, InstallPath: filepath.Join(root, "Game"), CreatedAt: now, UpdatedAt: now}}}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenOwnershipCatalog(path); err != nil {
		t.Fatal(err)
	}
	migrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(migrated, &header); err != nil || header.SchemaVersion != ownershipCatalogSchemaVersion {
		t.Fatalf("migrated schema = %d, error = %v", header.SchemaVersion, err)
	}
}

const (
	testBindingOne = "11111111-1111-4111-8111-111111111111"
	testBindingTwo = "22222222-2222-4222-8222-222222222222"
	testInstallID  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

func TestOwnershipCatalogReleaseAdoptAndOwnerGuard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ownership.json")
	catalog, err := OpenOwnershipCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Games", "MGA", "one")
	record := InstallationOwnershipRecord{LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, State: OwnershipInstalling, InstallKind: "managed_archive", InstallRoot: root, InstallPath: filepath.Join(root, "Game"), Title: "Game", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := catalog.BeginInstall(record); err != nil {
		t.Fatal(err)
	}
	if err := catalog.CompleteInstall(testInstallID, testBindingTwo); err == nil {
		t.Fatal("other binding completed installation")
	}
	if err := catalog.CompleteInstall(testInstallID, testBindingOne); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Release(testInstallID, testBindingTwo); err == nil {
		t.Fatal("other binding released installation")
	}
	if err := catalog.Release(testInstallID, testBindingOne); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Adopt(testInstallID, testBindingTwo); err != nil {
		t.Fatal(err)
	}
	reloaded, err := OpenOwnershipCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	items := reloaded.List()
	if len(items) != 1 || items[0].OwnerBindingID != testBindingTwo || items[0].State != OwnershipOwned || len(items[0].PreviousOwners) != 1 {
		t.Fatalf("adopted record = %+v", items)
	}
}

func TestServiceReleaseAndAdoptPreservesFilesAndRewritesManifest(t *testing.T) {
	service, err := New(t.TempDir(), buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	one := serviceTestBinding("one", "http://localhost:8900")
	two := serviceTestBinding("two", "http://tv2:8900")
	if err := service.configs.Save(clientconfig.Document{SchemaVersion: clientconfig.SchemaVersion, Bindings: []clientconfig.Binding{one, two}}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Games", "MGA", "one")
	path := filepath.Join(root, "Game")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeInstallManifest(path, installManifest{SchemaVersion: devicev1.InstallManifestSchemaVersion, LocalInstallationID: testInstallID, OwnerBindingID: one.BindingID, OwnershipState: string(OwnershipOwned), GameID: "game", SourceGameID: "source", InstallRoot: root, InstalledAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	record := InstallationOwnershipRecord{LocalInstallationID: testInstallID, OwnerBindingID: one.BindingID, State: OwnershipInstalling, InstallKind: devicev1.InstallKindManagedArchive, InstallRoot: root, InstallPath: path, GameID: "game", SourceGameID: "source", Title: "Game", CreatedAt: now, UpdatedAt: now}
	if err := service.ownership.BeginInstall(record); err != nil {
		t.Fatal(err)
	}
	if err := service.ownership.CompleteInstall(testInstallID, one.BindingID); err != nil {
		t.Fatal(err)
	}
	if err := service.ReleaseInstallation(ReleaseInstallationOptions{LocalInstallationID: testInstallID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("release removed files: %v", err)
	}
	manifest, _ := readInstallManifest(path)
	if manifest.OwnerBindingID != "" || manifest.OwnershipState != string(OwnershipReleased) {
		t.Fatalf("released manifest = %+v", manifest)
	}
	if err := service.AdoptInstallation(AdoptInstallationOptions{LocalInstallationID: testInstallID, ServerURL: two.ServerURL}); err != nil {
		t.Fatal(err)
	}
	manifest, _ = readInstallManifest(path)
	if manifest.OwnerBindingID != two.BindingID || manifest.OwnershipState != string(OwnershipOwned) {
		t.Fatalf("adopted manifest = %+v", manifest)
	}
}

func TestInstallationCoordinatorRejectsPathAndProductRaces(t *testing.T) {
	coordinator := NewInstallationCoordinator()
	release, err := coordinator.Reserve(testBindingOne, `C:\Games\MGA\one\Game`, "gog:game")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Reserve(testBindingTwo, `C:\Games\MGA\one\Game`, "other"); err == nil {
		t.Fatal("same path reservation succeeded")
	}
	if _, err := coordinator.Reserve(testBindingTwo, `C:\Games\MGA\two\Game`, "gog:game"); err == nil {
		t.Fatal("same product reservation succeeded")
	}
	release()
	if done, err := coordinator.Reserve(testBindingTwo, `C:\Games\MGA\two\Game`, "gog:game"); err != nil {
		t.Fatal(err)
	} else {
		done()
	}
}

func TestOwnershipCatalogRecoversInterruptedInstallForRelease(t *testing.T) {
	catalog, err := OpenOwnershipCatalog(filepath.Join(t.TempDir(), "ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Games", "MGA", "one")
	now := time.Now().UTC()
	record := InstallationOwnershipRecord{LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, State: OwnershipInstalling, InstallKind: "managed_archive", InstallRoot: root, InstallPath: filepath.Join(root, "Game"), Title: "Game", CreatedAt: now, UpdatedAt: now}
	if err := catalog.BeginInstall(record); err != nil {
		t.Fatal(err)
	}
	if err := catalog.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	if got := catalog.List()[0].State; got != OwnershipInterrupted {
		t.Fatalf("state = %s", got)
	}
	if err := catalog.Release(testInstallID, testBindingOne); err == nil {
		t.Fatal("interrupted installation became adoptable")
	}
}

func TestManagedInstallationObservationsHideOtherServerPath(t *testing.T) {
	catalog, err := OpenOwnershipCatalog(filepath.Join(t.TempDir(), "ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Games", "MGA", "one")
	now := time.Now().UTC()
	record := InstallationOwnershipRecord{LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, State: OwnershipInstalling, InstallKind: "managed_archive", InstallRoot: root, InstallPath: filepath.Join(root, "Game"), Title: "Game", CreatedAt: now, UpdatedAt: now}
	if err := catalog.BeginInstall(record); err != nil {
		t.Fatal(err)
	}
	if err := catalog.CompleteInstall(testInstallID, testBindingOne); err != nil {
		t.Fatal(err)
	}
	here, err := NewOwnedLocalInventoryCollector(catalog, testBindingOne).managedInstallationObservations()
	if err != nil {
		t.Fatal(err)
	}
	elsewhere, err := NewOwnedLocalInventoryCollector(catalog, testBindingTwo).managedInstallationObservations()
	if err != nil {
		t.Fatal(err)
	}
	if len(here) != 1 || !here[0].CanManage || here[0].InstallPath == "" || here[0].State != "managed_here" {
		t.Fatalf("owner observation = %+v", here)
	}
	if len(elsewhere) != 1 || elsewhere[0].CanManage || elsewhere[0].InstallPath != "" || elsewhere[0].State != "managed_elsewhere" {
		t.Fatalf("other observation = %+v", elsewhere)
	}
}

func TestOwnershipCatalogRefreshFailsFastOnTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ownership.json")
	catalog, err := OpenOwnershipCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Games", "MGA", "one")
	now := time.Now().UTC()
	record := InstallationOwnershipRecord{LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, State: OwnershipInstalling, InstallKind: "managed_archive", InstallRoot: root, InstallPath: filepath.Join(root, "Game"), Title: "Game", CreatedAt: now, UpdatedAt: now}
	if err := catalog.BeginInstall(record); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, []byte("{}")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.CompleteInstall(testInstallID, testBindingOne); err == nil {
		t.Fatal("catalog mutation accepted trailing JSON")
	}
}
