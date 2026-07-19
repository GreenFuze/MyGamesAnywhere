package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestIntegrationAndSettingRepositoriesRejectForeignOwners(t *testing.T) {
	database := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "profile-repositories.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	profiles := NewProfileRepository(database)
	for _, id := range []string{"profile-a", "profile-b"} {
		if err := profiles.Create(context.Background(), &core.Profile{ID: id, DisplayName: id, Role: core.ProfileRolePlayer, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	ctxA := core.WithProfile(context.Background(), &core.Profile{ID: "profile-a"})
	ctxB := core.WithProfile(context.Background(), &core.Profile{ID: "profile-b"})
	integrations := NewIntegrationRepository(database)
	if _, err := integrations.List(context.Background()); !errors.Is(err, core.ErrProfileRequired) {
		t.Fatalf("unscoped integration list error = %v", err)
	}
	if _, err := integrations.GetByID(context.Background(), "anything"); !errors.Is(err, core.ErrProfileRequired) {
		t.Fatalf("unscoped integration get error = %v", err)
	}
	if err := integrations.Create(context.Background(), &core.Integration{ID: "unscoped"}); !errors.Is(err, core.ErrProfileRequired) {
		t.Fatalf("unscoped integration create error = %v", err)
	}
	if err := integrations.Delete(context.Background(), "anything"); !errors.Is(err, core.ErrProfileRequired) {
		t.Fatalf("unscoped integration delete error = %v", err)
	}
	foreign := &core.Integration{ID: "foreign", ProfileID: "profile-b", PluginID: "plugin", Label: "Foreign", CreatedAt: now, UpdatedAt: now}
	if err := integrations.Create(ctxA, foreign); !errors.Is(err, core.ErrProfileForbidden) {
		t.Fatalf("foreign create error = %v", err)
	}
	owned := &core.Integration{ID: "owned-a", PluginID: "plugin", Label: "Owned", CreatedAt: now, UpdatedAt: now}
	if err := integrations.Create(ctxA, owned); err != nil {
		t.Fatal(err)
	}
	if owned.ProfileID != "profile-a" {
		t.Fatalf("derived owner = %q", owned.ProfileID)
	}
	if got, err := integrations.GetByID(ctxB, owned.ID); err != nil || got != nil {
		t.Fatalf("foreign get = %+v, %v", got, err)
	}
	owned.ProfileID = "profile-b"
	if err := integrations.Update(ctxA, owned); !errors.Is(err, core.ErrProfileForbidden) {
		t.Fatalf("foreign transfer error = %v", err)
	}
	if err := integrations.Delete(ctxB, owned.ID); err != nil {
		t.Fatal(err)
	}
	owned.ProfileID = "profile-a"
	if got, err := integrations.GetByID(ctxA, owned.ID); err != nil || got == nil {
		t.Fatalf("foreign delete affected owner: %+v, %v", got, err)
	}

	settings := NewSettingRepository(database)
	if err := settings.Upsert(ctxA, &core.Setting{ProfileID: "profile-b", Key: "future_profile_preference", Value: "bad", UpdatedAt: now}); !errors.Is(err, core.ErrProfileForbidden) {
		t.Fatalf("foreign setting error = %v", err)
	}
	if err := settings.Upsert(ctxA, &core.Setting{ProfileID: "profile-a", Key: "future_profile_preference", Value: "a", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if got, err := settings.Get(ctxA, "future_profile_preference"); err != nil || got == nil || got.Value != "a" {
		t.Fatalf("owned setting = %+v, %v", got, err)
	}
	if got, err := settings.Get(ctxB, "future_profile_preference"); err != nil || got != nil {
		t.Fatalf("foreign setting = %+v, %v", got, err)
	}
}
