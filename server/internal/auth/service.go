package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

type CredentialKind string

const (
	CredentialPassword CredentialKind = "password"
	CredentialPIN      CredentialKind = "pin"
	BootstrapPassword                 = "changeme"
)

const (
	argonMemory      = 19 * 1024
	argonIterations  = 2
	argonParallelism = 1
	argonSaltLength  = 16
	argonKeyLength   = 32
	sessionDuration  = 7 * 24 * time.Hour
)

var (
	ErrUnauthenticated      = errors.New("authenticated profile session required")
	ErrForbidden            = errors.New("profile session is not authorized")
	ErrInvalidCredential    = errors.New("invalid profile credential")
	ErrCredentialRequired   = errors.New("profile credential is not configured")
	ErrCredentialChange     = errors.New("profile credential must be changed")
	ErrCredentialConfigured = errors.New("profile credential is already configured")
	ErrProfileNotFound      = errors.New("profile not found")
)

type Credential struct {
	ProfileID  string
	Kind       CredentialKind
	Hash       string
	MustChange bool
	UpdatedAt  time.Time
}

type Session struct {
	ID         string
	ProfileID  string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	MustChange bool
}

type Store interface {
	GetCredential(ctx context.Context, profileID string) (*Credential, error)
	CreateCredential(ctx context.Context, credential Credential) error
	SetCredential(ctx context.Context, credential Credential) error
	DeleteCredential(ctx context.Context, profileID string) error
	CreateSession(ctx context.Context, session Session, tokenHash string) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error
	DeleteSessionsByProfile(ctx context.Context, profileID string) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) error
}

type CredentialStatus struct {
	Configured bool           `json:"configured"`
	Kind       CredentialKind `json:"kind,omitempty"`
	MustChange bool           `json:"must_change"`
}

type Service struct {
	store    Store
	profiles core.ProfileRepository
	now      func() time.Time
}

func NewService(store Store, profiles core.ProfileRepository) (*Service, error) {
	if store == nil {
		return nil, errors.New("auth store is required")
	}
	if profiles == nil {
		return nil, errors.New("profile repository is required")
	}
	return &Service{store: store, profiles: profiles, now: time.Now}, nil
}

// EnsureBootstrapCredential assigns the known bootstrap password to the first
// profile with the administrator role, regardless of its display name. Device
// authority remains blocked while MustChange is true.
func (s *Service) EnsureBootstrapCredential(ctx context.Context) error {
	profiles, err := s.profiles.List(ctx)
	if err != nil {
		return fmt.Errorf("list profiles for bootstrap credential: %w", err)
	}
	for _, profile := range profiles {
		if profile == nil || profile.Role != core.ProfileRoleAdminPlayer {
			continue
		}
		existing, err := s.store.GetCredential(ctx, profile.ID)
		if err != nil {
			return fmt.Errorf("read bootstrap credential: %w", err)
		}
		if existing != nil {
			return nil
		}
		hash, err := hashCredential(BootstrapPassword)
		if err != nil {
			return fmt.Errorf("hash bootstrap credential: %w", err)
		}
		return s.store.SetCredential(ctx, Credential{
			ProfileID:  profile.ID,
			Kind:       CredentialPassword,
			Hash:       hash,
			MustChange: true,
			UpdatedAt:  s.now(),
		})
	}
	return nil
}

func (s *Service) Login(ctx context.Context, profileID, value string) (string, *Session, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" || value == "" {
		return "", nil, ErrInvalidCredential
	}
	profile, err := s.profiles.GetByID(ctx, profileID)
	if err != nil {
		return "", nil, err
	}
	if profile == nil {
		return "", nil, ErrInvalidCredential
	}
	credential, err := s.store.GetCredential(ctx, profileID)
	if err != nil {
		return "", nil, err
	}
	if credential == nil {
		return "", nil, ErrCredentialRequired
	}
	valid, err := verifyCredential(credential.Hash, value)
	if err != nil || !valid {
		return "", nil, ErrInvalidCredential
	}
	return s.createSession(ctx, profileID, credential.MustChange)
}

func (s *Service) CredentialStatus(ctx context.Context, profileID string) (CredentialStatus, error) {
	credential, err := s.store.GetCredential(ctx, strings.TrimSpace(profileID))
	if err != nil {
		return CredentialStatus{}, err
	}
	if credential == nil {
		return CredentialStatus{}, nil
	}
	return CredentialStatus{Configured: true, Kind: credential.Kind, MustChange: credential.MustChange}, nil
}

func (s *Service) InitializeCredential(ctx context.Context, profileID, value string, kind CredentialKind) error {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return ErrInvalidCredential
	}
	profile, err := s.profiles.GetByID(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return ErrInvalidCredential
	}
	if err := validateNewCredential(kind, value); err != nil {
		return err
	}
	hash, err := hashCredential(value)
	if err != nil {
		return err
	}
	return s.store.CreateCredential(ctx, Credential{
		ProfileID: profileID,
		Kind:      kind,
		Hash:      hash,
		UpdatedAt: s.now(),
	})
}

