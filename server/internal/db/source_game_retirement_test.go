package db

import (
	"context"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// A library built from a filesystem connection accumulates records for games
// that were deleted from the drive years ago. Nothing used to remove them: a
// scan that did not see a record soft-deleted it and no code path ever deleted
// a soft-deleted record, so the count only grew. These tests pin the rule that
// drains it, and — more importantly — the two cases where it must not fire.

func driveBatch(integrationID string, sourceGames ...*core.SourceGame) *core.ScanBatch {
	return &core.ScanBatch{
		IntegrationID: integrationID,
		SourceGames:   sourceGames,
		// No include paths: the whole connection is in scope, so nothing is
		// hard-deleted for sitting outside it and the retirement rule is the
		// only thing under test.
		FilesystemScope: &core.FilesystemScanScope{PluginID: "game-source-google-drive"},
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
	}
}

func driveGame(integrationID, id, title string) *core.SourceGame {
	return &core.SourceGame{
		ID:            id,
		IntegrationID: integrationID,
		PluginID:      "game-source-google-drive",
		ExternalID:    id,
		RawTitle:      title,
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		Status:        "found",
		RootPath:      "Games/" + title,
	}
}

func sourceGameState(t *testing.T, ctx context.Context, db *sqliteDatabase, id string) (status string, missingCount int, present bool) {
	t.Helper()
	row := db.GetDB().QueryRowContext(ctx, `SELECT status, missing_scan_count FROM source_games WHERE id=?`, id)
	if err := row.Scan(&status, &missingCount); err != nil {
		return "", 0, false
	}
	return status, missingCount, true
}

func TestARecordAbsentFromThreeConsecutiveScansIsRetired(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	keeper := driveGame("integration-1", "scan:keeper", "Keeper")
	leaver := driveGame("integration-1", "scan:leaver", "Leaver")
	if err := store.PersistScanResults(ctx, driveBatch("integration-1", keeper, leaver)); err != nil {
		t.Fatal(err)
	}

	// Two scans without it: still there, so a wrong answer stays reversible.
	for scan := 1; scan <= 2; scan++ {
		if err := store.PersistScanResults(ctx, driveBatch("integration-1", keeper)); err != nil {
			t.Fatal(err)
		}
		status, count, present := sourceGameState(t, ctx, db, "scan:leaver")
		if !present {
			t.Fatalf("scan %d deleted the record; two misses must not be enough", scan)
		}
		if status != "not_found" {
			t.Fatalf("scan %d: status = %q, want not_found", scan, status)
		}
		if count != scan {
			t.Fatalf("scan %d: missing_scan_count = %d, want %d", scan, count, scan)
		}
	}

	// The third agrees, so the record goes.
	if err := store.PersistScanResults(ctx, driveBatch("integration-1", keeper)); err != nil {
		t.Fatal(err)
	}
	if _, _, present := sourceGameState(t, ctx, db, "scan:leaver"); present {
		t.Fatal("record survived three consecutive scans that did not return it")
	}
	if _, count, present := sourceGameState(t, ctx, db, "scan:keeper"); !present || count != 0 {
		t.Fatalf("keeper present=%v missing_scan_count=%d, want present with 0", present, count)
	}
}

func TestAReturningRecordStartsTheCountOver(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	keeper := driveGame("integration-1", "scan:keeper", "Keeper")
	flaky := driveGame("integration-1", "scan:flaky", "Flaky")
	if err := store.PersistScanResults(ctx, driveBatch("integration-1", keeper, flaky)); err != nil {
		t.Fatal(err)
	}

	// Absent twice, back once, absent twice again. Five scans, never three in a
	// row, so a drive that drops in and out never loses the game.
	for _, returned := range []bool{false, false, true, false, false} {
		batch := driveBatch("integration-1", keeper)
		if returned {
			batch = driveBatch("integration-1", keeper, flaky)
		}
		if err := store.PersistScanResults(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}

	status, count, present := sourceGameState(t, ctx, db, "scan:flaky")
	if !present {
		t.Fatal("a record that came back in between was retired anyway")
	}
	if status != "not_found" || count != 2 {
		t.Fatalf("status = %q, missing_scan_count = %d, want not_found and 2", status, count)
	}
}

func TestAConnectionThatReturnsNothingRetiresNothing(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	// Measured, not hypothetical: a signed-in Xbox connection answered two
	// consecutive scans with an empty list and no error, and every one of its
	// 99 games was soft-deleted. Soft-delete made that recoverable. Retirement
	// would not have been, so an empty answer must never advance the count.
	first := driveGame("integration-1", "scan:first", "First")
	second := driveGame("integration-1", "scan:second", "Second")
	if err := store.PersistScanResults(ctx, driveBatch("integration-1", first, second)); err != nil {
		t.Fatal(err)
	}

	for scan := 1; scan <= 4; scan++ {
		if err := store.PersistScanResults(ctx, driveBatch("integration-1")); err != nil {
			t.Fatal(err)
		}
	}

	for _, id := range []string{"scan:first", "scan:second"} {
		status, count, present := sourceGameState(t, ctx, db, id)
		if !present {
			t.Fatalf("%s was retired by a connection that returned nothing", id)
		}
		if status != "not_found" {
			t.Fatalf("%s status = %q, want not_found", id, status)
		}
		if count != 0 {
			t.Fatalf("%s missing_scan_count = %d, want 0: an empty answer proves nothing", id, count)
		}
	}
}

func TestAConnectionThatCannotProveAFileIsGoneRetiresNothing(t *testing.T) {
	ctx := context.Background()
	db, store := newTestGameStore(t)

	// A storefront reports what an account has, not what is on a disk, so it
	// carries no filesystem scope. Absence from its answer is a claim about
	// entitlement, not evidence that anything was deleted.
	storefrontBatch := func(sourceGames ...*core.SourceGame) *core.ScanBatch {
		return &core.ScanBatch{
			IntegrationID:   "integration-1",
			SourceGames:     sourceGames,
			ResolverMatches: map[string][]core.ResolverMatch{},
			MediaItems:      map[string][]core.MediaRef{},
		}
	}
	keeper := &core.SourceGame{
		ID: "scan:keeper", IntegrationID: "integration-1", PluginID: "game-source-xbox",
		ExternalID: "keeper", RawTitle: "Keeper", Platform: core.PlatformWindowsPC,
		Kind: core.GameKindBaseGame, GroupKind: core.GroupKindSelfContained, Status: "found",
	}
	dropped := &core.SourceGame{
		ID: "scan:dropped", IntegrationID: "integration-1", PluginID: "game-source-xbox",
		ExternalID: "dropped", RawTitle: "Dropped", Platform: core.PlatformWindowsPC,
		Kind: core.GameKindBaseGame, GroupKind: core.GroupKindSelfContained, Status: "found",
	}
	if err := store.PersistScanResults(ctx, storefrontBatch(keeper, dropped)); err != nil {
		t.Fatal(err)
	}

	for scan := 1; scan <= 5; scan++ {
		if err := store.PersistScanResults(ctx, storefrontBatch(keeper)); err != nil {
			t.Fatal(err)
		}
	}

	status, count, present := sourceGameState(t, ctx, db, "scan:dropped")
	if !present {
		t.Fatal("a storefront record was retired; only a filesystem connection can prove a file is gone")
	}
	if status != "not_found" || count != 0 {
		t.Fatalf("status = %q, missing_scan_count = %d, want not_found and 0", status, count)
	}
}
