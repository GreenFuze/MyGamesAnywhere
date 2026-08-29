package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrProfileRequired           = errors.New("catalog profile is required")
	ErrOfferNotFound             = errors.New("catalog offer not found")
	ErrCatalogIdentityNotVisible = errors.New("catalog identity is not visible to the active profile")
)

type Entitlement string

const (
	EntitlementOwned        Entitlement = "owned"
	EntitlementSubscription Entitlement = "subscription"
	EntitlementShared       Entitlement = "shared"
	EntitlementTrial        Entitlement = "trial"
	EntitlementNone         Entitlement = "none"
	EntitlementUnknown      Entitlement = "unknown"
)

func (e Entitlement) Valid() bool {
	switch e {
	case EntitlementOwned, EntitlementSubscription, EntitlementShared, EntitlementTrial, EntitlementNone, EntitlementUnknown:
		return true
	default:
		return false
	}
}

type Availability string

const (
	AvailabilityAvailable   Availability = "available"
	AvailabilityLeavingSoon Availability = "leaving_soon"
	AvailabilityUnavailable Availability = "unavailable"
	AvailabilityUnknown     Availability = "unknown"
)

func (a Availability) Valid() bool {
	switch a {
	case AvailabilityAvailable, AvailabilityLeavingSoon, AvailabilityUnavailable, AvailabilityUnknown:
		return true
	default:
		return false
	}
}

func (a Availability) Active() bool {
	return a == AvailabilityAvailable || a == AvailabilityLeavingSoon
}

type Delivery string

const (
	DeliveryMGAContent   Delivery = "mga_content"
	DeliveryStorefront   Delivery = "storefront"
	DeliveryCloud        Delivery = "cloud"
	DeliveryMetadataOnly Delivery = "metadata_only"
)

func (d Delivery) Valid() bool {
	switch d {
	case DeliveryMGAContent, DeliveryStorefront, DeliveryCloud, DeliveryMetadataOnly:
		return true
	default:
		return false
	}
}

type EventType string

const (
	EventAdded          EventType = "added"
	EventRemoved        EventType = "removed"
	EventReturned       EventType = "returned"
	EventLeavingSoon    EventType = "leaving_soon"
	EventVersionChanged EventType = "version_changed"
)

type PackageVersion struct {
	ID              string    `json:"id"`
	OfferID         string    `json:"offer_id"`
	Version         string    `json:"version,omitempty"`
	BuildID         string    `json:"build_id,omitempty"`
	Channel         string    `json:"channel,omitempty"`
	SourceRevision  string    `json:"source_revision,omitempty"`
	SHA256          string    `json:"sha256,omitempty"`
	SizeBytes       int64     `json:"size_bytes,omitempty"`
	ReleasedAt      time.Time `json:"released_at,omitempty"`
	FirstObservedAt time.Time `json:"first_observed_at"`
	LastObservedAt  time.Time `json:"last_observed_at"`
}

func (v *PackageVersion) normalize() {
	if v == nil {
		return
	}
	v.Version = strings.TrimSpace(v.Version)
	v.BuildID = strings.TrimSpace(v.BuildID)
	v.Channel = strings.ToLower(strings.TrimSpace(v.Channel))
	v.SourceRevision = strings.TrimSpace(v.SourceRevision)
	v.SHA256 = strings.ToLower(strings.TrimSpace(v.SHA256))
}

func (v PackageVersion) Validate() error {
	v.normalize()
	if v.Version == "" && v.BuildID == "" && v.SourceRevision == "" && v.SHA256 == "" {
		return errors.New("catalog package version requires version, build, revision, or sha256 identity")
	}
	if v.SizeBytes < 0 {
		return errors.New("catalog package size cannot be negative")
	}
	if v.SHA256 != "" {
		decoded, err := hex.DecodeString(v.SHA256)
		if err != nil || len(decoded) != sha256.Size {
			return errors.New("catalog package sha256 must be 64 hexadecimal characters")
		}
	}
	return nil
}

func (v PackageVersion) IdentityKey() string {
	v.normalize()
	return stableKey(v.Version, v.BuildID, v.Channel, v.SourceRevision, v.SHA256)
}

type ObservationCommand struct {
	CanonicalGameID string          `json:"canonical_game_id"`
	SourceGameID    string          `json:"source_game_id,omitempty"`
	IntegrationID   string          `json:"integration_id,omitempty"`
	Provider        string          `json:"provider"`
	SKU             string          `json:"sku"`
	Platform        string          `json:"platform"`
	Region          string          `json:"region"`
	Entitlement     Entitlement     `json:"entitlement"`
	Delivery        Delivery        `json:"delivery"`
	Availability    Availability    `json:"availability"`
	EvidenceSource  string          `json:"evidence_source"`
	EvidenceJSON    json.RawMessage `json:"evidence"`
	ObservedAt      time.Time       `json:"observed_at"`
	CurrentVersion  *PackageVersion `json:"current_version,omitempty"`
	LatestVersion   *PackageVersion `json:"latest_version,omitempty"`
}

