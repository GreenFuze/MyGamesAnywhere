package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/catalog"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type catalogTestFixture struct {
	database core.Database
	service  *catalog.Service
	ctxA     context.Context
	ctxB     context.Context
	baseTime time.Time
}

func newCatalogTestFixture(t *testing.T) *catalogTestFixture {
	t.Helper()
	database := NewSQLiteDatabaseWithMigrationOptions(
		testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "catalog.sqlite")},
		core.MigrationOptions{BackupBeforeMigrate: false},
	)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	baseTime := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	profiles := NewProfileRepository(database)
	profileA := &core.Profile{ID: "profile-a", DisplayName: "A", Role: core.ProfileRolePlayer, CreatedAt: baseTime, UpdatedAt: baseTime}
	profileB := &core.Profile{ID: "profile-b", DisplayName: "B", Role: core.ProfileRolePlayer, CreatedAt: baseTime, UpdatedAt: baseTime}
	for _, profile := range []*core.Profile{profileA, profileB} {
		if err := profiles.Create(context.Background(), profile); err != nil {
			t.Fatal(err)
		}
	}
	ctxA := core.WithProfile(context.Background(), profileA)
	ctxB := core.WithProfile(context.Background(), profileB)
	integrations := NewIntegrationRepository(database)
	for _, item := range []struct {
		ctx context.Context
		id  string
	}{
		{ctxA, "integration-a"},
		{ctxB, "integration-b"},
	} {
		if err := integrations.Create(item.ctx, &core.Integration{
			ID: item.id, PluginID: "game-source-steam", Label: item.id, ConfigJSON: `{}`,
			IntegrationType: "source", CreatedAt: baseTime, UpdatedAt: baseTime,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct {
		profileID, integrationID, sourceID, canonicalID string
	}{
		{"profile-a", "integration-a", "source-a", "game-a"},
		{"profile-b", "integration-b", "source-b", "game-b"},
	} {
		if _, err := database.GetDB().Exec(`INSERT INTO source_games
			(id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, created_at)
			VALUES (?, ?, ?, 'game-source-steam', ?, 'Game', 'windows_pc', 'base_game', 'unknown', 'found', ?)`,
			item.sourceID, item.profileID, item.integrationID, item.sourceID, baseTime.Unix()); err != nil {
			t.Fatal(err)
		}
		if _, err := database.GetDB().Exec(`INSERT INTO canonical_games(id, created_at) VALUES (?,?)`, item.canonicalID, baseTime.Unix()); err != nil {
			t.Fatal(err)
		}
		if _, err := database.GetDB().Exec(`INSERT INTO canonical_source_games_link(canonical_id, source_game_id) VALUES (?,?)`, item.canonicalID, item.sourceID); err != nil {
			t.Fatal(err)
		}
	}
	service, err := catalog.NewService(NewCatalogRepository(database))
	if err != nil {
		t.Fatal(err)
	}
	return &catalogTestFixture{database: database, service: service, ctxA: ctxA, ctxB: ctxB, baseTime: baseTime}
}

func (f *catalogTestFixture) observation(at time.Time, availability catalog.Availability, current, latest string) catalog.ObservationCommand {
	command := catalog.ObservationCommand{
		CanonicalGameID: "game-a", SourceGameID: "source-a", IntegrationID: "integration-a",
		Provider: "steam", SKU: "12345", Platform: "windows_pc", Region: "global",
		Entitlement: catalog.EntitlementOwned, Delivery: catalog.DeliveryStorefront, Availability: availability,
		EvidenceSource: "steam-library-scan", EvidenceJSON: []byte(`{"account":"fixture"}`), ObservedAt: at,
	}
	if current != "" {
		command.CurrentVersion = &catalog.PackageVersion{Version: current, Channel: "stable"}
	}
	if latest != "" {
		command.LatestVersion = &catalog.PackageVersion{Version: latest, Channel: "stable"}
	}
	return command
}

func TestCatalogObservationIsIdempotentAndHistoryTracksStateChanges(t *testing.T) {
	fixture := newCatalogTestFixture(t)
	first := fixture.observation(fixture.baseTime.Add(time.Minute), catalog.AvailabilityAvailable, "1.0", "1.1")
	offer, err := fixture.service.Observe(fixture.ctxA, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Observe(fixture.ctxA, first); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	if _, err := fixture.service.Observe(fixture.ctxA, fixture.observation(fixture.baseTime.Add(2*time.Minute), catalog.AvailabilityLeavingSoon, "1.0", "1.1")); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Observe(fixture.ctxA, fixture.observation(fixture.baseTime.Add(3*time.Minute), catalog.AvailabilityUnavailable, "1.0", "1.1")); err != nil {
		t.Fatal(err)
	}
	final, err := fixture.service.Observe(fixture.ctxA, fixture.observation(fixture.baseTime.Add(4*time.Minute), catalog.AvailabilityAvailable, "1.1", "1.2"))
	if err != nil {
		t.Fatal(err)
	}
	if final.ID != offer.ID || final.Availability != catalog.AvailabilityAvailable || final.CurrentVersion == nil || final.CurrentVersion.Version != "1.1" || final.LatestVersion == nil || final.LatestVersion.Version != "1.2" {
		t.Fatalf("final offer = %+v", final)
	}

	history, err := fixture.service.ListHistory(fixture.ctxA, offer.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	wantCounts := map[catalog.EventType]int{
		catalog.EventAdded: 1, catalog.EventLeavingSoon: 1, catalog.EventRemoved: 1,
		catalog.EventReturned: 1, catalog.EventVersionChanged: 1,
	}
	gotCounts := map[catalog.EventType]int{}
	for _, event := range history {
		gotCounts[event.Type]++
	}
	for eventType, want := range wantCounts {
		if gotCounts[eventType] != want {
			t.Fatalf("history count for %s = %d, want %d; history=%+v", eventType, gotCounts[eventType], want, history)
		}
	}
	if len(history) == 0 || history[len(history)-1].Type != catalog.EventAdded {
		t.Fatalf("history is not newest-first with the initial add last: %+v", history)
	}
	var observations, versions int
	if err := fixture.database.GetDB().QueryRow(`SELECT COUNT(*) FROM catalog_offer_observations WHERE offer_id=?`, offer.ID).Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.GetDB().QueryRow(`SELECT COUNT(*) FROM catalog_package_versions WHERE offer_id=?`, offer.ID).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if observations != 4 || versions != 3 {
		t.Fatalf("observations=%d versions=%d, want 4/3", observations, versions)
	}

	listed, err := fixture.service.ListOffers(fixture.ctxA, catalog.OfferFilter{Provider: "STEAM", Availability: catalog.AvailabilityAvailable})
	if err != nil || len(listed) != 1 || listed[0].ID != offer.ID {
		t.Fatalf("filtered offers=%+v err=%v", listed, err)
	}
}

func TestCatalogPackageEvidenceConflictFailsFastOnIdempotentObservation(t *testing.T) {
	fixture := newCatalogTestFixture(t)
	command := fixture.observation(fixture.baseTime.Add(time.Minute), catalog.AvailabilityAvailable, "1.0", "1.0")
	command.CurrentVersion.SizeBytes = 100
	command.LatestVersion.SizeBytes = 100
	if _, err := fixture.service.Observe(fixture.ctxA, command); err != nil {
		t.Fatal(err)
	}
	command.CurrentVersion.SizeBytes = 200
	command.LatestVersion.SizeBytes = 200
	if _, err := fixture.service.Observe(fixture.ctxA, command); err == nil {
		t.Fatal("expected conflicting package size evidence to fail")
	}
	var size int64
	if err := fixture.database.GetDB().QueryRow(`SELECT size_bytes FROM catalog_package_versions`).Scan(&size); err != nil {
		t.Fatal(err)
	}
	if size != 100 {
		t.Fatalf("conflicting retry changed package size to %d", size)
	}
}

func TestCatalogRefreshFailureMarksStaleWithoutInventingUnavailable(t *testing.T) {
	fixture := newCatalogTestFixture(t)
	offer, err := fixture.service.Observe(fixture.ctxA, fixture.observation(fixture.baseTime.Add(time.Minute), catalog.AvailabilityAvailable, "1.0", "1.0"))
	if err != nil {
		t.Fatal(err)
	}
	failureAt := fixture.baseTime.Add(2 * time.Minute)
	if err := fixture.service.MarkRefreshFailed(fixture.ctxA, catalog.RefreshFailure{
		RefreshScope: catalog.RefreshScope{Provider: "steam", IntegrationID: "integration-a", AttemptedAt: failureAt},
		Error:        "provider timeout",
	}); err != nil {
		t.Fatal(err)
	}
	stale, err := fixture.service.GetOffer(fixture.ctxA, offer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Stale() || !stale.StaleAt.Equal(failureAt) || stale.Availability != catalog.AvailabilityAvailable {
		t.Fatalf("stale offer = %+v", stale)
	}
	var observations, unavailable int
	if err := fixture.database.GetDB().QueryRow(`SELECT COUNT(*), SUM(CASE WHEN availability='unavailable' THEN 1 ELSE 0 END)
		FROM catalog_offer_observations WHERE offer_id=?`, offer.ID).Scan(&observations, &unavailable); err != nil {
		t.Fatal(err)
	}
	if observations != 1 || unavailable != 0 {
		t.Fatalf("failure synthesized state: observations=%d unavailable=%d", observations, unavailable)
	}
	if err := fixture.service.MarkRefreshSucceeded(fixture.ctxA, catalog.RefreshScope{
		Provider: "steam", IntegrationID: "integration-a", AttemptedAt: fixture.baseTime.Add(3 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	refreshed, err := fixture.service.GetOffer(fixture.ctxA, offer.ID)
	if err != nil || refreshed.Stale() || refreshed.Availability != catalog.AvailabilityAvailable {
		t.Fatalf("refreshed offer=%+v err=%v", refreshed, err)
	}
}

func TestCatalogAccessFailsClosedAcrossProfiles(t *testing.T) {
	fixture := newCatalogTestFixture(t)
	offer, err := fixture.service.Observe(fixture.ctxA, fixture.observation(fixture.baseTime.Add(time.Minute), catalog.AvailabilityAvailable, "1.0", "1.0"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.GetOffer(fixture.ctxB, offer.ID); !errors.Is(err, catalog.ErrOfferNotFound) {
		t.Fatalf("foreign GetOffer error = %v", err)
	}
	if _, err := fixture.service.ListHistory(fixture.ctxB, offer.ID, 10); !errors.Is(err, catalog.ErrOfferNotFound) {
		t.Fatalf("foreign ListHistory error = %v", err)
	}
	listed, err := fixture.service.ListOffers(fixture.ctxB, catalog.OfferFilter{})
	if err != nil || len(listed) != 0 {
		t.Fatalf("foreign ListOffers=%+v err=%v", listed, err)
	}
	if _, err := fixture.service.Observe(fixture.ctxB, fixture.observation(fixture.baseTime.Add(2*time.Minute), catalog.AvailabilityAvailable, "1.0", "1.0")); !errors.Is(err, catalog.ErrCatalogIdentityNotVisible) {
		t.Fatalf("foreign Observe error = %v", err)
	}
	if err := fixture.service.MarkRefreshFailed(fixture.ctxB, catalog.RefreshFailure{
		RefreshScope: catalog.RefreshScope{Provider: "steam", IntegrationID: "integration-a", AttemptedAt: fixture.baseTime.Add(2 * time.Minute)},
		Error:        "failure",
	}); !errors.Is(err, catalog.ErrCatalogIdentityNotVisible) {
		t.Fatalf("foreign refresh error = %v", err)
	}
	if _, err := fixture.service.Observe(context.Background(), fixture.observation(fixture.baseTime.Add(2*time.Minute), catalog.AvailabilityAvailable, "1.0", "1.0")); !errors.Is(err, catalog.ErrProfileRequired) {
		t.Fatalf("unscoped Observe error = %v", err)
	}
}

func TestCatalogRejectsOutOfOrderObservationWithoutChangingProjection(t *testing.T) {
	fixture := newCatalogTestFixture(t)
	newer := fixture.observation(fixture.baseTime.Add(2*time.Minute), catalog.AvailabilityAvailable, "2.0", "2.0")
	offer, err := fixture.service.Observe(fixture.ctxA, newer)
	if err != nil {
		t.Fatal(err)
	}
	older := fixture.observation(fixture.baseTime.Add(time.Minute), catalog.AvailabilityUnavailable, "1.0", "1.0")
	if _, err := fixture.service.Observe(fixture.ctxA, older); err == nil {
		t.Fatal("expected out-of-order observation to fail")
	}
	got, err := fixture.service.GetOffer(fixture.ctxA, offer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Availability != catalog.AvailabilityAvailable || got.CurrentVersion == nil || got.CurrentVersion.Version != "2.0" {
		t.Fatalf("out-of-order observation changed projection: %+v", got)
	}
	var observations int
	if err := fixture.database.GetDB().QueryRow(`SELECT COUNT(*) FROM catalog_offer_observations WHERE offer_id=?`, offer.ID).Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if observations != 1 {
		t.Fatalf("observations=%d, want 1", observations)
	}
}
