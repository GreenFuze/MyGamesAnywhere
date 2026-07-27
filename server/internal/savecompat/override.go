package savecompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

type OverrideOrigin string

const (
	OverrideOriginPlayer    OverrideOrigin = "player"
	OverrideOriginCommunity OverrideOrigin = "community"
)

type OverrideState string

const (
	OverrideStatePending  OverrideState = "pending"
	OverrideStateApproved OverrideState = "approved"
	OverrideStateConflict OverrideState = "conflict"
	OverrideStateRevoked  OverrideState = "revoked"
)

var (
	ErrOverrideConflict           = errors.New("save compatibility evidence conflicts with an approved override")
	ErrOverrideResolutionRequired = errors.New("save compatibility conflict requires explicit resolution")
)

// OverrideScope prevents player/community evidence from broadening beyond one
// exact, directional pair of profile-owned Save Domains and format versions.
type OverrideScope struct {
	OwnerProfileID string    `json:"owner_profile_id"`
	SourceDomainID string    `json:"source_domain_id"`
	TargetDomainID string    `json:"target_domain_id"`
	Source         FormatRef `json:"source"`
	Target         FormatRef `json:"target"`
}

func (s OverrideScope) Validate() error {
	if err := validateSingleLine("owner profile ID", s.OwnerProfileID, maxIDLength); err != nil {
		return err
	}
	if err := validateSingleLine("source Save Domain ID", s.SourceDomainID, maxIDLength); err != nil {
		return err
	}
	if err := validateSingleLine("target Save Domain ID", s.TargetDomainID, maxIDLength); err != nil {
		return err
	}
	if s.SourceDomainID == s.TargetDomainID {
		return errors.New("save compatibility override requires two distinct Save Domains")
	}
	if err := s.Source.Validate(); err != nil {
		return fmt.Errorf("source %w", err)
	}
	if err := s.Target.Validate(); err != nil {
		return fmt.Errorf("target %w", err)
	}
	return nil
}

func (s OverrideScope) Equal(other OverrideScope) bool {
	return s.OwnerProfileID == other.OwnerProfileID &&
		s.SourceDomainID == other.SourceDomainID &&
		s.TargetDomainID == other.TargetDomainID &&
		s.Source.Equal(other.Source) &&
		s.Target.Equal(other.Target)
}

