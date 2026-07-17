package v1

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	CapabilityEndpointPing              = "endpoint.ping"
	CapabilityEndpointRefresh           = "endpoint.refresh"
	CapabilityEndpointStop              = "endpoint.stop"
	CapabilityInventoryRefresh          = "inventory.refresh"
	CapabilityInstallationPreflight     = "installation.preflight"
	CapabilityGameInstallArchive        = "game.install_archive"
	CapabilityGameUninstall             = "game.uninstall"
	CapabilityGameInstallGogInno        = "game.install_gog_inno"
	CapabilityGameUninstallGogInno      = "game.uninstall_gog_inno"
	CapabilityGameCleanupGogInnoFailed  = "game.cleanup_gog_inno_failed"
	CapabilityGameValidateInstallations = "game.validate_installations"
	CapabilityGameLaunch                = "game.launch"
)

type ClientExecutionMode string

const (
	ClientExecutionModeStandard ClientExecutionMode = "standard"
	ClientExecutionModeElevated ClientExecutionMode = "elevated"
)

func (m ClientExecutionMode) Validate() error {
	if m == "" {
		return nil
	}
	if m != ClientExecutionModeStandard && m != ClientExecutionModeElevated {
		return fmt.Errorf("invalid client execution mode %q", m)
	}
	return nil
}

// EndpointMetadata describes one OS-user client installation. These fields are
// display and capability metadata; EndpointID remains the authorization target.
type EndpointMetadata struct {
	DisplayName   string              `json:"display_name"`
	HostName      string              `json:"host_name"`
	OSUser        string              `json:"os_user"`
	Platform      string              `json:"platform"`
	Arch          string              `json:"arch"`
	ExecutionMode ClientExecutionMode `json:"execution_mode"`
	Capabilities  []string            `json:"capabilities"`
}

// Validate rejects incomplete metadata and normalizes no values implicitly.
func (m EndpointMetadata) Validate() error {
	for name, value := range map[string]string{
		"display_name": m.DisplayName,
		"host_name":    m.HostName,
		"os_user":      m.OSUser,
		"platform":     m.Platform,
		"arch":         m.Arch,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if len(m.Capabilities) == 0 {
		return errors.New("at least one capability is required")
	}
	if err := m.ExecutionMode.Validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(m.Capabilities))
	for _, capability := range m.Capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" || !commandNamePattern.MatchString(capability) {
			return fmt.Errorf("invalid capability %q", capability)
		}
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("duplicate capability %q", capability)
		}
		seen[capability] = struct{}{}
	}
	return nil
}

// SortedCapabilities returns a stable copy for persistence and signatures.
func (m EndpointMetadata) SortedCapabilities() []string {
	capabilities := append([]string(nil), m.Capabilities...)
	sort.Strings(capabilities)
	return capabilities
}

// PairingRequest is sent over HTTPS to atomically consume a single-use code.
type PairingRequest struct {
	Code             string           `json:"code"`
	ClientInstanceID string           `json:"client_instance_id"`
	PublicKey        string           `json:"public_key"`
	ClientVersion    string           `json:"client_version"`
	Versions         VersionRange     `json:"versions"`
	Metadata         EndpointMetadata `json:"metadata"`
}

