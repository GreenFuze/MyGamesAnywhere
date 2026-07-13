package devices

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrClientLaunchNotFound = errors.New("MGA Client launch challenge not found")
	ErrClientLaunchExpired  = errors.New("MGA Client launch challenge expired")
	ErrClientLaunchUsed     = errors.New("MGA Client launch challenge was already used")
)

const clientLaunchLifetime = 2 * time.Minute

// ClientLaunchRegistry owns short-lived, process-local launch challenges. They
// intentionally do not survive server restarts and contain no durable state.
type ClientLaunchRegistry struct {
	mu      sync.Mutex
	entries map[string]ClientLaunch
}

func NewClientLaunchRegistry() *ClientLaunchRegistry {
	return &ClientLaunchRegistry{entries: make(map[string]ClientLaunch)}
}

func (r *ClientLaunchRegistry) Create(profileID string, now time.Time) (string, ClientLaunch, error) {
	if strings.TrimSpace(profileID) == "" {
		return "", ClientLaunch{}, errors.New("profile_id is required")
	}
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", ClientLaunch{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	launch := ClientLaunch{
		ID:        uuid.NewString(),
		ProfileID: profileID,
		TokenHash: hashClientLaunchToken(token),
		Status:    ClientLaunchWaiting,
		CreatedAt: now,
		ExpiresAt: now.Add(clientLaunchLifetime),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleteStaleLocked(now)
	r.entries[launch.ID] = launch
	return token, launch, nil
}

func (r *ClientLaunchRegistry) Get(id, profileID string, now time.Time) (ClientLaunch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	launch, ok := r.entries[strings.TrimSpace(id)]
	if !ok || launch.ProfileID != strings.TrimSpace(profileID) {
		return ClientLaunch{}, ErrClientLaunchNotFound
	}
	if launch.Status == ClientLaunchWaiting && !now.Before(launch.ExpiresAt) {
		launch.Status = ClientLaunchExpired
		r.entries[launch.ID] = launch
	}
	return launch, nil
}

// GetForRedemption is server-internal because public redemption has no profile
// session. Authorization is subsequently proven by the endpoint signature and
// the challenge profile's device grant.
func (r *ClientLaunchRegistry) GetForRedemption(id string, now time.Time) (ClientLaunch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	launch, ok := r.entries[strings.TrimSpace(id)]
	if !ok {
		return ClientLaunch{}, ErrClientLaunchNotFound
	}
	if launch.Status == ClientLaunchAcknowledged {
		return ClientLaunch{}, ErrClientLaunchUsed
	}
	if !now.Before(launch.ExpiresAt) {
		launch.Status = ClientLaunchExpired
		r.entries[launch.ID] = launch
		return ClientLaunch{}, ErrClientLaunchExpired
	}
	return launch, nil
}

func (r *ClientLaunchRegistry) Redeem(id, token, endpointID string, now time.Time) (ClientLaunch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	launch, ok := r.entries[strings.TrimSpace(id)]
	if !ok {
		return ClientLaunch{}, ErrClientLaunchNotFound
	}
	if launch.Status == ClientLaunchAcknowledged {
		return ClientLaunch{}, ErrClientLaunchUsed
	}
	if !now.Before(launch.ExpiresAt) {
		launch.Status = ClientLaunchExpired
		r.entries[launch.ID] = launch
		return ClientLaunch{}, ErrClientLaunchExpired
	}
	want := []byte(launch.TokenHash)
	got := []byte(hashClientLaunchToken(token))
	if len(want) != len(got) || subtle.ConstantTimeCompare(want, got) != 1 {
		return ClientLaunch{}, ErrClientLaunchNotFound
	}
	if strings.TrimSpace(endpointID) == "" {
		return ClientLaunch{}, errors.New("endpoint_id is required")
	}
	launch.EndpointID = strings.TrimSpace(endpointID)
	launch.Status = ClientLaunchAcknowledged
	r.entries[launch.ID] = launch
	return launch, nil
}

func (r *ClientLaunchRegistry) deleteStaleLocked(now time.Time) {
	for id, launch := range r.entries {
		if now.Sub(launch.ExpiresAt) > 10*time.Minute {
			delete(r.entries, id)
		}
	}
}

func hashClientLaunchToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