func (s *Service) RemoveOwnCredential(ctx context.Context, session *Session, profileID string) error {
	if err := s.RequireDeviceAuthority(session, profileID); err != nil {
		return err
	}
	if err := s.store.DeleteSessionsByProfile(ctx, session.ProfileID); err != nil {
		return err
	}
	return s.store.DeleteCredential(ctx, session.ProfileID)
}

// ResetCredentialToBootstrap is an offline/local-administration recovery path.
// It deliberately resets any profile to the public bootstrap password and marks
// it for an immediate forced change. Callers must enforce the local OS boundary.
func (s *Service) ResetCredentialToBootstrap(ctx context.Context, profileID string) error {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return ErrProfileNotFound
	}
	profile, err := s.profiles.GetByID(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return ErrProfileNotFound
	}
	hash, err := hashCredential(BootstrapPassword)
	if err != nil {
		return fmt.Errorf("hash recovery credential: %w", err)
	}
	if err := s.store.SetCredential(ctx, Credential{
		ProfileID:  profileID,
		Kind:       CredentialPassword,
		Hash:       hash,
		MustChange: true,
		UpdatedAt:  s.now(),
	}); err != nil {
		return err
	}
	return s.store.DeleteSessionsByProfile(ctx, profileID)
}

func (s *Service) Authenticate(ctx context.Context, token string) (*Session, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrUnauthenticated
	}
	session, err := s.store.GetSessionByTokenHash(ctx, tokenHash(token))
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrUnauthenticated
	}
	now := s.now()
	if !now.Before(session.ExpiresAt) {
		_ = s.store.DeleteSessionByTokenHash(ctx, tokenHash(token))
		return nil, ErrUnauthenticated
	}
	credential, err := s.store.GetCredential(ctx, session.ProfileID)
	if err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, ErrUnauthenticated
	}
	session.MustChange = credential.MustChange
	return session, nil
}

func (s *Service) ChangeOwnCredential(ctx context.Context, session *Session, currentValue, newValue string, kind CredentialKind) (string, *Session, error) {
	if session == nil {
		return "", nil, ErrUnauthenticated
	}
	credential, err := s.store.GetCredential(ctx, session.ProfileID)
	if err != nil {
		return "", nil, err
	}
	if credential == nil {
		return "", nil, ErrCredentialRequired
	}
	valid, err := verifyCredential(credential.Hash, currentValue)
	if err != nil || !valid {
		return "", nil, ErrInvalidCredential
	}
	if err := validateNewCredential(kind, newValue); err != nil {
		return "", nil, err
	}
	hash, err := hashCredential(newValue)
	if err != nil {
		return "", nil, err
	}
	if err := s.store.SetCredential(ctx, Credential{
		ProfileID: session.ProfileID,
		Kind:      kind,
		Hash:      hash,
		UpdatedAt: s.now(),
	}); err != nil {
		return "", nil, err
	}
	if err := s.store.DeleteSessionsByProfile(ctx, session.ProfileID); err != nil {
		return "", nil, err
	}
	return s.createSession(ctx, session.ProfileID, false)
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return s.store.DeleteSessionByTokenHash(ctx, tokenHash(token))
}

func (s *Service) RequireDeviceAuthority(session *Session, profileID string) error {
	if session == nil {
		return ErrUnauthenticated
	}
	if session.ProfileID != strings.TrimSpace(profileID) {
		return ErrForbidden
	}
	if session.MustChange {
		return ErrCredentialChange
	}
	return nil
}

func (s *Service) createSession(ctx context.Context, profileID string, mustChange bool) (string, *Session, error) {
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	now := s.now()
	session := &Session{
		ID:         uuid.NewString(),
		ProfileID:  profileID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(sessionDuration),
		MustChange: mustChange,
	}
	if err := s.store.CreateSession(ctx, *session, tokenHash(token)); err != nil {
		return "", nil, err
	}
	return token, session, nil
}

func validateNewCredential(kind CredentialKind, value string) error {
	switch kind {
	case CredentialPassword:
		if len(value) < 8 {
			return errors.New("password must contain at least 8 characters")
		}
	case CredentialPIN:
		if len(value) < 6 || len(value) > 12 {
			return errors.New("PIN must contain 6 to 12 digits")
		}
		for _, char := range value {
			if char < '0' || char > '9' {
				return errors.New("PIN must contain digits only")
			}
		}
	default:
		return fmt.Errorf("unsupported credential kind %q", kind)
	}
	return nil
}

func hashCredential(value string) (string, error) {
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(value), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonIterations,
		argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyCredential(encoded, value string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("unsupported credential hash")
	}
	version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil || version != argon2.Version {
		return false, errors.New("unsupported Argon2 version")
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, errors.New("invalid Argon2 parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errors.New("invalid credential salt")
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) == 0 {
		return false, errors.New("invalid credential hash")
	}
	actual := argon2.IDKey([]byte(value), salt, iterations, memory, parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
