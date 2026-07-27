package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/savecompat"
)

func TestSaveCompatibilityOverridesRequireReviewConflictAndRevokeWithoutDeletingHistory(t *testing.T) {
	database := NewSQLiteDatabaseWithMigrationOptions(
		testLogger{},
		testDBConfig{dbPath: filepath.Join(t.TempDir(), "mga.sqlite")},
		core.MigrationOptions{BackupBeforeMigrate: false},
	)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	for _, profile := range []struct {
		id, name, role string
	}{
		{"profile-owner", "Player", "player"},
		{"profile-other", "Other Player", "player"},
		{"profile-admin", "Admin", "admin_player"},
	} {
		if _, err := database.GetDB().Exec(`INSERT INTO profiles(id, display_name, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)`, profile.id, profile.name, profile.role, now.Unix(), now.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.GetDB().Exec(`INSERT INTO save_domain_versions
		(id, profile_id, domain_id, canonical_game_id, source_game_id, runtime, slot_id, integration_id,
		manifest_hash, origin_label, route_label, accepted_at, file_count, total_size, payload_key, created_at)
		VALUES ('history-1','profile-owner','save:source','game','source','scummvm','autosave','sync',
		?,'Player PC','ScummVM',?,1,12,'payload-1',?)`,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now.Unix(), now.Unix()); err != nil {
		t.Fatal(err)
	}

	repository := NewSaveCompatibilityOverrideRepository(database)
	scope := savecompat.OverrideScope{
		OwnerProfileID: "profile-owner",
		SourceDomainID: "save:source",
		TargetDomainID: "save:target",
		Source:         savecompat.FormatRef{ID: "scummvm:game", Version: "1"},
		Target:         savecompat.FormatRef{ID: "native:game", Version: "2"},
	}
	first := pendingOverride("override-a", scope, false, now)
	if err := repository.CreateOverride(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if got, err := repository.FindApprovedOverride(context.Background(), scope); err != nil || got != nil {
		t.Fatalf("pending override was usable: got=%+v err=%v", got, err)
	}
	approved, err := repository.ApproveOverride(context.Background(), first.ID, "profile-admin", now.Add(time.Minute), false)
	if err != nil || approved == nil || approved.State != savecompat.OverrideStateApproved {
		t.Fatalf("approved=%+v err=%v", approved, err)
	}

	reverse := scope
	reverse.SourceDomainID, reverse.TargetDomainID = scope.TargetDomainID, scope.SourceDomainID
	reverse.Source, reverse.Target = scope.Target, scope.Source
	if got, err := repository.FindApprovedOverride(context.Background(), reverse); err != nil || got != nil {
		t.Fatalf("direction broadened: got=%+v err=%v", got, err)
	}

	second := pendingOverride("override-b", scope, true, now.Add(2*time.Minute))
	if err := repository.CreateOverride(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	conflicted, err := repository.ApproveOverride(context.Background(), second.ID, "profile-admin", now.Add(3*time.Minute), false)
	if !errors.Is(err, savecompat.ErrOverrideConflict) || conflicted == nil || conflicted.State != savecompat.OverrideStateConflict {
		t.Fatalf("conflicted=%+v err=%v", conflicted, err)
	}
	if got, findErr := repository.FindApprovedOverride(context.Background(), scope); findErr != nil || got != nil {
		t.Fatalf("conflicted scope remained usable: got=%+v err=%v", got, findErr)
	}

	resolved, err := repository.ApproveOverride(context.Background(), second.ID, "profile-admin", now.Add(4*time.Minute), true)
	if err != nil || resolved == nil || resolved.State != savecompat.OverrideStateApproved {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	old, err := repository.GetOverride(context.Background(), first.ID)
	if err != nil || old == nil || old.State != savecompat.OverrideStateRevoked {
		t.Fatalf("old override=%+v err=%v", old, err)
	}
	if _, err := repository.RevokeOverride(context.Background(), second.ID, "profile-other", now.Add(5*time.Minute), false); err == nil {
		t.Fatal("unrelated player revoked another profile's override")
	}

	revoked, err := repository.RevokeOverride(context.Background(), second.ID, "profile-owner", now.Add(6*time.Minute), false)
	if err != nil || revoked == nil || revoked.State != savecompat.OverrideStateRevoked {
		t.Fatalf("revoked=%+v err=%v", revoked, err)
	}
	if got, findErr := repository.FindApprovedOverride(context.Background(), scope); findErr != nil || got != nil {
		t.Fatalf("revoked override remained usable: got=%+v err=%v", got, findErr)
	}
	var historyRows int
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM save_domain_versions WHERE id='history-1'`).Scan(&historyRows); err != nil || historyRows != 1 {
		t.Fatalf("save history changed during override revocation: rows=%d err=%v", historyRows, err)
	}
}

func pendingOverride(id string, scope savecompat.OverrideScope, reversible bool, now time.Time) savecompat.CompatibilityOverride {
	return savecompat.CompatibilityOverride{
		ID:              id,
		Scope:           scope,
		Relationship:    savecompat.RelationshipSameFormat,
		Reversible:      reversible,
		Origin:          savecompat.OverrideOriginCommunity,
		Attribution:     "Community fixture",
		EvidenceSource:  "fixture-list",
		EvidenceVersion: "1",
		EvidenceJSON:    `{"source_release":"edition-a","target_release":"edition-b"}`,
		State:           savecompat.OverrideStatePending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
