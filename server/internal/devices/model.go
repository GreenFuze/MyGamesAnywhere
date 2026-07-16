package devices

import (
	"context"
	"encoding/json"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type Endpoint struct {
	ID               string                       `json:"id"`
	ClientInstanceID string                       `json:"client_instance_id"`
	PublicKey        string                       `json:"-"`
	DisplayName      string                       `json:"display_name"`
	HostName         string                       `json:"host_name"`
	OSUser           string                       `json:"os_user"`
	Platform         string                       `json:"platform"`
	Arch             string                       `json:"arch"`
	ExecutionMode    devicev1.ClientExecutionMode `json:"execution_mode"`
	ClientVersion    string                       `json:"client_version"`
	ProtocolVersion  devicev1.ProtocolVersion     `json:"protocol_version"`
	Capabilities     []string                     `json:"capabilities"`
	Status           devicev1.EndpointState       `json:"status"`
	StatusReason     string                       `json:"status_reason,omitempty"`
	LastSeenAt       *time.Time                   `json:"last_seen_at,omitempty"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	AccessLevel      devicev1.AccessLevel         `json:"access_level"`
	Inventory        *devicev1.DeviceInventory    `json:"inventory,omitempty"`
	Installations    []GameInstallation           `json:"installations,omitempty"`
}

type PairingChallenge struct {
	ID        string
	CodeHash  string
	ProfileID string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type ClientLaunchStatus string

const (
	ClientLaunchWaiting      ClientLaunchStatus = "waiting"
	ClientLaunchAcknowledged ClientLaunchStatus = "acknowledged"
	ClientLaunchExpired      ClientLaunchStatus = "expired"
)

// ClientLaunch is an ephemeral browser-to-client association challenge. The
// raw token is returned only once and is never stored.
type ClientLaunch struct {
	ID            string                       `json:"id"`
	ProfileID     string                       `json:"-"`
	TokenHash     string                       `json:"-"`
	EndpointID    string                       `json:"endpoint_id,omitempty"`
	ExecutionMode devicev1.ClientExecutionMode `json:"execution_mode"`
	Status        ClientLaunchStatus           `json:"status"`
	CreatedAt     time.Time                    `json:"created_at"`
	ExpiresAt     time.Time                    `json:"expires_at"`
}

type Grant struct {
	EndpointID         string               `json:"endpoint_id"`
	ProfileID          string               `json:"profile_id"`
	ProfileDisplayName string               `json:"profile_display_name"`
	ProfileRole        string               `json:"profile_role"`
	AccessLevel        devicev1.AccessLevel `json:"access_level"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

type Command struct {
	ID                   string                 `json:"id"`
	EndpointID           string                 `json:"endpoint_id"`
	ProfileID            string                 `json:"profile_id"`
	Name                 string                 `json:"name"`
	SchemaVersion        uint16                 `json:"schema_version"`
	IdempotencyKey       string                 `json:"idempotency_key"`
	Status               devicev1.CommandStatus `json:"status"`
	Payload              json.RawMessage        `json:"payload"`
	Result               json.RawMessage        `json:"result,omitempty"`
	ErrorCode            string                 `json:"error_code,omitempty"`
	ErrorMessage         string                 `json:"error_message,omitempty"`
	ProgressSequence     uint64                 `json:"progress_sequence,omitempty"`
	ProgressPhase        string                 `json:"progress_phase,omitempty"`
	ProgressPercent      *uint8                 `json:"progress_percent,omitempty"`
	ProgressStage        string                 `json:"progress_stage,omitempty"`
	ProgressStagePercent *uint8                 `json:"progress_stage_percent,omitempty"`
	ProgressMessage      string                 `json:"progress_message,omitempty"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
	ExpiresAt            time.Time              `json:"expires_at"`
}

type GameInstallation struct {
	EndpointID                string                        `json:"endpoint_id"`
	GameID                    string                        `json:"game_id"`
	SourceGameID              string                        `json:"source_game_id"`
	ProfileID                 string                        `json:"profile_id"`
	InstallRoot               string                        `json:"install_root"`
	InstallPath               string                        `json:"install_path"`
	ArchiveSHA256             string                        `json:"archive_sha256"`
	ArchiveBytes              uint64                        `json:"archive_bytes"`
	InstalledAt               time.Time                     `json:"installed_at"`
	UpdatedAt                 time.Time                     `json:"updated_at"`
	LaunchTarget              string                        `json:"launch_target,omitempty"`
	LaunchCandidates          []string                      `json:"launch_candidates,omitempty"`
	InstallKind               string                        `json:"install_kind"`
	InstallerFamily           string                        `json:"installer_family,omitempty"`
	InstallerFiles            []devicev1.GogInnoPackageFile `json:"installer_files,omitempty"`
	UninstallTarget           string                        `json:"uninstall_target,omitempty"`
	InstallState              string                        `json:"install_state"`
	StateReason               string                        `json:"state_reason,omitempty"`
	LastVerifiedAt            *time.Time                    `json:"last_verified_at,omitempty"`
	StateChangedAt            *time.Time                    `json:"state_changed_at,omitempty"`
	CleanupMarkerID           string                        `json:"cleanup_marker_id,omitempty"`
	CleanupIgnoredAt          *time.Time                    `json:"cleanup_ignored_at,omitempty"`
	CleanupIgnoredByProfileID string                        `json:"cleanup_ignored_by_profile_id,omitempty"`
}

type InstallationEvent struct {
	ID             string          `json:"id"`
	EndpointID     string          `json:"endpoint_id"`
	GameID         string          `json:"game_id"`
	SourceGameID   string          `json:"source_game_id"`
	ActorProfileID string          `json:"actor_profile_id,omitempty"`
	EventType      string          `json:"event_type"`
	Reason         string          `json:"reason,omitempty"`
	Details        json.RawMessage `json:"details,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

type Store interface {
	CreatePairingChallenge(ctx context.Context, challenge PairingChallenge) error
	PairEndpoint(ctx context.Context, codeHash string, now time.Time, endpoint Endpoint) (string, error)
	GetEndpoint(ctx context.Context, endpointID string) (*Endpoint, error)
	ListEndpoints(ctx context.Context, profileID string) ([]Endpoint, error)
	GetGrant(ctx context.Context, endpointID, profileID string) (devicev1.AccessLevel, error)
	ListGrants(ctx context.Context, endpointID string) ([]Grant, error)
	SetGrant(ctx context.Context, endpointID, profileID string, accessLevel devicev1.AccessLevel, now time.Time) error
	DeleteGrant(ctx context.Context, endpointID, profileID string) error
	UpdateEndpointConnection(ctx context.Context, endpoint Endpoint) error
	SetEndpointStatus(ctx context.Context, endpointID string, status devicev1.EndpointState, reason string, seenAt time.Time) error
	RenameEndpoint(ctx context.Context, endpointID, displayName string) error
	DeleteEndpoint(ctx context.Context, endpointID string) error
	CreateCommand(ctx context.Context, command Command) error
	UpdateCommandStatus(ctx context.Context, endpointID, commandID string, status devicev1.CommandStatus, result json.RawMessage, protocolError *devicev1.ProtocolError, updatedAt time.Time) error
	RecordCommandProgress(ctx context.Context, endpointID string, progress devicev1.CommandProgress, updatedAt time.Time) error
	CompleteCommand(ctx context.Context, endpointID string, result devicev1.CommandResult, updatedAt time.Time) error
	ListCommands(ctx context.Context, endpointID, profileID string, limit int) ([]Command, error)
	GetInventory(ctx context.Context, endpointID string) (*devicev1.DeviceInventory, error)
	SaveInventory(ctx context.Context, endpointID string, inventory devicev1.DeviceInventory, updatedAt time.Time) error
	ListInstallations(ctx context.Context, endpointID, profileID string) ([]GameInstallation, error)
	UpdateInstallationLaunchTarget(ctx context.Context, endpointID, gameID, sourceGameID, profileID, launchTarget string, updatedAt time.Time) error
	SetInstallationFailureState(ctx context.Context, endpointID, gameID, sourceGameID, profileID, state, reason, markerID string, ignoredAt *time.Time, ignoredByProfileID, eventType string, details json.RawMessage, updatedAt time.Time) error
}
