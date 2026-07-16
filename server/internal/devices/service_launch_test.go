package devices

import (
	"bytes"
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
	endpoint      Endpoint
	grant         devicev1.AccessLevel
	installations []GameInstallation
	updatedTarget string
	failureState  string
	failureEvent  string
	failureMarker string
	ignoredBy     string
}

func (s *launchTestStore) GetEndpoint(_ context.Context, endpointID string) (*Endpoint, error) {
	if endpointID != s.endpoint.ID {
		return nil, nil
	}
	endpoint := s.endpoint
	return &endpoint, nil
}

func TestCommandPayloadForAuditRedactsArchiveDownloadToken(t *testing.T) {
	t.Parallel()
	payload, err := json.Marshal(devicev1.ArchiveInstallRequest{
		GameID: "game-1", SourceGameID: "source-1", Title: "Game", ArchiveName: "game.zip",
		ArchiveFormat: "zip", ArchiveSize: 10, DownloadURL: "http://server/archive",
		DownloadToken: "secret-grant", DestinationRoot: `%USERPROFILE%\Games`, DestinationName: "Game",
	})
	if err != nil {
		t.Fatal(err)
	}
	redacted, err := commandPayloadForAudit(devicev1.CapabilityGameInstallArchive, payload)
	if err != nil {
		t.Fatalf("commandPayloadForAudit() error = %v", err)
	}
	if string(redacted) == string(payload) || string(redacted) == "" {
		t.Fatalf("redacted payload = %s", redacted)
	}
	var request devicev1.ArchiveInstallRequest
	if err := json.Unmarshal(redacted, &request); err != nil {
		t.Fatal(err)
	}
	if request.DownloadToken != "[redacted]" || bytes.Contains(redacted, []byte("secret-grant")) {
		t.Fatalf("download token was not redacted: %s", redacted)
	}
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
func (*launchTestStore) RecordCommandProgress(context.Context, string, devicev1.CommandProgress, time.Time) error {
	return errors.New("unexpected call")
}
func (*launchTestStore) CompleteCommand(context.Context, string, devicev1.CommandResult, time.Time) error {
	return errors.New("unexpected call")
}
func (*launchTestStore) ListCommands(context.Context, string, string, int) ([]Command, error) {
	return nil, errors.New("unexpected call")
}
func (*launchTestStore) GetInventory(context.Context, string) (*devicev1.DeviceInventory, error) {
	return nil, errors.New("unexpected call")
}
func (*launchTestStore) SaveInventory(context.Context, string, devicev1.DeviceInventory, time.Time) error {
	return errors.New("unexpected call")
}
func (s *launchTestStore) ListInstallations(context.Context, string, string) ([]GameInstallation, error) {
	return s.installations, nil
}
func (s *launchTestStore) UpdateInstallationLaunchTarget(_ context.Context, _, _, _, _, launchTarget string, _ time.Time) error {
	s.updatedTarget = launchTarget
	return nil
}
func (s *launchTestStore) SetInstallationFailureState(_ context.Context, _, _, _, _ string, state, _ string, markerID string, _ *time.Time, ignoredBy, eventType string, _ json.RawMessage, _ time.Time) error {
	s.failureState = state
	s.failureEvent = eventType
	s.failureMarker = markerID
	s.ignoredBy = ignoredBy
	return nil
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

func TestServiceRejectsClientLaunchRedeemedWithDifferentExecutionMode(t *testing.T) {
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
	service.now = func() time.Time { return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC) }
	token, launch, err := service.CreateClientLaunchWithMode("profile-1", devicev1.ClientExecutionModeElevated)
	if err != nil {
		t.Fatalf("CreateClientLaunchWithMode() error = %v", err)
	}
	request := devicev1.ClientLaunchRequest{
		LaunchID:      launch.ID,
		Token:         token,
		EndpointID:    store.endpoint.ID,
		ExecutionMode: devicev1.ClientExecutionModeStandard,
	}
	signingBytes, err := request.SigningBytes()
	if err != nil {
		t.Fatalf("SigningBytes() error = %v", err)
	}
	request.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, signingBytes))

	if _, err := service.RedeemClientLaunch(context.Background(), request); !errors.Is(err, ErrClientLaunchNotFound) {
		t.Fatalf("RedeemClientLaunch() error = %v, want ErrClientLaunchNotFound", err)
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

func TestFailedInstallCleanupRequiresManageAccess(t *testing.T) {
	t.Parallel()
	level, err := requiredAccessForCommand(devicev1.CapabilityGameCleanupGogInnoFailed)
	if err != nil {
		t.Fatalf("requiredAccessForCommand() error = %v", err)
	}
	if level != devicev1.AccessManage {
		t.Fatalf("requiredAccessForCommand() = %q, want %q", level, devicev1.AccessManage)
	}
}

func TestGameLaunchRequiresPlayAccess(t *testing.T) {
	t.Parallel()
	level, err := requiredAccessForCommand(devicev1.CapabilityGameLaunch)
	if err != nil {
		t.Fatalf("requiredAccessForCommand() error = %v", err)
	}
	if level != devicev1.AccessPlay {
		t.Fatalf("requiredAccessForCommand() = %q, want %q", level, devicev1.AccessPlay)
	}
}

func TestServiceSelectsOnlyRecordedLaunchCandidate(t *testing.T) {
	t.Parallel()
	store := &launchTestStore{
		endpoint: Endpoint{ID: "endpoint-1"},
		grant:    devicev1.AccessManage,
		installations: []GameInstallation{{
			EndpointID: "endpoint-1", GameID: "game-1", SourceGameID: "source-1", ProfileID: "profile-1",
			LaunchCandidates: []string{"Game/Game.exe", "Game/alternate.exe"},
		}},
	}
	service, err := NewService(store, NewHub())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := service.SetInstallationLaunchTarget(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1", `game\GAME.exe`); err != nil {
		t.Fatalf("SetInstallationLaunchTarget() error = %v", err)
	}
	if store.updatedTarget != "Game/Game.exe" {
		t.Fatalf("updated launch target = %q", store.updatedTarget)
	}
	store.updatedTarget = ""
	if err := service.SetInstallationLaunchTarget(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1", "Game/not-recorded.exe"); err == nil {
		t.Fatal("SetInstallationLaunchTarget() accepted an unrecorded executable")
	}
	if store.updatedTarget != "" {
		t.Fatalf("unrecorded target was persisted as %q", store.updatedTarget)
	}
}

func TestServiceFailedInstallIgnoreReopenAndCleanupTransitions(t *testing.T) {
	t.Parallel()
	store := &launchTestStore{
		endpoint: Endpoint{ID: "endpoint-1"}, grant: devicev1.AccessManage,
		installations: []GameInstallation{{
			EndpointID: "endpoint-1", GameID: "game-1", SourceGameID: "source-1", ProfileID: "profile-1",
			InstallState: devicev1.InstallStateCleanupRequired, CleanupMarkerID: "marker-1",
		}},
	}
	service, err := NewService(store, NewHub())
	if err != nil {
		t.Fatal(err)
	}
	ignored, err := service.TransitionInstallationFailure(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1", "ignore", "")
	if err != nil {
		t.Fatalf("ignore transition: %v", err)
	}
	if ignored.InstallState != devicev1.InstallStateIgnoredFailure || store.failureState != devicev1.InstallStateIgnoredFailure || store.failureEvent != "failure_ignored" || store.ignoredBy != "profile-1" {
		t.Fatalf("ignored=%#v store=%#v", ignored, store)
	}
	store.installations[0] = *ignored
	reopened, err := service.TransitionInstallationFailure(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1", "reopen", "")
	if err != nil {
		t.Fatalf("reopen transition: %v", err)
	}
	if reopened.InstallState != devicev1.InstallStateCleanupRequired || store.failureEvent != "failure_reopened" || store.failureMarker != "marker-1" {
		t.Fatalf("reopened=%#v store=%#v", reopened, store)
	}
	store.installations[0] = *reopened
	running, err := service.TransitionInstallationFailure(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1", "cleanup_started", "Cleanup requested")
	if err != nil || running.InstallState != devicev1.InstallStateCleanupRunning || store.failureEvent != "cleanup_started" {
		t.Fatalf("cleanup start=%#v error=%v store=%#v", running, err, store)
	}
}

func TestServiceLegacyAttentionCanBeIgnoredButNotCleaned(t *testing.T) {
	t.Parallel()
	store := &launchTestStore{
		endpoint: Endpoint{ID: "endpoint-1"}, grant: devicev1.AccessManage,
		installations: []GameInstallation{{
			EndpointID: "endpoint-1", GameID: "game-1", SourceGameID: "source-1", ProfileID: "profile-1",
			InstallState: devicev1.InstallStateAttentionRequired,
		}},
	}
	service, err := NewService(store, NewHub())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.TransitionInstallationFailure(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1", "cleanup_started", ""); err == nil {
		t.Fatal("legacy attention row without a marker was eligible for cleanup")
	}
	if _, err := service.TransitionInstallationFailure(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1", "ignore", ""); err != nil {
		t.Fatalf("ignore legacy attention row: %v", err)
	}
	store.grant = devicev1.AccessView
	if _, err := service.TransitionInstallationFailure(context.Background(), "endpoint-1", "game-1", "source-1", "profile-1", "ignore", ""); !errors.Is(err, ErrDeviceForbidden) {
		t.Fatalf("view-only ignore error = %v, want ErrDeviceForbidden", err)
	}
}
