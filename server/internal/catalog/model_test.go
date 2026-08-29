package catalog

import (
	"strings"
	"testing"
	"time"
)

func TestObservationCommandNormalizesStableIdentityAndRejectsUnknownPolicy(t *testing.T) {
	observedAt := time.Date(2026, 8, 29, 9, 0, 0, 999, time.UTC)
	command := ObservationCommand{
		CanonicalGameID: " game-a ", Provider: " Steam ", SKU: " 123 ", Platform: " WINDOWS_PC ",
		Entitlement: EntitlementOwned, Delivery: DeliveryStorefront, Availability: AvailabilityAvailable,
		EvidenceSource: " scan ", EvidenceJSON: []byte(`{"ok":true}`), ObservedAt: observedAt,
		CurrentVersion: &PackageVersion{Version: " 1.0 ", Channel: " STABLE "},
	}
	command.Normalize()
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	if command.Provider != "steam" || command.Platform != "windows_pc" || command.Region != "global" || command.ObservedAt.Nanosecond() != 0 {
		t.Fatalf("normalized command=%+v", command)
	}
	if command.CurrentVersion == nil || command.CurrentVersion.Version != "1.0" || command.CurrentVersion.Channel != "stable" {
		t.Fatalf("normalized version=%+v", command.CurrentVersion)
	}
	if len(command.OfferKey()) != 64 || len(command.ObservationKey()) != 64 {
		t.Fatal("stable catalog keys must be SHA-256 hex")
	}

	command.Availability = Availability("invented")
	if err := command.Validate(); err == nil || !strings.Contains(err.Error(), "availability") {
		t.Fatalf("invalid availability error=%v", err)
	}
}

func TestPackageVersionRequiresStrongIdentityAndValidDigest(t *testing.T) {
	if err := (PackageVersion{}).Validate(); err == nil {
		t.Fatal("empty package identity should fail")
	}
	if err := (PackageVersion{Version: "1", SHA256: "bad"}).Validate(); err == nil {
		t.Fatal("invalid package digest should fail")
	}
	if err := (PackageVersion{Version: "1", SHA256: strings.Repeat("ab", 32)}).Validate(); err != nil {
		t.Fatalf("valid package identity: %v", err)
	}
}
