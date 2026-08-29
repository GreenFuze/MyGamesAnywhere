package frontendauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type memoryRepository struct {
	clients map[string]Client
	audit   []AuditEvent
}

func newMemoryRepository() *memoryRepository { return &memoryRepository{clients: map[string]Client{}} }
func (r *memoryRepository) Create(_ context.Context, client Client) error {
	r.clients[client.ID] = client
	return nil
}
func (r *memoryRepository) ListByProfile(_ context.Context, profileID string) ([]Client, error) {
	result := []Client{}
	for _, client := range r.clients {
		if client.ProfileID == profileID {
			result = append(result, client)
		}
	}
	return result, nil
}
func (r *memoryRepository) GetByID(_ context.Context, id string) (*Client, error) {
	client, ok := r.clients[id]
	if !ok {
		return nil, nil
	}
	return &client, nil
}
func (r *memoryRepository) Rotate(_ context.Context, profileID, id, digest string, now time.Time) (*Client, error) {
	client, ok := r.clients[id]
	if !ok || client.ProfileID != profileID {
		return nil, ErrNotFound
	}
	client.SecretHash, client.RevokedAt, client.LastUsedAt, client.UpdatedAt = digest, nil, nil, now
	r.clients[id] = client
	return &client, nil
}
func (r *memoryRepository) Revoke(_ context.Context, profileID, id string, now time.Time) (*Client, error) {
	client, ok := r.clients[id]
	if !ok || client.ProfileID != profileID {
		return nil, ErrNotFound
	}
	client.RevokedAt, client.UpdatedAt = &now, now
	r.clients[id] = client
	return &client, nil
}
func (r *memoryRepository) TouchLastUsed(_ context.Context, id string, now time.Time) error {
	client := r.clients[id]
	client.LastUsedAt = &now
	r.clients[id] = client
	return nil
}
func (r *memoryRepository) RecordAudit(_ context.Context, event AuditEvent) error {
	r.audit = append(r.audit, event)
	return nil
}

func TestServiceIssuesOneTimeHashedScopedToken(t *testing.T) {
	repository := newMemoryRepository()
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	service.random = func(target []byte) error {
		for i := range target {
			target[i] = byte(i + 1)
		}
		return nil
	}
	issued, err := service.Create(context.Background(), "profile-1", "Playnite", []Scope{ScopeContentRead, ScopeCatalogRead, ScopeCatalogRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(issued.Token, TokenPrefix) || issued.SecretHash == "" || strings.Contains(issued.SecretHash, issued.Token) {
		t.Fatalf("issued client did not separate raw token from digest: %+v", issued.Client)
	}
	stored := repository.clients[issued.ID]
	if stored.SecretHash != issued.SecretHash || len(stored.Scopes) != 2 {
		t.Fatalf("stored client = %+v", stored)
	}
	principal, err := service.Authenticate(context.Background(), issued.Token, ScopeCatalogRead)
	if err != nil || principal.ProfileID != "profile-1" {
		t.Fatalf("authenticate = %+v, %v", principal, err)
	}
	if _, err := service.Authenticate(context.Background(), issued.Token, ScopeManagement); !errors.Is(err, ErrForbidden) {
		t.Fatalf("over-scoped error = %v", err)
	}
	for _, event := range repository.audit {
		if strings.Contains(event.Reason, issued.Token) {
			t.Fatal("audit retained raw token")
		}
	}
}

func TestServiceRotationRevocationExpiryAndMalformedTokensFailClosed(t *testing.T) {
	repository := newMemoryRepository()
	service, _ := NewService(repository)
	now := time.Unix(1_800_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	seed := byte(1)
	service.random = func(target []byte) error {
		for i := range target {
			target[i] = seed
		}
		seed++
		return nil
	}
	expires := now.Add(time.Hour)
	issued, err := service.Create(context.Background(), "profile-1", "Adapter", []Scope{ScopeMetadataRead}, &expires)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := service.Rotate(context.Background(), "profile-1", issued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), issued.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("old token error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), "mga_v1_broken"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("malformed error = %v", err)
	}
	now = expires
	if _, err := service.Authenticate(context.Background(), rotated.Token); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired error = %v", err)
	}
	now = expires.Add(-time.Minute)
	if _, err := service.Revoke(context.Background(), "other-profile", issued.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-profile revoke = %v", err)
	}
	if _, err := service.Revoke(context.Background(), "profile-1", issued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), rotated.Token); !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked error = %v", err)
	}
}
