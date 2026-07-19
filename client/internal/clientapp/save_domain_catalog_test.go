package clientapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSaveDomainCatalogClaimsOneWriterAndReleasesWithoutFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "authority.json")
	saves := filepath.Join(root, "saves")
	if err := os.MkdirAll(saves, 0o755); err != nil {
		t.Fatal(err)
	}
	catalog, err := OpenSaveDomainCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	catalog.now = func() time.Time { return now }
	domain, err := catalog.Resolve("scummvm", "route-a", "evidence-a", []string{saves})
	if err != nil {
		t.Fatal(err)
	}
	bindingA := uuid.NewString()
	bindingB := uuid.NewString()
	if err := catalog.SetScummVMGameID(domain.LocalSaveDomainID, "scumm:test"); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Claim(domain.LocalSaveDomainID, bindingA); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Claim(domain.LocalSaveDomainID, bindingB); err == nil {
		t.Fatal("second writer was allowed to claim the save domain")
	}
	marker := filepath.Join(saves, "game.sav")
	if err := os.WriteFile(marker, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Release(domain.LocalSaveDomainID, bindingA); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "preserve" {
		t.Fatalf("release changed save files: %q, %v", data, err)
	}
	if err := catalog.Claim(domain.LocalSaveDomainID, bindingB); err != nil {
		t.Fatal(err)
	}
	pending, _ := catalog.FindByID(domain.LocalSaveDomainID)
	if pending.State != SaveDomainReconciliationRequired || pending.PendingWriterBindingID != bindingB {
		t.Fatalf("pending transfer = %+v", pending)
	}
	if err := catalog.CompleteReconciliation(domain.LocalSaveDomainID, bindingB, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
}

func TestSaveDomainCatalogEvidenceChangeFailsClosed(t *testing.T) {
	root := t.TempDir()
	catalog, err := OpenSaveDomainCatalog(filepath.Join(root, "authority.json"))
	if err != nil {
		t.Fatal(err)
	}
	catalog.now = func() time.Time { return time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC) }
	domain, err := catalog.Resolve("scummvm", "route-a", "evidence-a", []string{filepath.Join(root, "one")})
	if err != nil {
		t.Fatal(err)
	}
	bindingID := uuid.NewString()
	if err := catalog.SetScummVMGameID(domain.LocalSaveDomainID, "scumm:test"); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Claim(domain.LocalSaveDomainID, bindingID); err != nil {
		t.Fatal(err)
	}
	changed, err := catalog.Resolve("scummvm", "route-a", "evidence-b", []string{filepath.Join(root, "two")})
	if err != nil {
		t.Fatal(err)
	}
	if changed.State != SaveDomainReconciliationRequired || changed.WriterBindingID != "" || changed.PendingWriterBindingID != bindingID {
		t.Fatalf("changed evidence retained writer authority: %+v", changed)
	}
	if err := catalog.Claim(changed.LocalSaveDomainID, uuid.NewString()); err == nil {
		t.Fatal("reconciliation-required save domain was claimable")
	}
}

func TestSaveDomainCatalogRejectsUnknownAndTrailingJSON(t *testing.T) {
	for name, data := range map[string]string{
		"unknown":  `{"schema_version":1,"domains":[],"future":true}`,
		"trailing": `{"schema_version":1,"domains":[]} {}`,
		"schema":   `{"schema_version":2,"domains":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "authority.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenSaveDomainCatalog(path); err == nil {
				t.Fatal("invalid save domain catalog was accepted")
			}
		})
	}
}

func TestSaveDomainInventoryDoesNotLeakPathsOrWriterIdentity(t *testing.T) {
	root := t.TempDir()
	catalog, err := OpenSaveDomainCatalog(filepath.Join(root, "authority.json"))
	if err != nil {
		t.Fatal(err)
	}
	domain, err := catalog.Resolve("scummvm", "route-a", "evidence-a", []string{filepath.Join(root, "secret-saves")})
	if err != nil {
		t.Fatal(err)
	}
	owner := uuid.NewString()
	observer := uuid.NewString()
	if err := catalog.SetScummVMGameID(domain.LocalSaveDomainID, "scumm:test"); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Claim(domain.LocalSaveDomainID, owner); err != nil {
		t.Fatal(err)
	}
	collector := NewOwnedLocalInventoryCollectorWithSaveDomains(nil, catalog, observer)
	observations := collector.saveDomainObservations()
	if len(observations) != 1 || observations[0].State != "owned_elsewhere" || observations[0].CanWrite || observations[0].CanClaim {
		t.Fatalf("observer inventory = %+v", observations)
	}
	encoded := strings.ToLower(strings.TrimSpace(observations[0].LocalSaveDomainID + observations[0].AdapterID + observations[0].State))
	if strings.Contains(encoded, "secret") || strings.Contains(encoded, strings.ToLower(owner)) {
		t.Fatalf("inventory leaked local authority evidence: %s", encoded)
	}
}
