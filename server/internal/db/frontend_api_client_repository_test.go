package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/frontendauth"
)

func TestFrontendAPIClientRepositoryLifecycleAndAudit(t *testing.T) {
	database := NewSQLiteDatabaseWithMigrationOptions(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "clients.sqlite")}, core.MigrationOptions{BackupBeforeMigrate: false})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	profile := &core.Profile{ID: "profile-1", DisplayName: "Admin", Role: core.ProfileRoleAdminPlayer}
	if err := NewProfileRepository(database).Create(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	repository, err := NewFrontendAPIClientRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	client := frontendauth.Client{ID: "client-1", ProfileID: profile.ID, Name: "Playnite", SecretHash: "digest", Scopes: []frontendauth.Scope{frontendauth.ScopeCatalogRead}, CreatedAt: now, UpdatedAt: now}
	if err := repository.Create(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	items, err := repository.ListByProfile(context.Background(), profile.ID)
	if err != nil || len(items) != 1 || items[0].SecretHash != "digest" {
		t.Fatalf("list = %+v, %v", items, err)
	}
	if _, err := repository.Rotate(context.Background(), "other", client.ID, "new", now.Add(time.Minute)); err != frontendauth.ErrNotFound {
		t.Fatalf("cross-profile rotate = %v", err)
	}
	rotated, err := repository.Rotate(context.Background(), profile.ID, client.ID, "new", now.Add(time.Minute))
	if err != nil || rotated.SecretHash != "new" {
		t.Fatalf("rotate = %+v, %v", rotated, err)
	}
	if err := repository.RecordAudit(context.Background(), frontendauth.AuditEvent{ProfileID: profile.ID, ClientID: client.ID, Action: "rotate", Outcome: "success", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM frontend_api_client_audit WHERE client_id=?`, client.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit count = %d, %v", count, err)
	}
	if _, err := repository.Revoke(context.Background(), profile.ID, client.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
}
