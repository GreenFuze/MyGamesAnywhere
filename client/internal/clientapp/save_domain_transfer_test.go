package clientapp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/google/uuid"
)

func TestSaveDomainSnapshotAndRestorePreserveChangedLocalFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "saves")
	if err := os.MkdirAll(filepath.Join(root, "slot"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "slot", "game.sav"), []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := captureSaveDomain(root, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "slot", "game.sav"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	backup, err := restoreSaveDomain(root, snapshot, true, time.Date(2026, 7, 18, 14, 0, 0, 0, time.UTC))
	if err != nil || !backup {
		t.Fatalf("restore = backup %t, error %v", backup, err)
	}
	data, err := os.ReadFile(filepath.Join(root, "slot", "game.sav"))
	if err != nil || string(data) != "before" {
		t.Fatalf("restored data = %q, %v", data, err)
	}
	backups, err := filepath.Glob(root + ".mga-backup-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, %v", backups, err)
	}
	data, err = os.ReadFile(filepath.Join(backups[0], "slot", "game.sav"))
	if err != nil || string(data) != "changed" {
		t.Fatalf("preserved data = %q, %v", data, err)
	}
}

func TestExtractSaveSnapshotRejectsTraversal(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("../outside.sav")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("bad"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractSaveSnapshot(t.TempDir(), archive.Bytes(), []devicev1.SaveDomainSnapshotFile{{Path: "../outside.sav", Size: 3, Hash: strings.Repeat("a", 64)}}); err == nil {
		t.Fatal("traversal archive was accepted")
	}
}

func TestReleasedSaveDomainRequiresReconciliationForAnotherServer(t *testing.T) {
	root := t.TempDir()
	catalog, err := OpenSaveDomainCatalog(filepath.Join(root, "authority.json"))
	if err != nil {
		t.Fatal(err)
	}
	bindingA, bindingB := uuid.NewString(), uuid.NewString()
	confirmer := &fakeSaveDomainConfirmer{claim: true, release: true}
	managerA := &LocalSaveDomainManager{catalog: catalog, bindingID: bindingA, serverURL: "http://first", saveRoot: filepath.Join(root, "domains"), confirmer: confirmer, client: http.DefaultClient, coordinator: NewInstallationCoordinator(), detector: fixedScummVMRouteDetector{gameID: "scumm:test"}, now: time.Now}
	claim := devicev1.SaveDomainClaimRequest{GameID: "game", SourceGameID: "source", Title: "Game", AdapterID: "scummvm", RouteKind: "emulator", EmulatorID: "scummvm", RouteFingerprint: strings.Repeat("c", 64)}
	claimed, err := managerA.Claim(context.Background(), claim)
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.RecordSnapshot(claimed.LocalSaveDomainID, bindingA, strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := managerA.Release(context.Background(), devicev1.SaveDomainReleaseRequest{GameID: "game", SourceGameID: "source", Title: "Game", LocalSaveDomainID: claimed.LocalSaveDomainID}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer upload-token" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var snapshot devicev1.SaveDomainSnapshot
		if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil || snapshot.Validate() != nil {
			http.Error(w, "bad snapshot", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(devicev1.SaveDomainUploadResponse{Stored: true, ManifestHash: strings.Repeat("e", 64)})
	}))
	defer server.Close()
	managerB := &LocalSaveDomainManager{catalog: catalog, bindingID: bindingB, serverURL: server.URL, saveRoot: filepath.Join(root, "domains"), confirmer: confirmer, client: server.Client(), coordinator: NewInstallationCoordinator(), detector: fixedScummVMRouteDetector{gameID: "scumm:test"}, now: time.Now}
	claim.LocalSaveDomainID = claimed.LocalSaveDomainID
	pending, err := managerB.Claim(context.Background(), claim)
	if err != nil || pending.State != "reconciliation_required" {
		t.Fatalf("pending claim = %+v, %v", pending, err)
	}
	domain, _ := catalog.FindByID(claimed.LocalSaveDomainID)
	if domain.State != SaveDomainReconciliationRequired || domain.WriterBindingID != "" || domain.PendingWriterBindingID != bindingB {
		t.Fatalf("pending domain = %+v", domain)
	}
	reconciled, err := managerB.Reconcile(context.Background(), devicev1.SaveDomainReconcileRequest{GameID: "game", SourceGameID: "source", Title: "Game", LocalSaveDomainID: claimed.LocalSaveDomainID, Strategy: "keep_local", TransferURL: "/api/device-transfers/save-domain", TransferToken: "upload-token"})
	if err != nil || reconciled.State != "owned_here" || reconciled.ManifestHash != strings.Repeat("e", 64) {
		t.Fatalf("reconciled = %+v, %v", reconciled, err)
	}
	domain, _ = catalog.FindByID(claimed.LocalSaveDomainID)
	if domain.State != SaveDomainOwned || domain.WriterBindingID != bindingB {
		t.Fatalf("owned domain = %+v", domain)
	}
}

func TestRestoreRejectsInvalidSnapshotArchive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.RawQuery != "" || request.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "missing bounded bearer token", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(devicev1.SaveDomainSnapshot{LocalFingerprint: strings.Repeat("a", 64), CapturedAt: time.Now(), ArchiveBase64: base64.StdEncoding.EncodeToString([]byte("not zip"))})
	}))
	defer server.Close()
	manager := &LocalSaveDomainManager{serverURL: server.URL, client: server.Client()}
	snapshot, err := manager.downloadSnapshot(context.Background(), "/api/device-transfers/save-domain", "token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restoreSaveDomain(filepath.Join(t.TempDir(), "saves"), snapshot, false, time.Now()); err == nil {
		t.Fatal("invalid restore snapshot was accepted")
	}
}