func (c *ObservationCommand) Normalize() {
	if c == nil {
		return
	}
	c.CanonicalGameID = strings.TrimSpace(c.CanonicalGameID)
	c.SourceGameID = strings.TrimSpace(c.SourceGameID)
	c.IntegrationID = strings.TrimSpace(c.IntegrationID)
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	c.SKU = strings.TrimSpace(c.SKU)
	c.Platform = strings.ToLower(strings.TrimSpace(c.Platform))
	c.Region = strings.ToLower(strings.TrimSpace(c.Region))
	c.EvidenceSource = strings.TrimSpace(c.EvidenceSource)
	if c.Region == "" {
		c.Region = "global"
	}
	if len(c.EvidenceJSON) == 0 {
		c.EvidenceJSON = json.RawMessage(`{}`)
	}
	if !c.ObservedAt.IsZero() {
		c.ObservedAt = c.ObservedAt.UTC().Truncate(time.Second)
	}
	c.CurrentVersion.normalize()
	c.LatestVersion.normalize()
}

func (c ObservationCommand) Validate() error {
	c.Normalize()
	if c.CanonicalGameID == "" || c.Provider == "" || c.SKU == "" || c.Platform == "" {
		return errors.New("canonical game, provider, sku, and platform are required")
	}
	if !c.Entitlement.Valid() {
		return fmt.Errorf("unsupported catalog entitlement %q", c.Entitlement)
	}
	if !c.Delivery.Valid() {
		return fmt.Errorf("unsupported catalog delivery %q", c.Delivery)
	}
	if !c.Availability.Valid() {
		return fmt.Errorf("unsupported catalog availability %q", c.Availability)
	}
	if c.EvidenceSource == "" || !json.Valid(c.EvidenceJSON) {
		return errors.New("catalog evidence source and valid JSON are required")
	}
	if c.ObservedAt.IsZero() {
		return errors.New("catalog observation time is required")
	}
	for _, version := range []*PackageVersion{c.CurrentVersion, c.LatestVersion} {
		if version != nil {
			if err := version.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c ObservationCommand) OfferKey() string {
	c.Normalize()
	return stableKey(c.IntegrationID, c.Provider, c.SKU, c.Platform, c.Region)
}

func (c ObservationCommand) ObservationKey() string {
	c.Normalize()
	current, latest := "", ""
	if c.CurrentVersion != nil {
		current = c.CurrentVersion.IdentityKey()
	}
	if c.LatestVersion != nil {
		latest = c.LatestVersion.IdentityKey()
	}
	return stableKey(
		c.OfferKey(), string(c.Entitlement), string(c.Delivery), string(c.Availability),
		current, latest, c.EvidenceSource, string(c.EvidenceJSON), c.ObservedAt.UTC().Format(time.RFC3339Nano),
	)
}

type Offer struct {
	ID              string          `json:"id"`
	ProfileID       string          `json:"profile_id"`
	CanonicalGameID string          `json:"canonical_game_id"`
	SourceGameID    string          `json:"source_game_id,omitempty"`
	IntegrationID   string          `json:"integration_id,omitempty"`
	Provider        string          `json:"provider"`
	SKU             string          `json:"sku"`
	Platform        string          `json:"platform"`
	Region          string          `json:"region"`
	Entitlement     Entitlement     `json:"entitlement"`
	Delivery        Delivery        `json:"delivery"`
	Availability    Availability    `json:"availability"`
	EvidenceSource  string          `json:"evidence_source"`
	EvidenceJSON    json.RawMessage `json:"evidence"`
	ObservedAt      time.Time       `json:"observed_at"`
	LastSuccessAt   time.Time       `json:"last_success_at"`
	StaleAt         time.Time       `json:"stale_at,omitempty"`
	CurrentVersion  *PackageVersion `json:"current_version,omitempty"`
	LatestVersion   *PackageVersion `json:"latest_version,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

func (o Offer) Stale() bool { return !o.StaleAt.IsZero() }

type HistoryEvent struct {
	ID                       string       `json:"id"`
	OfferID                  string       `json:"offer_id"`
	ObservationID            string       `json:"observation_id"`
	Type                     EventType    `json:"type"`
	PreviousAvailability     Availability `json:"previous_availability,omitempty"`
	Availability             Availability `json:"availability"`
	PreviousCurrentVersionID string       `json:"previous_current_version_id,omitempty"`
	CurrentVersionID         string       `json:"current_version_id,omitempty"`
	PreviousLatestVersionID  string       `json:"previous_latest_version_id,omitempty"`
	LatestVersionID          string       `json:"latest_version_id,omitempty"`
	OccurredAt               time.Time    `json:"occurred_at"`
}

type RefreshScope struct {
	Provider      string    `json:"provider"`
	IntegrationID string    `json:"integration_id,omitempty"`
	AttemptedAt   time.Time `json:"attempted_at"`
}

func (s *RefreshScope) Normalize() {
	if s == nil {
		return
	}
	s.Provider = strings.ToLower(strings.TrimSpace(s.Provider))
	s.IntegrationID = strings.TrimSpace(s.IntegrationID)
	if !s.AttemptedAt.IsZero() {
		s.AttemptedAt = s.AttemptedAt.UTC().Truncate(time.Second)
	}
}

func (s RefreshScope) Validate() error {
	s.Normalize()
	if s.Provider == "" || s.AttemptedAt.IsZero() {
		return errors.New("catalog refresh provider and attempted time are required")
	}
	return nil
}

func (s RefreshScope) Key() string {
	s.Normalize()
	return stableKey(s.Provider, s.IntegrationID)
}

type RefreshFailure struct {
	RefreshScope
	Error string `json:"error"`
}

func (f RefreshFailure) Validate() error {
	if err := f.RefreshScope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(f.Error) == "" {
		return errors.New("catalog refresh failure requires an error")
	}
	return nil
}

type OfferFilter struct {
	CanonicalGameID string
	Provider        string
	Availability    Availability
	StaleOnly       bool
}

func stableKey(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