// Validate rejects malformed pairing material before it reaches persistence.
func (r PairingRequest) Validate() error {
	if strings.TrimSpace(r.Code) == "" {
		return errors.New("pairing code is required")
	}
	if strings.TrimSpace(r.ClientInstanceID) == "" {
		return errors.New("client_instance_id is required")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(r.PublicKey)
	if err != nil || len(publicKey) != 32 {
		return errors.New("public_key must be a base64url Ed25519 public key")
	}
	if strings.TrimSpace(r.ClientVersion) == "" {
		return errors.New("client_version is required")
	}
	if err := r.Versions.Validate(); err != nil {
		return fmt.Errorf("validate protocol range: %w", err)
	}
	if _, err := NegotiateVersion(r.Versions, SupportedVersionRange()); err != nil {
		return err
	}
	return r.Metadata.Validate()
}

// PairingResponse is returned once the endpoint and Owner grant are durable.
type PairingResponse struct {
	EndpointID      string          `json:"endpoint_id"`
	ProtocolVersion ProtocolVersion `json:"protocol_version"`
	WebSocketURL    string          `json:"websocket_url"`
}

// Validate rejects incomplete or unsafe pairing responses.
func (r PairingResponse) Validate() error {
	if strings.TrimSpace(r.EndpointID) == "" {
		return errors.New("endpoint_id is required")
	}
	if r.ProtocolVersion != Version {
		return fmt.Errorf("unsupported selected protocol version %d", r.ProtocolVersion)
	}
	parsed, err := url.Parse(r.WebSocketURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		return errors.New("websocket_url must be an absolute ws or wss URL")
	}
	return nil
}

// Hello starts an authenticated WebSocket connection.
type Hello struct {
	EndpointID       string           `json:"endpoint_id"`
	ClientInstanceID string           `json:"client_instance_id"`
	ClientVersion    string           `json:"client_version"`
	Versions         VersionRange     `json:"versions"`
	Metadata         EndpointMetadata `json:"metadata"`
}

func (h Hello) Validate() error {
	if strings.TrimSpace(h.EndpointID) == "" {
		return errors.New("endpoint_id is required")
	}
	if strings.TrimSpace(h.ClientInstanceID) == "" {
		return errors.New("client_instance_id is required")
	}
	if strings.TrimSpace(h.ClientVersion) == "" {
		return errors.New("client_version is required")
	}
	if err := h.Versions.Validate(); err != nil {
		return fmt.Errorf("validate protocol range: %w", err)
	}
	return h.Metadata.Validate()
}

// AuthChallenge contains fresh server material signed by the endpoint key.
type AuthChallenge struct {
	ConnectionID string    `json:"connection_id"`
	Nonce        string    `json:"nonce"`
	IssuedAt     time.Time `json:"issued_at"`
}

func (c AuthChallenge) Validate() error {
	if strings.TrimSpace(c.ConnectionID) == "" {
		return errors.New("connection_id is required")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(c.Nonce)
	if err != nil || len(nonce) < 32 {
		return errors.New("nonce must contain at least 32 base64url bytes")
	}
	if c.IssuedAt.IsZero() {
		return errors.New("issued_at is required")
	}
	return nil
}

// SigningBytes returns the canonical bytes signed by the endpoint.
func (c AuthChallenge) SigningBytes(endpointID string) ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(endpointID) == "" {
		return nil, errors.New("endpoint_id is required")
	}
	return []byte("mga-device-v1\n" + endpointID + "\n" + c.ConnectionID + "\n" + c.Nonce + "\n" + c.IssuedAt.UTC().Format(time.RFC3339Nano)), nil
}

// AuthResponse proves possession of the paired endpoint private key.
type AuthResponse struct {
	EndpointID   string `json:"endpoint_id"`
	ConnectionID string `json:"connection_id"`
	Signature    string `json:"signature"`
}

func (r AuthResponse) Validate() error {
	if strings.TrimSpace(r.EndpointID) == "" {
		return errors.New("endpoint_id is required")
	}
	if strings.TrimSpace(r.ConnectionID) == "" {
		return errors.New("connection_id is required")
	}
	signature, err := base64.RawURLEncoding.DecodeString(r.Signature)
	if err != nil || len(signature) != 64 {
		return errors.New("signature must be a base64url Ed25519 signature")
	}
	return nil
}

// ConnectionAccepted confirms authentication and heartbeat policy.
type ConnectionAccepted struct {
	ConnectionID     string          `json:"connection_id"`
	ProtocolVersion  ProtocolVersion `json:"protocol_version"`
	HeartbeatSeconds uint16          `json:"heartbeat_seconds"`
	ServerTime       time.Time       `json:"server_time"`
	UpdateRequired   bool            `json:"update_required"`
	RequiredVersion  string          `json:"required_version,omitempty"`
}

func (a ConnectionAccepted) Validate() error {
	if strings.TrimSpace(a.ConnectionID) == "" {
		return errors.New("connection_id is required")
	}
	if a.ProtocolVersion != Version {
		return fmt.Errorf("unsupported selected protocol version %d", a.ProtocolVersion)
	}
	if a.HeartbeatSeconds < 5 {
		return errors.New("heartbeat_seconds must be at least 5")
	}
	if a.ServerTime.IsZero() {
		return errors.New("server_time is required")
	}
	if a.UpdateRequired && strings.TrimSpace(a.RequiredVersion) == "" {
		return errors.New("required_version is required in update-required mode")
	}
	return nil
}

// Heartbeat reports current client readiness. Offline remains server-derived.
type Heartbeat struct {
	Sequence           uint64        `json:"sequence"`
	State              EndpointState `json:"state"`
	StateReason        string        `json:"state_reason,omitempty"`
	ClientVersion      string        `json:"client_version"`
	ActiveCommandCount uint16        `json:"active_command_count"`
}

func (h Heartbeat) Validate() error {
	if h.Sequence == 0 {
		return errors.New("sequence must be greater than zero")
	}
	if err := h.State.Validate(); err != nil {
		return err
	}
	if h.State == EndpointOffline {
		return errors.New("offline is server-derived and cannot be reported by a client")
	}
	if strings.TrimSpace(h.ClientVersion) == "" {
		return errors.New("client_version is required")
	}
	if (h.State == EndpointError || h.State == EndpointUpdateRequired) && strings.TrimSpace(h.StateReason) == "" {
		return errors.New("state_reason is required for error or update-required state")
	}
	return nil
}
