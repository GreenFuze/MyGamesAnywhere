package clientapp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/google/uuid"
)

type fakeSaveDomainConfirmer struct {
	claim        bool
	release      bool
	claims       int
	claimPreview SaveDomainClaimPreview
}

type fixedScummVMRouteDetector struct{ gameID string }

func (d fixedScummVMRouteDetector) Detect(context.Context, string) (string, error) {
	return d.gameID, nil
}

func (f *fakeSaveDomainConfirmer) ConfirmClaim(_ context.Context, preview SaveDomainClaimPreview) (bool, error) {
	f.claims++
	f.claimPreview = preview
	return f.claim, nil
}

func (f *fakeSaveDomainConfirmer) ConfirmRelease(context.Context, string, string) (bool, error) {
	return f.release, nil
}

func (f *fakeSaveDomainConfirmer) ConfirmRestore(context.Context, string, string) (bool, error) {
	return true, nil
}

func TestLocalSaveDomainManagerRequiresConfirmationAndPreservesFilesOnRelease(t *testing.T) {
	root := t.TempDir()
	catalog, err := OpenSaveDomainCatalog(filepath.Join(root, "authority.json"))
	if err != nil {
		t.Fatal(err)
	}
	bindingID := uuid.NewString()
	confirmer := &fakeSaveDomainConfirmer{}
	manager := &LocalSaveDomainManager{catalog: catalog, bindingID: bindingID, serverURL: "http://mga:8900", saveRoot: filepath.Join(root, "save-domains"), confirmer: confirmer, coordinator: NewInstallationCoordinator(), detector: fixedScummVMRouteDetector{gameID: "scumm:test"}, now: time.Now}
	request := devicev1.SaveDomainClaimRequest{GameID: "game-1", SourceGameID: "source-1", Title: "Game", AdapterID: "scummvm", RouteKind: "emulator", EmulatorID: "scummvm", RouteFingerprint: strings.Repeat("a", 64)}
	if _, err := manager.Claim(context.Background(), request); err != ErrSaveDomainConfirmationDeclined {
		t.Fatalf("declined claim error = %v", err)
	}
	expectedPath := filepath.Join(root, "save-domains", "scummvm", strings.Repeat("a", 16))
	if confirmer.claimPreview.Title != "Game" ||
		confirmer.claimPreview.Server != "http://mga:8900" ||
		confirmer.claimPreview.Adapter != "ScummVM" ||
		confirmer.claimPreview.ExactTarget != "scumm:test" ||
		confirmer.claimPreview.SaveKind != "ScummVM save files" ||
		confirmer.claimPreview.LocalPath != expectedPath {
		t.Fatalf("claim preview = %+v", confirmer.claimPreview)
	}
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Fatalf("declined claim prepared save folder: %v", err)
	}
	confirmer.claim = true
	confirmer.release = true
	claimed, err := manager.Claim(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	domain, found := catalog.FindByID(claimed.LocalSaveDomainID)
	if !found || len(domain.ResolvedPaths) != 1 {
		t.Fatalf("claimed domain = %+v", domain)
	}
	marker := filepath.Join(domain.ResolvedPaths[0], "game.sav")
	if err := os.WriteFile(marker, []byte("save"), 0o600); err != nil {
		t.Fatal(err)
	}
	released, err := manager.Release(context.Background(), devicev1.SaveDomainReleaseRequest{GameID: "game-1", SourceGameID: "source-1", Title: "Game", LocalSaveDomainID: claimed.LocalSaveDomainID})
	if err != nil {
		t.Fatal(err)
	}
	if released.State != "released" {
		t.Fatalf("release result = %+v", released)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "save" {
		t.Fatalf("release changed save files: %q, %v", data, err)
	}
}

func TestLocalSaveDomainManagerRejectsAnotherWriterBeforeConfirmation(t *testing.T) {
	root := t.TempDir()
	catalog, err := OpenSaveDomainCatalog(filepath.Join(root, "authority.json"))
	if err != nil {
		t.Fatal(err)
	}
	request := devicev1.SaveDomainClaimRequest{GameID: "game-1", SourceGameID: "source-1", Title: "Game", AdapterID: "scummvm", RouteKind: "emulator", EmulatorID: "scummvm", RouteFingerprint: strings.Repeat("b", 64)}
	first := &LocalSaveDomainManager{catalog: catalog, bindingID: uuid.NewString(), serverURL: "http://first:8900", saveRoot: filepath.Join(root, "save-domains"), confirmer: &fakeSaveDomainConfirmer{claim: true}, detector: fixedScummVMRouteDetector{gameID: "scumm:test"}, now: time.Now}
	if _, err := first.Claim(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	secondConfirmation := &fakeSaveDomainConfirmer{claim: true}
	second := &LocalSaveDomainManager{catalog: catalog, bindingID: uuid.NewString(), serverURL: "http://second:8900", saveRoot: filepath.Join(root, "save-domains"), confirmer: secondConfirmation, detector: fixedScummVMRouteDetector{gameID: "scumm:test"}, now: time.Now}
	if _, err := second.Claim(context.Background(), request); err == nil || !strings.Contains(err.Error(), "another MGA Server") {
		t.Fatalf("second claim error = %v", err)
	}
	if secondConfirmation.claims != 0 {
		t.Fatalf("confirmation count = %d, want 0", secondConfirmation.claims)
	}
}
