package v1

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStorefrontCandidateRequiresExactSteamAppID(t *testing.T) {
	valid := StorefrontProductCandidate{GameID: "game-1", SourceGameID: "source-1", Provider: StorefrontProviderSteam, ProductID: "12345", Title: "Game"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, productID := range []string{"", "steam:12345", "Game Title", "0123", "12345678901"} {
		candidate := valid
		candidate.ProductID = productID
		if err := candidate.Validate(); err == nil {
			t.Fatalf("invalid Steam App ID %q was accepted", productID)
		}
	}
}

func TestInventoryRefreshRequestRejectsDuplicateAndUnsupportedCandidates(t *testing.T) {
	candidate := StorefrontProductCandidate{GameID: "game-1", SourceGameID: "source-1", Provider: StorefrontProviderSteam, ProductID: "12345", Title: "Game"}
	if err := (InventoryRefreshRequest{StorefrontCandidates: []StorefrontProductCandidate{candidate}}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (InventoryRefreshRequest{StorefrontCandidates: []StorefrontProductCandidate{candidate, candidate}}).Validate(); err == nil {
		t.Fatal("duplicate storefront candidates were accepted")
	}
	candidate.Provider = "xbox"
	if err := (InventoryRefreshRequest{StorefrontCandidates: []StorefrontProductCandidate{candidate}}).Validate(); err == nil {
		t.Fatal("unsupported storefront provider was accepted")
	}
}

func TestStorefrontObservationRequiresCurrentInventorySchema(t *testing.T) {
	observation := StorefrontProductObservation{
		StorefrontProductCandidate: StorefrontProductCandidate{GameID: "game-1", SourceGameID: "source-1", Provider: StorefrontProviderSteam, ProductID: "12345", Title: "Game"},
		InstallPath:                filepath.Join(t.TempDir(), "Game"), ObservedAt: time.Now(),
	}
	inventory := DeviceInventory{SchemaVersion: InventorySchemaVersion, CapturedAt: time.Now(), StorefrontProducts: []StorefrontProductObservation{observation}}
	if err := inventory.Validate(); err != nil {
		t.Fatal(err)
	}
	inventory.SchemaVersion = InventorySchemaVersionPrevious
	if err := inventory.Validate(); err == nil {
		t.Fatal("schema 7 accepted storefront observations")
	}
}
