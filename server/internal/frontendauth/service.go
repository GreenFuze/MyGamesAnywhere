package frontendauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	TokenPrefix      = "mga_v1_"
	TransportWarning = "Bearer tokens sent over plain HTTP can be observed and reused. Use HTTPS or a private authenticated tunnel outside an isolated trusted LAN."
)

type Repository interface {
	Create(context.Context, Client) error
	ListByProfile(context.Context, string) ([]Client, error)
	GetByID(context.Context, string) (*Client, error)
	Rotate(context.Context, string, string, string, time.Time) (*Client, error)
	Revoke(context.Context, string, string, time.Time) (*Client, error)
	TouchLastUsed(context.Context, string, time.Time) error
	RecordAudit(context.Context, AuditEvent) error
}

type Service struct {
	repository Repository
	now        func() time.Time
	random     func([]byte) error
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("frontend API client repository is required")
	}
	return &Service{repository: repository, now: time.Now, random: func(target []byte) error {
		_, err := rand.Read(target)
		return err
	}}, nil
}

func (s *Service) Create(ctx context.Context, profileID, name string, scopes []Scope, expiresAt *time.Time) (*IssuedClient, error) {
	profileID, name = strings.TrimSpace(profileID), strings.TrimSpace(name)
	if profileID == "" || name == "" || len(name) > 100 {
		return nil, ErrInvalid
	}
	normalized, err := NormalizeScopes(scopes)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC().Truncate(time.Second)
	if expiresAt != nil {
		expires := expiresAt.UTC().Truncate(time.Second)
		if !expires.After(now) {
			return nil, ErrInvalid
		}
		expiresAt = &expires
	}
	clientID := uuid.NewString()
	token, digest, err := s.issueToken(clientID)
	if err != nil {
		return nil, err
	}
	client := Client{ID: clientID, ProfileID: profileID, Name: name, Scopes: normalized, CreatedAt: now, UpdatedAt: now, ExpiresAt: expiresAt, SecretHash: digest}
	if err := s.repository.Create(ctx, client); err != nil {
		return nil, fmt.Errorf("create frontend API client: %w", err)
	}
	s.audit(ctx, AuditEvent{ProfileID: profileID, ClientID: clientID, Action: "create", Outcome: "success", CreatedAt: now})
	return &IssuedClient{Client: client, Token: token, TransportWarning: TransportWarning}, nil
}

func (s *Service) List(ctx context.Context, profileID string) ([]Client, error) {
	if strings.TrimSpace(profileID) == "" {
		return nil, ErrInvalid
	}
	return s.repository.ListByProfile(ctx, strings.TrimSpace(profileID))
}

func (s *Service) Rotate(ctx context.Context, profileID, clientID string) (*IssuedClient, error) {
	profileID, clientID = strings.TrimSpace(profileID), strings.TrimSpace(clientID)
	if profileID == "" || clientID == "" {
		return nil, ErrInvalid
	}
	token, digest, err := s.issueToken(clientID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC().Truncate(time.Second)
	client, err := s.repository.Rotate(ctx, profileID, clientID, digest, now)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, AuditEvent{ProfileID: profileID, ClientID: clientID, Action: "rotate", Outcome: "success", CreatedAt: now})
	return &IssuedClient{Client: *client, Token: token, TransportWarning: TransportWarning}, nil
}

func (s *Service) Revoke(ctx context.Context, profileID, clientID string) (*Client, error) {
	profileID, clientID = strings.TrimSpace(profileID), strings.TrimSpace(clientID)
	if profileID == "" || clientID == "" {
		return nil, ErrInvalid
	}
	now := s.now().UTC().Truncate(time.Second)
	client, err := s.repository.Revoke(ctx, profileID, clientID, now)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, AuditEvent{ProfileID: profileID, ClientID: clientID, Action: "revoke", Outcome: "success", CreatedAt: now})
	return client, nil
}

func (s *Service) Authenticate(ctx context.Context, bearer string, required ...Scope) (Principal, error) {
	now := s.now().UTC().Truncate(time.Second)
	clientID, secret, err := parseToken(bearer)
	if err != nil {
		s.audit(ctx, AuditEvent{Action: "authenticate", Outcome: "rejected", Reason: "malformed", CreatedAt: now})
		return Principal{}, ErrUnauthenticated
	}
	client, err := s.repository.GetByID(ctx, clientID)
	if err != nil {
		return Principal{}, fmt.Errorf("read frontend API client: %w", err)
	}
	if client == nil {
		s.audit(ctx, AuditEvent{ClientID: clientID, Action: "authenticate", Outcome: "rejected", Reason: "unknown", CreatedAt: now})
		return Principal{}, ErrUnauthenticated
	}
	reject := func(reason string, public error) (Principal, error) {
		s.audit(ctx, AuditEvent{ProfileID: client.ProfileID, ClientID: client.ID, Action: "authenticate", Outcome: "rejected", Reason: reason, CreatedAt: now})
		return Principal{}, public
	}
	digestBytes := sha256.Sum256(secret)
	expected, decodeErr := hex.DecodeString(client.SecretHash)
	if decodeErr != nil || len(expected) != sha256.Size || subtle.ConstantTimeCompare(digestBytes[:], expected) != 1 {
		return reject("secret", ErrUnauthenticated)
	}
	if client.RevokedAt != nil {
		return reject("revoked", ErrRevoked)
	}
	if client.ExpiresAt != nil && !client.ExpiresAt.After(now) {
		return reject("expired", ErrExpired)
	}
	principal := Principal{ClientID: client.ID, ProfileID: client.ProfileID, Name: client.Name, Scopes: append([]Scope(nil), client.Scopes...)}
	for _, scope := range required {
		if _, ok := supportedScopes[scope]; !ok || !principal.Has(scope) {
			return reject("scope", ErrForbidden)
		}
	}
	if err := s.repository.TouchLastUsed(ctx, client.ID, now); err != nil {
		return Principal{}, fmt.Errorf("update frontend API client use: %w", err)
	}
	s.audit(ctx, AuditEvent{ProfileID: client.ProfileID, ClientID: client.ID, Action: "authenticate", Outcome: "success", CreatedAt: now})
	return principal, nil
}

func (s *Service) issueToken(clientID string) (string, string, error) {
	secret := make([]byte, 32)
	if err := s.random(secret); err != nil {
		return "", "", fmt.Errorf("generate frontend API client secret: %w", err)
	}
	digest := sha256.Sum256(secret)
	token := TokenPrefix + clientID + "_" + base64.RawURLEncoding.EncodeToString(secret)
	return token, hex.EncodeToString(digest[:]), nil
}

func parseToken(token string) (string, []byte, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, TokenPrefix) {
		return "", nil, ErrUnauthenticated
	}
	remainder := strings.TrimPrefix(token, TokenPrefix)
	separator := strings.IndexByte(remainder, '_')
	if separator < 1 {
		return "", nil, ErrUnauthenticated
	}
	clientID, encoded := remainder[:separator], remainder[separator+1:]
	if _, err := uuid.Parse(clientID); err != nil || encoded == "" {
		return "", nil, ErrUnauthenticated
	}
	secret, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(secret) != 32 {
		return "", nil, ErrUnauthenticated
	}
	return clientID, secret, nil
}

func (s *Service) audit(ctx context.Context, event AuditEvent) {
	metadata := auditMetadataFromContext(ctx)
	if event.RequestID == "" {
		event.RequestID = metadata.RequestID
	}
	if event.RemoteIP == "" {
		event.RemoteIP = metadata.RemoteIP
	}
	_ = s.repository.RecordAudit(ctx, event)
}
