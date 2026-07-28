package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestSourceMoveStoreScopesJobsByProfileAndPreservesInterruptedPhase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	database := NewSQLiteDatabaseWithMigrationOptions(
		testLogger{},
		testDBConfig{dbPath: dbPath},
		core.MigrationOptions{BackupBeforeMigrate: false},
	)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	for _, id := range []string{"profile-a", "profile-b"} {
		if _, err := database.GetDB().Exec(`INSERT INTO profiles(id, display_name, role, created_at, updated_at)
			VALUES (?, ?, 'player', ?, ?)`, id, id, now, now); err != nil {
			t.Fatal(err)
		}
	}

	store := NewSourceMoveStore(database)
	ctxA := core.WithProfile(context.Background(), &core.Profile{ID: "profile-a"})
	ctxB := core.WithProfile(context.Background(), &core.Profile{ID: "profile-b"})
	job := sourceMoveStoreFixture("job-a", "transfer-a")
	job.Status = core.SourceMoveStatusDeletingSource
	if err := store.CreateJob(ctxA, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if visible, err := store.GetJob(ctxB, job.ID); err != nil || visible != nil {
		t.Fatalf("other profile GetJob() = %#v, %v", visible, err)
	}
	if err := store.MarkInFlightJobsInterrupted(context.Background()); err != nil {
		t.Fatalf("MarkInFlightJobsInterrupted() error = %v", err)
	}
	interrupted, err := store.GetJob(ctxA, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.Status != core.SourceMoveStatusInterrupted ||
		interrupted.RecoveryPhase != core.SourceMoveStatusDeletingSource ||
		!strings.Contains(interrupted.Message, "restart") {
		t.Fatalf("interrupted job = %#v", interrupted)
	}
}

func TestSourceMoveStoreReservesActiveDestinationUntilJobFinishes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mga.sqlite")
	database := NewSQLiteDatabaseWithMigrationOptions(
		testLogger{},
		testDBConfig{dbPath: dbPath},
		core.MigrationOptions{BackupBeforeMigrate: false},
	)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	for _, profileID := range []string{"profile-a", "profile-b"} {
		if _, err := database.GetDB().Exec(`INSERT INTO profiles(id, display_name, role, created_at, updated_at)
			VALUES (?, ?, 'player', ?, ?)`, profileID, profileID, now, now); err != nil {
			t.Fatal(err)
		}
	}
	store := NewSourceMoveStore(database)
	ctx := core.WithProfile(context.Background(), &core.Profile{ID: "profile-a"})
	ctxB := core.WithProfile(context.Background(), &core.Profile{ID: "profile-b"})
	first := sourceMoveStoreFixture("job-a", "transfer-a")
	if err := store.CreateJob(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := sourceMoveStoreFixture("job-b", "transfer-b")
	second.DestinationIntegrationID = "another-profile-connection"
	second.DestinationPath = "games/GAME"
	if err := store.CreateJob(ctxB, second); err == nil {
		t.Fatal("CreateJob() allowed a cross-profile, case-insensitive backing destination collision")
	}
	finished := time.Now().UTC()
	first.Status = core.SourceMoveStatusCompleted
	first.FinishedAt = &finished
	if err := store.UpdateJob(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateJob(ctxB, second); err != nil {
		t.Fatalf("CreateJob() after completion error = %v", err)
	}
}

func sourceMoveStoreFixture(id, transferID string) *core.SourceMoveJob {
	return &core.SourceMoveJob{
		ID: id, TransferID: transferID,
		CanonicalGameID: "game", CanonicalTitle: "Game",
		SourceGameID: "source", SourceTitle: "Game",
		SourceIntegrationID: "source-integration", SourcePluginID: "game-source-smb",
		SourceRootPath:           "Library/Game",
		DestinationIntegrationID: "destination-integration",
		DestinationPluginID:      "game-source-google-drive",
		DestinationAuthority:     "gdrive:test",
		DestinationLabel:         "Drive", DestinationPath: "Games/Game",
		Status: core.SourceMoveStatusQueued, ProgressTotal: 1,
		Files: []core.SourceMoveFile{{
			Ordinal: 0, SourcePath: "Library/Game/game.bin", RelativePath: "game.bin",
			Size: 7, Status: "pending",
		}},
	}
}
