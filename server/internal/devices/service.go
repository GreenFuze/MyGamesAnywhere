package devices

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/google/uuid"
)

var (
	ErrInvalidPairingCode  = errors.New("pairing code is invalid, expired, or already used")
	ErrClientAlreadyPaired = errors.New("this MGA Client installation is already paired")
	ErrEndpointNotFound    = errors.New("device endpoint not found")
	ErrDeviceForbidden     = errors.New("profile does not have access to this device endpoint")
	ErrEndpointOffline     = errors.New("device endpoint is offline")
	ErrCapabilityMissing   = errors.New("device endpoint does not advertise the required capability")
	ErrCommandNotFound     = errors.New("device command does not belong to this endpoint")
	ErrGrantNotFound       = errors.New("device grant not found")
	ErrLastOwner           = errors.New("device endpoint must retain at least one owner")
)

const pairingLifetime = 10 * time.Minute

type Service struct {
	store    Store
	hub      *Hub
	launches *ClientLaunchRegistry
	now      func() time.Time
}

func NewService(store Store, hub *Hub) (*Service, error) {
	if store == nil {
		return nil, errors.New("device store is required")
	}
	if hub == nil {
		return nil, errors.New("device connection hub is required")
	}
	return &Service{store: store, hub: hub, launches: NewClientLaunchRegistry(), now: time.Now}, nil
}

func (s *Service) CreateClientLaunch(profileID string) (string, ClientLaunch, error) {
	return s.launches.Create(profileID, s.now())
}

func (s *Service) GetClientLaunch(id, profileID string) (ClientLaunch, error) {
	return s.launches.Get(id, profileID, s.now())
}

func (s *Service) RedeemClientLaunch(ctx context.Context, request devicev1.ClientLaunchRequest) (ClientLaunch, error) {
	if err := request.Validate(); err != nil {
		return ClientLaunch{}, err
	}
	launch, err := s.launches.GetForRedemption(request.LaunchID, s.now())
	if err != nil {
		return ClientLaunch{}, err
	}
	if err := s.requireAccess(ctx, request.EndpointID, launch.ProfileID, devicev1.AccessView); err != nil {
		return ClientLaunch{}, ErrClientLaunchNotFound
	}
	endpoint, err := s.EndpointForConnection(ctx, request.EndpointID)
	if err != nil {
		return ClientLaunch{}, ErrClientLaunchNotFound
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(endpoint.PublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return ClientLaunch{}, errors.New("paired endpoint has an invalid public key")
	}
	signingBytes, err := request.SigningBytes()
	if err != nil {
		return ClientLaunch{}, err
	}
	signature, _ := base64.RawURLEncoding.DecodeString(request.Signature)
	if !ed25519.Verify(ed25519.PublicKey(publicKey), signingBytes, signature) {
		return ClientLaunch{}, ErrClientLaunchNotFound
	}
	return s.launches.Redeem(request.LaunchID, request.Token, request.EndpointID, s.now())
}

func (s *Service) CreatePairingChallenge(ctx context.Context, profileID string) (string, PairingChallenge, error) {
	if strings.TrimSpace(profileID) == "" {
		return "", PairingChallenge{}, errors.New("profile_id is required")
	}
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", PairingChallenge{}, fmt.Errorf("generate pairing code: %w", err)
	}
	code := base64.RawURLEncoding.EncodeToString(raw)
	now := s.now()
	challenge := PairingChallenge{
		ID:        uuid.NewString(),
		CodeHash:  hashPairingCode(code),
		ProfileID: profileID,
		CreatedAt: now,
		ExpiresAt: now.Add(pairingLifetime),
	}
	if err := s.store.CreatePairingChallenge(ctx, challenge); err != nil {
		return "", PairingChallenge{}, err
	}
	return code, challenge, nil
}