type CompatibilityOverride struct {
	ID               string         `json:"id"`
	Scope            OverrideScope  `json:"scope"`
	Relationship     Relationship   `json:"relationship"`
	ConverterID      string         `json:"converter_id,omitempty"`
	ConverterVersion string         `json:"converter_version,omitempty"`
	Reversible       bool           `json:"reversible"`
	Origin           OverrideOrigin `json:"origin"`
	Attribution      string         `json:"attribution"`
	EvidenceSource   string         `json:"evidence_source"`
	EvidenceVersion  string         `json:"evidence_version"`
	EvidenceJSON     string         `json:"evidence_json"`
	State            OverrideState  `json:"state"`
	ReviewedBy       string         `json:"reviewed_by,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	ReviewedAt       *time.Time     `json:"reviewed_at,omitempty"`
	RevokedAt        *time.Time     `json:"revoked_at,omitempty"`
}

func (o CompatibilityOverride) Validate() error {
	if err := validateSingleLine("override ID", o.ID, maxIDLength); err != nil {
		return err
	}
	if err := o.Scope.Validate(); err != nil {
		return err
	}
	if err := validateSingleLine("override attribution", o.Attribution, maxAttributionLength); err != nil {
		return err
	}
	if err := validateSingleLine("evidence source", o.EvidenceSource, maxAttributionLength); err != nil {
		return err
	}
	if err := validateSingleLine("evidence version", o.EvidenceVersion, maxVersionLength); err != nil {
		return err
	}
	if err := validateEvidenceJSON(o.EvidenceJSON); err != nil {
		return err
	}
	switch o.Origin {
	case OverrideOriginPlayer, OverrideOriginCommunity:
	default:
		return errors.New("unsupported save compatibility override origin")
	}
	switch o.State {
	case OverrideStatePending, OverrideStateApproved, OverrideStateConflict, OverrideStateRevoked:
	default:
		return errors.New("unsupported save compatibility override state")
	}
	switch o.Relationship {
	case RelationshipSameFormat:
		if o.ConverterID != "" || o.ConverterVersion != "" {
			return errors.New("same-format override cannot name a converter")
		}
	case RelationshipConverter:
		if err := validateSingleLine("converter ID", o.ConverterID, maxIDLength); err != nil {
			return err
		}
		if err := validateSingleLine("converter version", o.ConverterVersion, maxVersionLength); err != nil {
			return err
		}
	default:
		return errors.New("unsupported save compatibility relationship")
	}
	if o.CreatedAt.IsZero() || o.UpdatedAt.Before(o.CreatedAt) {
		return errors.New("save compatibility override timestamps are invalid")
	}
	if o.State == OverrideStatePending && (o.ReviewedBy != "" || o.ReviewedAt != nil || o.RevokedAt != nil) {
		return errors.New("pending override cannot contain review or revocation metadata")
	}
	if o.State == OverrideStateApproved || o.State == OverrideStateConflict {
		if o.ReviewedBy != "" {
			if err := validateSingleLine("reviewer profile ID", o.ReviewedBy, maxIDLength); err != nil {
				return err
			}
		}
		if o.ReviewedAt == nil || o.RevokedAt != nil {
			return errors.New("reviewed override metadata is incomplete")
		}
	}
	if o.State == OverrideStateRevoked && o.RevokedAt == nil {
		return errors.New("revoked override requires a revocation timestamp")
	}
	return nil
}

func (o CompatibilityOverride) DecisionEqual(other CompatibilityOverride) bool {
	return o.Relationship == other.Relationship &&
		o.ConverterID == other.ConverterID &&
		o.ConverterVersion == other.ConverterVersion &&
		o.Reversible == other.Reversible
}

type OverrideSubmission struct {
	Scope            OverrideScope
	Relationship     Relationship
	ConverterID      string
	ConverterVersion string
	Reversible       bool
	Origin           OverrideOrigin
	Attribution      string
	EvidenceSource   string
	EvidenceVersion  string
	EvidenceJSON     string
}

type OverrideRepository interface {
	CreateOverride(context.Context, CompatibilityOverride) error
	GetOverride(context.Context, string) (*CompatibilityOverride, error)
	ApproveOverride(context.Context, string, string, time.Time, bool) (*CompatibilityOverride, error)
	RevokeOverride(context.Context, string, string, time.Time, bool) (*CompatibilityOverride, error)
	FindApprovedOverride(context.Context, OverrideScope) (*CompatibilityOverride, error)
}

var windowsAbsolutePath = regexp.MustCompile(`(?i)^[a-z]:[\\/]`)

func validateEvidenceJSON(raw string) error {
	if len(raw) == 0 || len(raw) > maxEvidenceBytes {
		return errors.New("compatibility evidence must be bounded valid JSON")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return errors.New("compatibility evidence must be bounded valid JSON")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return errors.New("compatibility evidence must contain exactly one JSON value")
	}
	if _, ok := value.(map[string]any); !ok {
		return errors.New("compatibility evidence must be a JSON object")
	}
	nodes := 0
	if err := validateEvidenceValue(value, "", 0, &nodes); err != nil {
		return err
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	return err
}

func validateEvidenceValue(value any, key string, depth int, nodes *int) error {
	(*nodes)++
	if depth > 8 || *nodes > 256 {
		return errors.New("compatibility evidence is too deeply nested or complex")
	}
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			if err := validateSingleLine("evidence field", childKey, maxIDLength); err != nil {
				return err
			}
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(childKey, "-", "_"), " ", "_"))
			for _, blocked := range []string{"password", "secret", "token", "api_key", "authorization", "cookie", "path"} {
				if strings.Contains(normalized, blocked) {
					return fmt.Errorf("compatibility evidence field %q may contain credentials or local paths", childKey)
				}
			}
			if err := validateEvidenceValue(childValue, childKey, depth+1, nodes); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateEvidenceValue(child, key, depth+1, nodes); err != nil {
				return err
			}
		}
	case string:
		if len(typed) > 1024 || strings.ContainsRune(typed, '\x00') {
			return fmt.Errorf("compatibility evidence field %q contains an invalid string", key)
		}
		trimmed := strings.TrimSpace(typed)
		if windowsAbsolutePath.MatchString(trimmed) || strings.HasPrefix(trimmed, `\\`) || strings.HasPrefix(trimmed, "/") {
			return fmt.Errorf("compatibility evidence field %q contains a local absolute path", key)
		}
	case json.Number, bool, nil:
	default:
		return errors.New("compatibility evidence contains an unsupported value")
	}
	return nil
}
