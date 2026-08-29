package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestContentRepositoryFailsClosedAcrossProfiles(t *testing.T) {
	database := NewSQLiteDatabaseWithMigrationOptions(
		testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "content.sqlite")},
		core.MigrationOptions{BackupBeforeMigrate: false},
	)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	profiles := NewProfileRepository(database)
	profileA := &core.Profile{ID: "profile-content-a", DisplayName: "A", Role: core.ProfileRolePlayer, CreatedAt: now, UpdatedAt: now}
	profileB := &core.Profile{ID: "profile-content-b", DisplayName: "B", Role: core.ProfileRolePlayer, CreatedAt: now, UpdatedAt: now}
	for _, profile := range []*core.Profile{profileA, profileB} {
		if err := profiles.Create(context.Background(), profile); err != nil {
			t.Fatal(err)
		}
	}

	for _, fixture := range []struct {
		profileID, integrationID, copyID, canonicalID string
	}{
		{profileA.ID, "integration-content-a", "copy-content-a", "canonical-content-a"},
		{profileB.ID, "integration-content-b", "copy-content-b", "canonical-content-b"},
	} {
		if _, err := database.GetDB().Exec(`INSERT INTO integrations
			(id, profile_id, plugin_id, label, config_json, integration_type, created_at, updated_at)
			VALUES (?, ?, 'game-source-local', ?, '{}', 'source', ?, ?)`, fixture.integrationID, fixture.profileID, fixture.integrationID, now.Unix(), now.Unix()); err != nil {
			t.Fatal(err)
		}
		if _, err := database.GetDB().Exec(`INSERT INTO source_games
			(id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, root_path, status, created_at)
			VALUES (?, ?, ?, 'game-source-local', ?, 'Content Game', 'windows_pc', 'base_game', 'self_contained', ?, 'found', ?)`,
			fixture.copyID, fixture.profileID, fixture.integrationID, fixture.copyID, t.TempDir(), now.Unix()); err != nil {
			t.Fatal(err)
		}
		if _, err := database.GetDB().Exec(`INSERT INTO canonical_games(id, created_at) VALUES (?, ?)`, fixture.canonicalID, now.Unix()); err != nil {
			t.Fatal(err)
		}
		if _, err := database.GetDB().Exec(`INSERT INTO canonical_source_games_link(canonical_id, source_game_id) VALUES (?, ?)`, fixture.canonicalID, fixture.copyID); err != nil {
			t.Fatal(err)
		}
		if _, err := database.GetDB().Exec(`INSERT INTO game_files
			(source_game_id, path, file_name, role, file_kind, size, is_dir, object_id, revision, modified_at)
			VALUES (?, 'game/game.exe', 'game.exe', 'root', 'executable', 42, 0, '', 'revision-1', ?)`, fixture.copyID, now.Unix()); err != nil {
			t.Fatal(err)
		}
	}

	repository := NewContentRepository(database)
	ctxA := core.WithProfile(context.Background(), profileA)
	copy, err := repository.GetCopy(ctxA, "copy-content-a")
	if err != nil {
		t.Fatal(err)
	}
	if copy == nil || copy.CanonicalGameID != "canonical-content-a" || len(copy.SourceGame.Files) != 1 || copy.SourceGame.Files[0].Revision != "revision-1" {
		t.Fatalf("unexpected own copy: %+v", copy)
	}
	foreign, err := repository.GetCopy(ctxA, "copy-content-b")
	if err != nil {
		t.Fatal(err)
	}
	if foreign != nil {
		t.Fatalf("foreign copy leaked: %+v", foreign)
	}
	ownerless, err := repository.GetCopy(context.Background(), "copy-content-a")
	if err != nil {
		t.Fatal(err)
	}
	if ownerless != nil {
		t.Fatalf("ownerless copy leaked: %+v", ownerless)
	}

	if _, err := database.GetDB().Exec(`INSERT INTO canonical_games(id, created_at) VALUES ('canonical-content-ambiguous', ?)`, now.Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetDB().Exec(`INSERT INTO canonical_source_games_link(canonical_id, source_game_id)
		VALUES ('canonical-content-ambiguous', 'copy-content-a')`); err != nil {
		t.Fatal(err)
	}
	ambiguous, err := repository.GetCopy(ctxA, "copy-content-a")
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous != nil {
		t.Fatalf("ambiguous copy identity was served: %+v", ambiguous)
	}
}