func (s *Service) Pair(ctx context.Context, request devicev1.PairingRequest, websocketURL string) (devicev1.PairingResponse, error) {
	if err := request.Validate(); err != nil {
		return devicev1.PairingResponse{}, err
	}
	selected, err := devicev1.NegotiateVersion(request.Versions, devicev1.SupportedVersionRange())
	if err != nil {
		return devicev1.PairingResponse{}, err
	}
	now := s.now()
	endpoint := Endpoint{
		ID:               uuid.NewString(),
		ClientInstanceID: request.ClientInstanceID,
		PublicKey:        request.PublicKey,
		DisplayName:      strings.TrimSpace(request.Metadata.DisplayName),
		HostName:         strings.TrimSpace(request.Metadata.HostName),
		OSUser:           strings.TrimSpace(request.Metadata.OSUser),
		Platform:         strings.TrimSpace(request.Metadata.Platform),
		Arch:             strings.TrimSpace(request.Metadata.Arch),
		ClientVersion:    strings.TrimSpace(request.ClientVersion),
		ProtocolVersion:  selected,
		Capabilities:     request.Metadata.SortedCapabilities(),
		Status:           devicev1.EndpointOffline,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if _, err := s.store.PairEndpoint(ctx, hashPairingCode(request.Code), now, endpoint); err != nil {
		return devicev1.PairingResponse{}, err
	}
	response := devicev1.PairingResponse{EndpointID: endpoint.ID, ProtocolVersion: selected, WebSocketURL: websocketURL}
	if err := response.Validate(); err != nil {
		return devicev1.PairingResponse{}, err
	}
	return response, nil
}

func (s *Service) ListEndpoints(ctx context.Context, profileID string) ([]Endpoint, error) {
	return s.store.ListEndpoints(ctx, profileID)
}

func (s *Service) EndpointForConnection(ctx context.Context, endpointID string) (*Endpoint, error) {
	endpoint, err := s.store.GetEndpoint(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	if endpoint == nil {
		return nil, ErrEndpointNotFound
	}
	return endpoint, nil
}

func (s *Service) MarkConnected(ctx context.Context, endpoint *Endpoint, hello devicev1.Hello, selected devicev1.ProtocolVersion) error {
	if endpoint == nil {
		return ErrEndpointNotFound
	}
	now := s.now()
	endpoint.HostName = hello.Metadata.HostName
	endpoint.OSUser = hello.Metadata.OSUser
	endpoint.Platform = hello.Metadata.Platform
	endpoint.Arch = hello.Metadata.Arch
	endpoint.ClientVersion = hello.ClientVersion
	endpoint.ProtocolVersion = selected
	endpoint.Capabilities = hello.Metadata.SortedCapabilities()
	endpoint.Status = devicev1.EndpointReady
	endpoint.StatusReason = ""
	endpoint.LastSeenAt = &now
	endpoint.UpdatedAt = now
	return s.store.UpdateEndpointConnection(ctx, *endpoint)
}

func (s *Service) RecordHeartbeat(ctx context.Context, endpointID string, heartbeat devicev1.Heartbeat) error {
	if err := heartbeat.Validate(); err != nil {
		return err
	}
	return s.store.SetEndpointStatus(ctx, endpointID, heartbeat.State, heartbeat.StateReason, s.now())
}

func (s *Service) MarkOffline(ctx context.Context, endpointID string) error {
	return s.store.SetEndpointStatus(ctx, endpointID, devicev1.EndpointOffline, "", s.now())
}

func (s *Service) RenameEndpoint(ctx context.Context, endpointID, profileID, displayName string) error {
	if err := s.requireAccess(ctx, endpointID, profileID, devicev1.AccessOwner); err != nil {
		return err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 100 {
		return errors.New("display_name must contain 1 to 100 characters")
	}
	return s.store.RenameEndpoint(ctx, endpointID, displayName)
}

func (s *Service) RevokeEndpoint(ctx context.Context, endpointID, profileID string) error {
	if err := s.requireAccess(ctx, endpointID, profileID, devicev1.AccessOwner); err != nil {
		return err
	}
	s.hub.Disconnect(endpointID)
	return s.store.DeleteEndpoint(ctx, endpointID)
}

func (s *Service) ListGrants(ctx context.Context, endpointID, profileID string) ([]Grant, error) {
	if err := s.requireAccess(ctx, endpointID, profileID, devicev1.AccessOwner); err != nil {
		return nil, err
	}
	return s.store.ListGrants(ctx, endpointID)
}

func (s *Service) SetGrant(ctx context.Context, endpointID, actorProfileID, targetProfileID string, accessLevel devicev1.AccessLevel) error {
	if err := s.requireAccess(ctx, endpointID, actorProfileID, devicev1.AccessOwner); err != nil {
		return err
	}
	if strings.TrimSpace(targetProfileID) == "" {
		return errors.New("target profile_id is required")
	}
	if err := accessLevel.Validate(); err != nil {
		return err
	}
	return s.store.SetGrant(ctx, endpointID, targetProfileID, accessLevel, s.now())
}

func (s *Service) DeleteGrant(ctx context.Context, endpointID, actorProfileID, targetProfileID string) error {
	if err := s.requireAccess(ctx, endpointID, actorProfileID, devicev1.AccessOwner); err != nil {
		return err
	}
	return s.store.DeleteGrant(ctx, endpointID, targetProfileID)
}

func (s *Service) DispatchCommand(ctx context.Context, endpointID, profileID, name string, payload json.RawMessage) (*Command, error) {
	required, err := requiredAccessForCommand(name)
	if err != nil {
		return nil, err
	}
	if err := s.requireAccess(ctx, endpointID, profileID, required); err != nil {
		return nil, err
	}
	endpoint, err := s.EndpointForConnection(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	if !hasCapability(endpoint.Capabilities, name) {
		return nil, fmt.Errorf("%w: %s", ErrCapabilityMissing, name)
	}
	if !s.hub.IsConnected(endpointID) {
		return nil, ErrEndpointOffline
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	now := s.now()
	command := &Command{
		ID:             uuid.NewString(),
		EndpointID:     endpointID,
		ProfileID:      profileID,
		Name:           name,
		SchemaVersion:  1,
		IdempotencyKey: uuid.NewString(),
		Status:         devicev1.CommandDispatched,
		Payload:        payload,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpiresAt:      now.Add(2 * time.Minute),
	}
	request := devicev1.CommandRequest{
		CommandID:      command.ID,
		IdempotencyKey: command.IdempotencyKey,
		Name:           command.Name,
		SchemaVersion:  command.SchemaVersion,
		RequiredLevel:  required,
		Authorization: devicev1.AuthorizationContext{
			ProfileID:    profileID,
			GrantedLevel: required,
		},
		CreatedAt: now,
		ExpiresAt: command.ExpiresAt,
		Payload:   payload,
	}
	if err := request.ValidateAt(now); err != nil {
		return nil, err
	}
	if err := s.store.CreateCommand(ctx, *command); err != nil {
		return nil, err
	}
	envelope, err := devicev1.NewEnvelope(devicev1.MessageCommandRequest, uuid.NewString(), command.ID, now, request)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	if err := s.hub.Send(ctx, endpointID, data); err != nil {
		protocolError := &devicev1.ProtocolError{Code: "endpoint_offline", Message: err.Error(), Retryable: true}
		_ = s.store.UpdateCommandStatus(ctx, endpointID, command.ID, devicev1.CommandRejected, nil, protocolError, s.now())
		return nil, err
	}
	return command, nil
}

func (s *Service) RecordCommandResult(ctx context.Context, endpointID string, result devicev1.CommandResult) error {
	if err := result.Validate(); err != nil {
		return err
	}
	return s.store.UpdateCommandStatus(ctx, endpointID, result.CommandID, result.Status, result.Payload, result.Error, s.now())
}

func (s *Service) RecordCommandStatus(ctx context.Context, endpointID, commandID string, status devicev1.CommandStatus) error {
	return s.store.UpdateCommandStatus(ctx, endpointID, commandID, status, nil, nil, s.now())
}

func (s *Service) ListCommands(ctx context.Context, endpointID, profileID string) ([]Command, error) {
	if err := s.requireAccess(ctx, endpointID, profileID, devicev1.AccessView); err != nil {
		return nil, err
	}
	return s.store.ListCommands(ctx, endpointID, profileID, 20)
}

func (s *Service) requireAccess(ctx context.Context, endpointID, profileID string, required devicev1.AccessLevel) error {
	granted, err := s.store.GetGrant(ctx, endpointID, profileID)
	if err != nil {
		return err
	}
	allowed, err := granted.Allows(required)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrDeviceForbidden
	}
	return nil
}

func requiredAccessForCommand(name string) (devicev1.AccessLevel, error) {
	switch strings.TrimSpace(name) {
	case devicev1.CapabilityEndpointPing:
		return devicev1.AccessView, nil
	case devicev1.CapabilityEndpointRefresh:
		return devicev1.AccessManage, nil
	case devicev1.CapabilityEndpointStop:
		return devicev1.AccessManage, nil
	default:
		return "", fmt.Errorf("unsupported device command %q", name)
	}
}

func hasCapability(capabilities []string, required string) bool {
	for _, capability := range capabilities {
		if capability == required {
			return true
		}
	}
	return false
}

func hashPairingCode(code string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
