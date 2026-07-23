package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/auth"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestCredentialTicketsRequireAdminAndRedeemOnce(t *testing.T) {
	database := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "auth-tickets.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	profiles := NewProfileRepository(database)
	now := time.Now().Truncate(time.Second)
	admin := &core.Profile{ID: "admin", DisplayName: "Admin", Role: core.ProfileRoleAdminPlayer, CreatedAt: now, UpdatedAt: now}
	player := &core.Profile{ID: "player", DisplayName: "Player", Role: core.ProfileRolePlayer, CreatedAt: now, UpdatedAt: now}
	for _, profile := range []*core.Profile{admin, player} {
		if err := profiles.Create(context.Background(), profile); err != nil {
			t.Fatal(err)
		}
	}
	store := NewAuthStore(database)
	service, err := auth.NewService(store, profiles)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.InitializeCredential(context.Background(), admin.ID, "admin-pass", auth.CredentialPassword); err != nil {
		t.Fatal(err)
	}
	if err := service.InitializeCredential(context.Background(), player.ID, "old-pin", auth.CredentialPassword); err != nil {
		t.Fatal(err)
	}
	_, adminSession, err := service.Login(context.Background(), admin.ID, "admin-pass")
	if err != nil {
		t.Fatal(err)
	}
	playerToken, playerSession, err := service.Login(context.Background(), player.ID, "old-pin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateCredentialTicket(context.Background(), playerSession, admin.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("non-admin ticket error = %v, want forbidden", err)
	}
	first, err := service.CreateCredentialTicket(context.Background(), adminSession, player.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateCredentialTicket(context.Background(), adminSession, player.ID)
	if err != nil {
		t.Fatal(err)
	}
	var persistedTokenHash string
	if err := database.GetDB().QueryRow(`SELECT token_hash FROM profile_credential_tickets WHERE id=?`, second.ID).Scan(&persistedTokenHash); err != nil {
		t.Fatal(err)
	}
	if persistedTokenHash == second.Token || persistedTokenHash == "" {
		t.Fatal("raw credential ticket was persisted")
	}
	if err := service.RedeemCredentialTicket(context.Background(), player.ID, first.Token, "1234", auth.CredentialPIN); !errors.Is(err, auth.ErrCredentialTicket) {
		t.Fatalf("replaced ticket error = %v", err)
	}
	if err := service.RedeemCredentialTicket(context.Background(), player.ID, second.Token, "1234", auth.CredentialPIN); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), playerToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("old session authentication error = %v", err)
	}
	if _, _, err := service.Login(context.Background(), player.ID, "1234"); err != nil {
		t.Fatalf("login with redeemed PIN: %v", err)
	}
	if err := service.RedeemCredentialTicket(context.Background(), player.ID, second.Token, "5678", auth.CredentialPIN); !errors.Is(err, auth.ErrCredentialTicket) {
		t.Fatalf("replayed ticket error = %v", err)
	}
	expired := auth.CredentialTicket{
		ID: "expired", ProfileID: player.ID, CreatedByProfileID: admin.ID,
		CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute),
	}
	if err := store.CreateCredentialTicket(context.Background(), expired, "expired-hash", expired.CreatedAt); err != nil {
		t.Fatal(err)
	}
	if err := store.RedeemCredentialTicket(context.Background(), "expired-hash", player.ID, now, auth.Credential{
		ProfileID: player.ID, Kind: auth.CredentialPIN, Hash: "unused", UpdatedAt: now,
	}); !errors.Is(err, auth.ErrCredentialTicket) {
		t.Fatalf("expired ticket error = %v", err)
	}
	active, err := service.ActiveCredentialTicket(context.Background(), adminSession, player.ID)
	if err != nil || active != nil {
		t.Fatalf("active ticket after redemption = %+v, %v", active, err)
	}
}
