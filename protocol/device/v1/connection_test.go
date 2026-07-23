package v1

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestPairingRequestValidate(t *testing.T) {
	t.Parallel()

	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	request := PairingRequest{
		Code:             "test-code",
		ClientInstanceID: "instance-1",
		PublicKey:        base64.RawURLEncoding.EncodeToString(publicKey),
		ClientVersion:    "dev",
		Versions:         SupportedVersionRange(),
		Metadata:         validEndpointMetadata(),
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("PairingRequest.Validate() error = %v", err)
	}
	request.PublicKey = "invalid"
	if err := request.Validate(); err == nil {
		t.Fatal("PairingRequest.Validate() invalid key error = nil, want error")
	}
}

func TestExistingEndpointPairingRequestRequiresAndSignsIdentityProof(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	request := PairingRequest{
		Code:               "profile-grant-code",
		ClientInstanceID:   "instance-1",
		PublicKey:          base64.RawURLEncoding.EncodeToString(publicKey),
		ClientVersion:      "dev",
		Versions:           SupportedVersionRange(),
		Metadata:           validEndpointMetadata(),
		ExistingEndpointID: "endpoint-1",
	}
	if err := request.Validate(); err == nil {
		t.Fatal("Validate() without signature error = nil, want error")
	}
	signingBytes, err := request.SigningBytes()
	if err != nil {
		t.Fatalf("SigningBytes() error = %v", err)
	}
	request.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, signingBytes))
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	repeated, err := request.SigningBytes()
	if err != nil || string(repeated) != string(signingBytes) {
		t.Fatalf("SigningBytes() is unstable: %q != %q, err=%v", repeated, signingBytes, err)
	}
	request.Code = "tampered-code"
	tampered, err := request.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	signature, _ := base64.RawURLEncoding.DecodeString(request.Signature)
	if ed25519.Verify(publicKey, tampered, signature) {
		t.Fatal("signature remained valid after pairing code changed")
	}
}

func TestPairingResponseValidate(t *testing.T) {
	t.Parallel()

	valid := PairingResponse{EndpointID: "endpoint-1", ProtocolVersion: Version, WebSocketURL: "ws://127.0.0.1:8900/api/devices/connect"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("PairingResponse.Validate() error = %v", err)
	}
	valid.WebSocketURL = "https://example.test/connect"
	if err := valid.Validate(); err == nil {
		t.Fatal("PairingResponse.Validate() invalid URL error = nil, want error")
	}
}

func TestAuthChallengeSigningBytesAreStable(t *testing.T) {
	t.Parallel()

	challenge := AuthChallenge{
		ConnectionID: "connection-1",
		Nonce:        base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
		IssuedAt:     time.Date(2026, time.July, 13, 12, 0, 0, 123, time.FixedZone("test", 2*60*60)),
	}
	got, err := challenge.SigningBytes("endpoint-1")
	if err != nil {
		t.Fatalf("SigningBytes() error = %v", err)
	}
	want := "mga-device-v1\nendpoint-1\nconnection-1\n" + challenge.Nonce + "\n2026-07-13T10:00:00.000000123Z"
	if string(got) != want {
		t.Fatalf("SigningBytes() = %q, want %q", got, want)
	}
}

func TestConnectionPayloadValidation(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	_ = publicKey
	challenge := AuthChallenge{
		ConnectionID: "connection-1",
		Nonce:        base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
		IssuedAt:     now,
	}
	message, err := challenge.SigningBytes("endpoint-1")
	if err != nil {
		t.Fatalf("SigningBytes() error = %v", err)
	}
	response := AuthResponse{
		EndpointID:   "endpoint-1",
		ConnectionID: challenge.ConnectionID,
		Signature:    base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, message)),
	}
	if err := response.Validate(); err != nil {
		t.Fatalf("AuthResponse.Validate() error = %v", err)
	}
	accepted := ConnectionAccepted{
		ConnectionID:     "connection-1",
		ProtocolVersion:  Version,
		HeartbeatSeconds: 15,
		ServerTime:       now,
	}
	if err := accepted.Validate(); err != nil {
		t.Fatalf("ConnectionAccepted.Validate() error = %v", err)
	}
	heartbeat := Heartbeat{Sequence: 1, State: EndpointReady, ClientVersion: "dev"}
	if err := heartbeat.Validate(); err != nil {
		t.Fatalf("Heartbeat.Validate() error = %v", err)
	}
	heartbeat.State = EndpointOffline
	if err := heartbeat.Validate(); err == nil || !strings.Contains(err.Error(), "server-derived") {
		t.Fatalf("Heartbeat.Validate() error = %v, want server-derived error", err)
	}
}

func TestEndpointMetadataRejectsDuplicateCapabilities(t *testing.T) {
	t.Parallel()

	metadata := validEndpointMetadata()
	metadata.Capabilities = []string{CapabilityEndpointPing, CapabilityEndpointPing}
	if err := metadata.Validate(); err == nil {
		t.Fatal("EndpointMetadata.Validate() error = nil, want duplicate capability error")
	}
	metadata = validEndpointMetadata()
	metadata.Capabilities = []string{CapabilityEndpointRefresh, CapabilityEndpointPing}
	sorted := metadata.SortedCapabilities()
	if sorted[0] != CapabilityEndpointPing || sorted[1] != CapabilityEndpointRefresh {
		t.Fatalf("SortedCapabilities() = %v", sorted)
	}
}

func validEndpointMetadata() EndpointMetadata {
	return EndpointMetadata{
		DisplayName:  "Test PC / Alice",
		HostName:     "test-pc",
		OSUser:       "alice",
		Platform:     "windows",
		Arch:         "amd64",
		Capabilities: []string{CapabilityEndpointPing, CapabilityEndpointRefresh},
	}
}
