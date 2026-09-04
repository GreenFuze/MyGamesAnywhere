package scan

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/catalog"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type recordingObserver struct {
	commands []catalog.ObservationCommand
	err      error
}

func (r *recordingObserver) Observe(_ context.Context, command catalog.ObservationCommand) (*catalog.Offer, error) {
	r.commands = append(r.commands, command)
	if r.err != nil {
		return nil, r.err
	}
	return &catalog.Offer{}, nil
}

func gamePassOffer() StorefrontOffer {
	return StorefrontOffer{
		ExternalID:           "title-1",
		SKU:                  "9PABCDEF",
		Platform:             "windows_pc",
		Entitlement:          "subscription",
		Availability:         "available",
		LastPlayedAt:         "2026-08-14T09:30:00Z",
		AchievementsUnlocked: 12,
		GamerscoreEarned:     240,
		IsGamePass:           true,
	}
}

func canonicalWithSource(sourceGame *core.SourceGame) []*core.CanonicalGame {
	return []*core.CanonicalGame{{ID: "canonical-1", SourceGames: []*core.SourceGame{sourceGame}}}
}

func xboxSourceGame() *core.SourceGame {
	return &core.SourceGame{
		ID:            "sg-1",
		IntegrationID: "integration-1",
		PluginID:      "game-source-xbox",
		ExternalID:    "title-1",
		Platform:      core.Platform("windows_pc"),
	}
}

func TestObservationsAreRecordedForEachDescribedTitle(t *testing.T) {
	observer := &recordingObserver{}
	orchestrator := &Orchestrator{logger: testLogger{}, catalogObserver: observer}

	orchestrator.recordCatalogObservations(
		context.Background(),
		canonicalWithSource(xboxSourceGame()),
		map[string]map[string]StorefrontOffer{"integration-1": {"title-1": gamePassOffer()}},
	)

	if len(observer.commands) != 1 {
		t.Fatalf("expected one observation, got %d", len(observer.commands))
	}
	command := observer.commands[0]
	if command.CanonicalGameID != "canonical-1" || command.SourceGameID != "sg-1" {
		t.Fatalf("unexpected identity: %+v", command)
	}
	if command.Entitlement != catalog.EntitlementSubscription {
		t.Fatalf("entitlement = %q", command.Entitlement)
	}
	if command.Availability != catalog.AvailabilityAvailable {
		t.Fatalf("availability = %q", command.Availability)
	}
	if command.Delivery != catalog.DeliveryStorefront {
		t.Fatalf("delivery = %q", command.Delivery)
	}
	if command.SKU != "9PABCDEF" || command.Provider != "game-source-xbox" {
		t.Fatalf("unexpected offer key parts: %+v", command)
	}

	// The evidence must record why MGA concluded what it did.
	var evidence catalogEvidence
	if err := json.Unmarshal(command.EvidenceJSON, &evidence); err != nil {
		t.Fatalf("evidence is not valid JSON: %v", err)
	}
	if !evidence.IsGamePass || !evidence.Played || evidence.GamerscoreEarned != 240 {
		t.Fatalf("evidence lost the reasoning: %+v", evidence)
	}
}

func TestAnUnknownClaimIsNeverUpgradedToOwnership(t *testing.T) {
	// A provider that says nothing, or something this server does not
	// understand, leaves MGA not knowing. Rendering that as ownership would be
	// asserting something it never observed.
	for _, claimed := range []string{"", "  ", "purchased", "definitely-owned"} {
		if got := entitlementOrUnknown(claimed); got != catalog.EntitlementUnknown {
			t.Fatalf("entitlementOrUnknown(%q) = %q, want unknown", claimed, got)
		}
	}
	if got := entitlementOrUnknown("SUBSCRIPTION"); got != catalog.EntitlementSubscription {
		t.Fatalf("a recognised claim should survive casing, got %q", got)
	}
}

