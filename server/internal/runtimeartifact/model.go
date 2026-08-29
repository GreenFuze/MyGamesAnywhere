package runtimeartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	ErrProfileRequired = errors.New("runtime artifact profile is required")
	ErrNotFound        = errors.New("runtime artifact not found")
	ErrDeliveryBlocked = errors.New("runtime artifact delivery is blocked")
	ErrIntegrity       = errors.New("runtime artifact integrity verification failed")
	identifierPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	spdxPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]{0,127}$`)
)

type Category string

const (
	CategoryRuntime  Category = "runtime"
	CategoryEmulator Category = "emulator"
)

func (v Category) Valid() bool { return v == CategoryRuntime || v == CategoryEmulator }

type AcquisitionMode string

const (
	AcquisitionBundled      AcquisitionMode = "bundled"
	AcquisitionCached       AcquisitionMode = "cached"
	AcquisitionProxy        AcquisitionMode = "proxy"
	AcquisitionUpstreamLink AcquisitionMode = "upstream_link"
)

func (v AcquisitionMode) Valid() bool {
	switch v {
	case AcquisitionBundled, AcquisitionCached, AcquisitionProxy, AcquisitionUpstreamLink:
		return true
	default:
		return false
	}
}

type ComplianceState string

const (
	ComplianceUnknown  ComplianceState = "unknown"
	ComplianceApproved ComplianceState = "approved"
	ComplianceBlocked  ComplianceState = "blocked"
)

func (v ComplianceState) Valid() bool {
	return v == ComplianceUnknown || v == ComplianceApproved || v == ComplianceBlocked
}

type Artifact struct {
	ID                string          `json:"id"`
	PackageID         string          `json:"package_id"`
	DisplayName       string          `json:"display_name"`
	Category          Category        `json:"category"`
	Version           string          `json:"version"`
	Channel           string          `json:"channel"`
	OS                string          `json:"os"`
	Architecture      string          `json:"architecture"`
	Compatibility     json.RawMessage `json:"compatibility"`
	LicenseSPDX       string          `json:"license_spdx"`
	LicenseURL        string          `json:"license_url,omitempty"`
	Notices           string          `json:"notices,omitempty"`
	UpstreamURL       string          `json:"upstream_url"`
	AcquisitionMode   AcquisitionMode `json:"acquisition_mode"`
	Redistributable   bool            `json:"redistributable"`
	ComplianceState   ComplianceState `json:"compliance_state"`
	SHA256            string          `json:"sha256,omitempty"`
	Signature         string          `json:"signature,omitempty"`
	ReleaseObservedAt *time.Time      `json:"release_observed_at,omitempty"`
	SizeBytes         int64           `json:"size_bytes"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func (a *Artifact) Normalize() {
	if a == nil {
		return
	}
	a.ID = strings.ToLower(strings.TrimSpace(a.ID))
	a.PackageID = strings.ToLower(strings.TrimSpace(a.PackageID))
	a.DisplayName = strings.TrimSpace(a.DisplayName)
	a.Category = Category(strings.ToLower(strings.TrimSpace(string(a.Category))))
	a.Version = strings.TrimSpace(a.Version)
	a.Channel = strings.ToLower(strings.TrimSpace(a.Channel))
	a.OS = strings.ToLower(strings.TrimSpace(a.OS))
	a.Architecture = strings.ToLower(strings.TrimSpace(a.Architecture))
	a.LicenseSPDX = strings.TrimSpace(a.LicenseSPDX)
	a.LicenseURL = strings.TrimSpace(a.LicenseURL)
	a.Notices = strings.TrimSpace(a.Notices)
	a.UpstreamURL = strings.TrimSpace(a.UpstreamURL)
	a.AcquisitionMode = AcquisitionMode(strings.ToLower(strings.TrimSpace(string(a.AcquisitionMode))))
	a.ComplianceState = ComplianceState(strings.ToLower(strings.TrimSpace(string(a.ComplianceState))))
	a.SHA256 = strings.ToLower(strings.TrimSpace(a.SHA256))
	a.Signature = strings.TrimSpace(a.Signature)
	if len(a.Compatibility) == 0 {
		a.Compatibility = json.RawMessage(`{}`)
	}
	if a.ReleaseObservedAt != nil {
		value := a.ReleaseObservedAt.UTC().Truncate(time.Second)
		a.ReleaseObservedAt = &value
	}
}

func (a Artifact) Validate() error {
	a.Normalize()
	if !identifierPattern.MatchString(a.ID) || !identifierPattern.MatchString(a.PackageID) {
		return errors.New("artifact id and package_id must be canonical lowercase identifiers")
	}
	if a.DisplayName == "" || a.Version == "" || !identifierPattern.MatchString(a.Channel) || !identifierPattern.MatchString(a.OS) || !identifierPattern.MatchString(a.Architecture) {
		return errors.New("display_name, version, channel, os, and architecture are required")
	}
	if !a.Category.Valid() || !a.AcquisitionMode.Valid() || !a.ComplianceState.Valid() {
		return errors.New("unsupported artifact category, acquisition mode, or compliance state")
	}
	if !json.Valid(a.Compatibility) || len(a.Compatibility) > 64*1024 {
		return errors.New("compatibility must be valid bounded JSON")
	}
	if !spdxPattern.MatchString(a.LicenseSPDX) {
		return errors.New("a valid SPDX license identifier is required")
	}
	if err := validateHTTPSURL(a.UpstreamURL, true); err != nil {
		return err
	}
	if err := validateHTTPSURL(a.LicenseURL, false); err != nil {
		return err
	}
	if a.SizeBytes < 0 {
		return errors.New("artifact size cannot be negative")
	}
	if a.SHA256 != "" {
		decoded, err := hex.DecodeString(a.SHA256)
		if err != nil || len(decoded) != sha256.Size {
			return errors.New("artifact sha256 must be 64 lowercase hexadecimal characters")
		}
	}
	if a.ComplianceState == ComplianceApproved && a.Redistributable && a.AcquisitionMode != AcquisitionUpstreamLink && a.SHA256 == "" {
		return errors.New("deliverable approved artifact requires sha256 evidence")
	}
	if a.AcquisitionMode == AcquisitionUpstreamLink && a.Redistributable {
		return errors.New("upstream_link artifact cannot claim redistributable delivery")
	}
	return nil
}

func (a Artifact) Deliverable() bool {
	return a.ComplianceState == ComplianceApproved && a.Redistributable && a.AcquisitionMode != AcquisitionUpstreamLink && a.SHA256 != ""
}

func validateHTTPSURL(value string, required bool) error {
	if value == "" && !required {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return errors.New("artifact URLs must be absolute HTTPS URLs without embedded credentials")
	}
	return nil
}
