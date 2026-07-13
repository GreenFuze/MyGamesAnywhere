package devices

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type launchTestStore struct {
	endpoint Endpoint
	grant    devicev1.AccessLevel
}

func (s *launchTestStore) GetEndpoint(_ context.Context, endpointID string) (*Endpoint, error) {
	if endpointID != s.endpoint.ID {
		return nil, nil
	}
	endpoint := s.endpoint
	return &endpoint, nil
}

func (s *launchTestStore) GetGrant(_ context.Context, endpointID, profileID string) (devicev1.AccessLevel, error) {
	if endpointID != s.endpoint.ID || profileID != "profile-1" {
		return "", ErrDeviceForbidden
	}
	return s.grant, nil
}

func (*launchTestStore) CreatePairingChallenge(context.Context, PairingChallenge) error {
	return errors.New("unexpected call")
}
func (*launchTestStore) PairEndpoint(context.Context, string, time.Time, Endpoint) (string, error) {
	return "", errors.New("unexpected call")
}
func (*launchTestStore) ListEndpoints(context.Context, string) ([]Endpoint, error) {
	return nil, errors.New("unexpected call")
}
func (*launchTestStore) ListGrants(context.Context, string) ([]Grant, error) {
	return nil, errors.New("unexpected call")
}
func (*launchTestStore) SetGrant(context.Context, string, string, devicev1.AccessLevel, time.Time) error {
	return errors.New("unexpected call")
}
func (*launchTestStore) DeleteGrant(context.Context, string, string) error {
	return errors.New("unexpected call")
}
func (*launchTestStore) UpdateEndpointConnection(context.Context, Endpoint) error {
	return errors.New("unexpected call")
}
func (*launchTestStore) SetEndpointStatus(context.Context, string, devicev1.EndpointState, string, time.Time) error {
	return errors.New("unexpected call")
}
func (*launchTestStore) RenameEndpoint(context.Context, string, string) error {
	return errors.New("unexpected call")
}
func (*launchTestStore) DeleteEndpoint(context.Context, string) error {
	return errors.New("unexpected call")
}
func (*launchTestStore) CreateCommand(context.Context, Command) error {
	return errors.New("unexpected call")
}
func (*launchTestStore) UpdateCommandStatus(context.Context, string, string, devicev1.CommandStatus, json.RawMessage, *devicev1.ProtocolError, time.Time) error {
	return errors.New("unexpected call")
}
func (*launchTestStore) ListCommands(context.Context, string, string, int) ([]Command, error) {
	return nil, errors.New("unexpected call")
}

func TestServiceRedeemsSignedClientLaunch(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	store := &launchTestStore{
		endpoint: Endpoint{ID: "endpoint-1", PublicKey: base64.RawURLEncoding.EncodeToString(publicKey)},
		grant:    devicev1.AccessOwner,
	}
	service, err := NewService(store, NewHub())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	token, launch, err := service.CreateClientLaunch("profile-1")
	if err != nil {
		t.Fatalf("CreateClientLaunch() error = %v", err)
	}
	request := devicev1.ClientLaunchRequest{LaunchID: launch.ID, Token: token, EndpointID: store.endpoint.ID}
	signingBytes, err := request.SigningBytes()
	if err != nil {
		t.Fatalf("SigningBytes() error = %v", err)
	}
	request.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, signingBytes))

	redeemed, err := service.RedeemClientLaunch(context.Background(), request)
	if err != nil {
		t.Fatalf("RedeemClientLaunch() error = %v", err)
	}
	if redeemed.EndpointID != store.endpoint.ID || redeemed.Status != ClientLaunchAcknowledged {
		t.Fatalf("RedeemClientLaunch() = %#v", redeemed)
	}
}

func TestClientStopRequiresManageAccess(t *testing.T) {
	t.Parallel()

	level, err := requiredAccessForCommand(devicev1.CapabilityEndpointStop)
	if err != nil {
		t.Fatalf("requiredAccessForCommand() error = %v", err)
	}
	if level != devicev1.AccessManage {
		t.Fatalf("requiredAccessForCommand() = %q, want %q", level, devicev1.AccessManage)
	}
}
