package v1

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const clientLaunchSigningContext = "mga-client-launch-v1"

// ClientLaunchRequest proves that the installed per-user client opened a
// short-lived mga://start challenge. It never carries a reusable credential.
type ClientLaunchRequest struct {
	LaunchID   string `json:"launch_id"`
	Token      string `json:"token"`
	EndpointID string `json:"endpoint_id"`
	Signature  string `json:"signature"`
}

// SigningBytes returns the canonical launch proof signed by the endpoint key.
func (r ClientLaunchRequest) SigningBytes() ([]byte, error) {
	if strings.TrimSpace(r.LaunchID) == "" {
		return nil, errors.New("launch_id is required")
	}
	token, err := base64.RawURLEncoding.DecodeString(r.Token)
	if err != nil || len(token) < 18 {
		return nil, errors.New("token must contain at least 18 base64url bytes")
	}
	if strings.TrimSpace(r.EndpointID) == "" {
		return nil, errors.New("endpoint_id is required")
	}
	return []byte(fmt.Sprintf("%s\n%s\n%s\n%s", clientLaunchSigningContext, r.LaunchID, r.Token, r.EndpointID)), nil
}

// Validate rejects malformed launch proofs before authorization checks.
func (r ClientLaunchRequest) Validate() error {
	if _, err := r.SigningBytes(); err != nil {
		return err
	}
	signature, err := base64.RawURLEncoding.DecodeString(r.Signature)
	if err != nil || len(signature) != 64 {
		return errors.New("signature must be a base64url Ed25519 signature")
	}
	return nil
}