func TestSilenceIsNotTreatedAsRemoval(t *testing.T) {
	// "unknown" and "unavailable" must stay distinct: a provider going quiet
	// must not make MGA announce that a game was removed.
	for _, claimed := range []string{"", "gone", "maybe"} {
		if got := availabilityOrUnknown(claimed); got != catalog.AvailabilityUnknown {
			t.Fatalf("availabilityOrUnknown(%q) = %q, want unknown", claimed, got)
		}
	}
	if got := availabilityOrUnknown("unavailable"); got != catalog.AvailabilityUnavailable {
		t.Fatalf("an explicit removal must be preserved, got %q", got)
	}
}

func TestPlayedIsEvidenceBasedNotPresenceBased(t *testing.T) {
	// Appearing in a play-history listing is not the same as having played it;
	// the evidence is a timestamp, an achievement, or gamerscore.
	if (StorefrontOffer{}).Played() {
		t.Fatal("an offer with no engagement evidence must not count as played")
	}
	for _, offer := range []StorefrontOffer{
		{LastPlayedAt: "2026-08-14T09:30:00Z"},
		{AchievementsUnlocked: 1},
		{GamerscoreEarned: 10},
	} {
		if !offer.Played() {
			t.Fatalf("expected played for %+v", offer)
		}
	}
	if (StorefrontOffer{LastPlayedAt: "   "}).Played() {
		t.Fatal("whitespace is not a timestamp")
	}
}

func TestSourceGamesWithoutAProviderReportAreLeftAlone(t *testing.T) {
	// A filesystem source says nothing about entitlement. It must not acquire a
	// fabricated offer just because a scan ran.
	observer := &recordingObserver{}
	orchestrator := &Orchestrator{logger: testLogger{}, catalogObserver: observer}

	local := &core.SourceGame{
		ID: "sg-local", IntegrationID: "integration-2",
		PluginID: "game-source-local", ExternalID: "scan:abc",
	}
	orchestrator.recordCatalogObservations(
		context.Background(),
		canonicalWithSource(local),
		map[string]map[string]StorefrontOffer{"integration-1": {"title-1": gamePassOffer()}},
	)

	if len(observer.commands) != 0 {
		t.Fatalf("expected no observations, got %+v", observer.commands)
	}
}

func TestAFailedObservationDoesNotFailTheScan(t *testing.T) {
	// History is worth having, not worth losing a completed scan over.
	observer := &recordingObserver{err: context.DeadlineExceeded}
	orchestrator := &Orchestrator{logger: testLogger{}, catalogObserver: observer}

	orchestrator.recordCatalogObservations(
		context.Background(),
		canonicalWithSource(xboxSourceGame()),
		map[string]map[string]StorefrontOffer{"integration-1": {"title-1": gamePassOffer()}},
	)

	if len(observer.commands) != 1 {
		t.Fatalf("expected the observation to be attempted, got %d", len(observer.commands))
	}
}

func TestNoObserverMeansNoChangeInBehaviour(t *testing.T) {
	orchestrator := &Orchestrator{logger: testLogger{}}
	orchestrator.recordCatalogObservations(
		context.Background(),
		canonicalWithSource(xboxSourceGame()),
		map[string]map[string]StorefrontOffer{"integration-1": {"title-1": gamePassOffer()}},
	)
}

func TestSKUFallsBackToTheExternalIDWhenThereIsNoStoreProduct(t *testing.T) {
	// Without a SKU the observation would fail validation and the title would
	// silently drop out of the catalog.
	offer := gamePassOffer()
	offer.SKU = ""
	command, err := buildObservationCommand(
		&core.CanonicalGame{ID: "canonical-1"},
		xboxSourceGame(),
		offer,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("build observation: %v", err)
	}
	if command.SKU != "title-1" {
		t.Fatalf("sku = %q, want the external id", command.SKU)
	}
}

func TestAnObservationWithoutACanonicalGameIsRefused(t *testing.T) {
	_, err := buildObservationCommand(&core.CanonicalGame{}, xboxSourceGame(), gamePassOffer(), time.Now())
	if err == nil {
		t.Fatal("expected an offer with no canonical game to be refused")
	}
}
