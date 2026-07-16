package v1

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestClientLaunchRequestValidation(t *testing.T) {
	t.Parallel()

	request := ClientLaunchRequest{
		LaunchID:   "launch-1",
		Token:      base64.RawURLEncoding.EncodeToString([]byte("123456789012345678")),
		EndpointID: "endpoint-1",
		Signature:  base64.RawURLEncoding.EncodeToString(make([]byte, 64)),
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	bytes, err := request.SigningBytes()
	if err != nil {
		t.Fatalf("SigningBytes() error = %v", err)
	}
	if !strings.HasPrefix(string(bytes), clientLaunchSigningContext+"\n") {
		t.Fatalf("SigningBytes() = %q", bytes)
	}

	request.Token = "short"
	if err := request.Validate(); err == nil {
		t.Fatal("Validate() with a short token error = nil, want error")
	}
}

func TestClientLaunchRequestExecutionModeIsSigned(t *testing.T) {
	t.Parallel()

	request := ClientLaunchRequest{
		LaunchID:      "launch-1",
		Token:         base64.RawURLEncoding.EncodeToString([]byte("123456789012345678")),
		EndpointID:    "endpoint-1",
		ExecutionMode: ClientExecutionModeElevated,
	}
	elevated, err := request.SigningBytes()
	if err != nil {
		t.Fatalf("elevated SigningBytes() error = %v", err)
	}
	request.ExecutionMode = ClientExecutionModeStandard
	standard, err := request.SigningBytes()
	if err != nil {
		t.Fatalf("standard SigningBytes() error = %v", err)
	}
	if string(elevated) == string(standard) {
		t.Fatal("execution mode did not change signed bytes")
	}
}
