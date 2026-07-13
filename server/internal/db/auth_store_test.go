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

func TestProfileAuthenticationBootstrapAndCredentialChange(t *testing.T) {
	t.Parallel()

	database := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "auth.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	profiles := NewProfileRepository(database)
	profile := &core.Profile{ID: "admin-1", DisplayName: "Admin", Role: core.ProfileRoleAdminPlayer, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := profiles.Create(context.Background(), profile); err != nil {
		t.Fatalf("Create(profile) error = %v", err)
	}
	service, err := auth.NewService(NewAuthStore(database), profiles)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := service.EnsureBootstrapCredential(context.Background()); err != nil {
		t.Fatalf("EnsureBootstrapCredential() error = %v", err)
	}
	token, session, err := service.Login(context.Background(), profile.ID, auth.BootstrapPassword)
	if err != nil {
		t.Fatalf("Login(bootstrap) error = %v", err)
	}
	if token == "" || !session.MustChange {
		t.Fatalf("bootstrap session = %+v, token empty = %v", session, token == "")
	}
	if err := service.RequireDeviceAuthority(session, profile.ID); !errors.Is(err, auth.ErrCredentialChange) {
		t.Fatalf("RequireDeviceAuthority() error = %v, want ErrCredentialChange", err)
	}
	newToken, nextSession, err := service.ChangeOwnCredential(context.Background(), session, auth.BootstrapPassword, "246810", auth.CredentialPIN)
	if err != nil {
		t.Fatalf("ChangeOwnCredential() error = %v", err)
	}
	if newToken == "" || nextSession.MustChange {
		t.Fatalf("changed session = %+v, token empty = %v", nextSession, newToken == "")
	}
	if err := service.RequireDeviceAuthority(nextSession, profile.ID); err != nil {
		t.Fatalf("RequireDeviceAuthority() error = %v", err)
	}
	if _, _, err := service.Login(context.Background(), profile.ID, auth.BootstrapPassword); !errors.Is(err, auth.ErrInvalidCredential) {
		t.Fatalf("Login(old credential) error = %v, want ErrInvalidCredential", err)
	}
	if _, _, err := service.Login(context.Background(), profile.ID, "246810"); err != nil {
		t.Fatalf("Login(new PIN) error = %v", err)
	}
	if err := service.ResetCredentialToBootstrap(context.Background(), profile.ID); err != nil {
		t.Fatalf("ResetCredentialToBootstrap() error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), newToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("Authenticate(pre-recovery session) error = %v, want ErrUnauthenticated", err)
	}
	_, recoveredSession, err := service.Login(context.Background(), profile.ID, auth.BootstrapPassword)
	if err != nil {
		t.Fatalf("Login(recovered bootstrap) error = %v", err)
	}
	if !recoveredSession.MustChange {
		t.Fatalf("recovered session = %+v, want MustChange", recoveredSession)
	}
	if err := service.ResetCredentialToBootstrap(context.Background(), "missing-profile"); !errors.Is(err, auth.ErrProfileNotFound) {
		t.Fatalf("ResetCredentialToBootstrap(missing) error = %v, want ErrProfileNotFound", err)
	}
}

func TestOptionalProfileCredentialCanBeInitializedAndRemoved(t *testing.T) {
	t.Parallel()

	database := NewSQLiteDatabase(testLogger{}, testDBConfig{dbPath: filepath.Join(t.TempDir(), "optional-auth.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	profiles := NewProfileRepository(database)
	profile := &core.Profile{ID: "player-1", DisplayName: "Player", Role: core.ProfileRolePlayer, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := profiles.Create(context.Background(), profile); err != nil {
		t.Fatalf("Create(profile) error = %v", err)
	}
	service, err := auth.NewService(NewAuthStore(database), profiles)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	status, err := service.CredentialStatus(context.Background(), profile.ID)
	if err != nil || status.Configured {
		t.Fatalf("initial credential status = %+v, error = %v", status, err)
	}
	if err := service.InitializeCredential(context.Background(), profile.ID, "246810", auth.CredentialPIN); err != nil {
		t.Fatalf("InitializeCredential() error = %v", err)
	}
	if err := service.InitializeCredential(context.Background(), profile.ID, "135790", auth.CredentialPIN); !errors.Is(err, auth.ErrCredentialConfigured) {
		t.Fatalf("duplicate InitializeCredential() error = %v, want ErrCredentialConfigured", err)
	}
	_, session, err := service.Login(context.Background(), profile.ID, "246810")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if err := service.RemoveOwnCredential(context.Background(), session, profile.ID); err != nil {
		t.Fatalf("RemoveOwnCredential() error = %v", err)
	}
	status, err = service.CredentialStatus(context.Background(), profile.ID)
	if err != nil || status.Configured {
		t.Fatalf("removed credential status = %+v, error = %v", status, err)
	}
}
