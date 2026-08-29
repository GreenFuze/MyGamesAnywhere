package frontendauth

import (
	"errors"
	"sort"
	"strings"
	"time"
)

type Scope string

const (
	ScopeCatalogRead    Scope = "catalog.read"
	ScopeMetadataRead   Scope = "metadata.read"
	ScopeContentRead    Scope = "content.read"
	ScopeContentPrepare Scope = "content.prepare"
	ScopeManagement     Scope = "management"
)

var supportedScopes = map[Scope]struct{}{
	ScopeCatalogRead: {}, ScopeMetadataRead: {}, ScopeContentRead: {},
	ScopeContentPrepare: {}, ScopeManagement: {},
}

var (
	ErrInvalid         = errors.New("invalid frontend API client")
	ErrUnauthenticated = errors.New("valid frontend API client bearer token required")
	ErrForbidden       = errors.New("frontend API client scope is not authorized")
	ErrNotFound        = errors.New("frontend API client not found")
	ErrRevoked         = errors.New("frontend API client is revoked")
	ErrExpired         = errors.New("frontend API client is expired")
	ErrProfileMismatch = errors.New("frontend API client profile does not match")
)

type Client struct {
	ID         string     `json:"id"`
	ProfileID  string     `json:"profile_id"`
	Name       string     `json:"name"`
	Scopes     []Scope    `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
	SecretHash string     `json:"-"`
}

type IssuedClient struct {
	Client
	Token            string `json:"token"`
	TransportWarning string `json:"transport_warning"`
}

type Principal struct {
	ClientID  string  `json:"client_id"`
	ProfileID string  `json:"profile_id"`
	Name      string  `json:"name"`
	Scopes    []Scope `json:"scopes"`
}

type AuditEvent struct {
	ProfileID string
	ClientID  string
	Action    string
	Outcome   string
	Reason    string
	RequestID string
	RemoteIP  string
	CreatedAt time.Time
}

func AllScopes() []Scope {
	result := make([]Scope, 0, len(supportedScopes))
	for scope := range supportedScopes {
		result = append(result, scope)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func NormalizeScopes(scopes []Scope) ([]Scope, error) {
	seen := make(map[Scope]struct{}, len(scopes))
	result := make([]Scope, 0, len(scopes))
	for _, raw := range scopes {
		scope := Scope(strings.ToLower(strings.TrimSpace(string(raw))))
		if _, ok := supportedScopes[scope]; !ok {
			return nil, ErrInvalid
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	if len(result) == 0 {
		return nil, ErrInvalid
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func (p Principal) Has(scope Scope) bool {
	for _, candidate := range p.Scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}
