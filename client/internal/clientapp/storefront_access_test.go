package clientapp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type fakeStorefrontObserver struct {
	installPath string
	observedAt  time.Time
	calls       int
}

func (f *fakeStorefrontObserver) Observe(_ context.Context, candidates []devicev1.StorefrontProductCandidate) ([]devicev1.StorefrontProductObservation, error) {
	f.calls++
	result := make([]devicev1.StorefrontProductObservation, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, devicev1.StorefrontProductObservation{StorefrontProductCandidate: candidate, InstallPath: f.installPath, ObservedAt: f.observedAt})
	}
	return result, nil
}

type fakeStorefrontLauncher struct {
	provider  string
	productID string
	calls     int
}

func (f *fakeStorefrontLauncher) Launch(_ context.Context, provider, productID string) error {
	f.provider, f.productID = provider, productID
	f.calls++
	return nil
}

type fakeStorefrontConfirmer struct{ calls int }

func (f *fakeStorefrontConfirmer) Confirm(context.Context, string, string, string) (bool, error) {
	f.calls++
	return true, nil
}

func TestStorefrontGrantIsBindingScopedAndRevalidatedBeforeLaunch(t *testing.T) {
	catalog, err := OpenStorefrontGrantCatalog(filepath.Join(t.TempDir(), "storefront-grants.json"))
	if err != nil {
		t.Fatal(err)
	}
	const bindingA = "11111111-1111-4111-8111-111111111111"
	const bindingB = "22222222-2222-4222-8222-222222222222"
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	observer := &fakeStorefrontObserver{installPath: filepath.Join(t.TempDir(), "Steam", "Game"), observedAt: now}
	launcher := &fakeStorefrontLauncher{}
	confirmer := &fakeStorefrontConfirmer{}
	accessA, err := NewLocalStorefrontAccess(bindingA, "http://server-a:8900", catalog)
	if err != nil {
		t.Fatal(err)
	}
	accessA.observer, accessA.launcher, accessA.confirmer, accessA.now = observer, launcher, confirmer, func() time.Time { return now }
	candidate := devicev1.StorefrontProductCandidate{GameID: "game-1", SourceGameID: "source-1", Provider: devicev1.StorefrontProviderSteam, ProductID: "12345", Title: "Game"}
	if _, err := accessA.Use(context.Background(), devicev1.UseStorefrontProductRequest{StorefrontProductCandidate: candidate}); err != nil {
		t.Fatal(err)
	}
	if confirmer.calls != 1 || !catalog.Has(bindingA, candidate.Provider, candidate.ProductID) || catalog.Has(bindingB, candidate.Provider, candidate.ProductID) {
		t.Fatalf("grant was not isolated to binding A: confirmations=%d", confirmer.calls)
	}
	if _, err := accessA.Use(context.Background(), devicev1.UseStorefrontProductRequest{StorefrontProductCandidate: candidate}); err != nil {
		t.Fatal(err)
	}
	if confirmer.calls != 1 {
		t.Fatalf("existing grant prompted again: %d confirmations", confirmer.calls)
	}
	launch := devicev1.StorefrontLaunchRequest{GameID: candidate.GameID, SourceGameID: candidate.SourceGameID, Provider: candidate.Provider, ProductID: candidate.ProductID}
	if _, err := accessA.Launch(context.Background(), launch); err != nil {
		t.Fatal(err)
	}
	if launcher.calls != 1 || launcher.provider != devicev1.StorefrontProviderSteam || launcher.productID != "12345" || observer.calls != 3 {
		t.Fatalf("launch did not revalidate exact evidence: launcher=%d observer=%d", launcher.calls, observer.calls)
	}
	accessB, err := NewLocalStorefrontAccess(bindingB, "http://server-b:8900", catalog)
	if err != nil {
		t.Fatal(err)
	}
	accessB.observer, accessB.launcher = observer, launcher
	if _, err := accessB.Launch(context.Background(), launch); err == nil {
		t.Fatal("binding B used binding A's grant")
	}
	if launcher.calls != 1 {
		t.Fatal("ungranted binding reached the storefront launcher")
	}
}
