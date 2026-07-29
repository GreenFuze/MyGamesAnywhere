package main

// Steam QR sign-in and token renewal (ADR-0033).
//
// MGA never handles a Steam password. The player scans a QR challenge with the
// Steam mobile app, where password and Steam Guard stay on their own device.
// Steam then returns a long-lived refresh token, and MGA mints short-lived
// access tokens from it for the Steam Families endpoints.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// steamAuthAPIBase is overridable in tests.
var steamAuthAPIBase = steamAPIBase

const (
	// authServiceBegin starts a QR sign-in session.
	authServiceBegin = "IAuthenticationService/BeginAuthSessionViaQR/v1"
	// authServicePoll reports whether the player approved the session yet.
	authServicePoll = "IAuthenticationService/PollAuthSessionStatus/v1"
	// authServiceRenew mints an access token from a refresh token.
	authServiceRenew = "IAuthenticationService/GenerateAccessTokenForApp/v1"

	// platformTypeMobileApp requests a refresh token that Steam's public Web
	// API can exchange for short-lived access tokens. WebBrowser (2) tokens
	// cannot use GenerateAccessTokenForApp, while MobileApp (3) tokens can.
	platformTypeMobileApp = "3"

	// deviceFriendlyName is shown to the player in the Steam app's approval
	// prompt and in their Steam authorized-devices list.
	deviceFriendlyName = "MyGamesAnywhere"
)

// errAuthPending means the player has not approved the QR challenge yet. It is
// a normal polling state, not a failure.
var errAuthPending = errors.New("steam sign-in is still waiting for approval")

// errAuthSessionExpired means the QR challenge is no longer valid and the
// player must start a new sign-in.
var errAuthSessionExpired = errors.New("steam sign-in session expired; start again")

// qrSession is an in-progress QR sign-in.
type qrSession struct {
	ClientID     string `json:"client_id"`
	RequestID    string `json:"request_id"`
	ChallengeURL string `json:"challenge_url"`
	IntervalSecs int    `json:"interval_seconds"`
}

type beginAuthResponse struct {
	Response struct {
		ClientID     string  `json:"client_id"`
		RequestID    string  `json:"request_id"`
		ChallengeURL string  `json:"challenge_url"`
		Interval     float64 `json:"interval"`
	} `json:"response"`
}

type pollAuthResponse struct {
	Response struct {
		RefreshToken string `json:"refresh_token"`
		AccessToken  string `json:"access_token"`
		AccountName  string `json:"account_name"`
		// Pointer preserves the difference between Steam explicitly reporting
		// false while a session is pending and omitting the field for an empty,
		// expired response object.
		HadRemoteInteraction *bool  `json:"had_remote_interaction"`
		NewChallengeURL      string `json:"new_challenge_url"`
	} `json:"response"`
}

type renewTokenResponse struct {
	Response struct {
		AccessToken string `json:"access_token"`
	} `json:"response"`
}

type steamTokenClaims struct {
	Subject  string   `json:"sub"`
	Audience []string `json:"aud"`
}

func (c steamTokenClaims) steamID() (string, error) {
	steamID := strings.TrimSpace(c.Subject)
	if steamID == "" {
		return "", fmt.Errorf("steam token has no account identity")
	}
	if _, err := strconv.ParseUint(steamID, 10, 64); err != nil {
		return "", fmt.Errorf("steam token account identity is invalid")
	}
	if !containsString(c.Audience, "derive") || !containsString(c.Audience, "mobile") {
		return "", fmt.Errorf("steam did not return a renewable mobile token")
	}
	return steamID, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// steamIDFromRefreshToken extracts the provider-authenticated SteamID from the
// JWT payload. The token itself is still treated as opaque and is validated by
// Steam whenever MGA exchanges it; decoding is only used to bind the approved
// account identity instead of trusting a previously configured SteamID.
func steamIDFromRefreshToken(token string) (string, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("steam returned an invalid refresh token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode steam token identity: %w", err)
	}
	var claims steamTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("decode steam token claims: %w", err)
	}
	return claims.steamID()
}

// steamAuthClient talks to Steam's unauthenticated auth service. These calls
// need no Web API key: the refresh token itself is the credential.
type steamAuthClient struct {
	httpClient *http.Client
}

func newSteamAuthClient() *steamAuthClient {
	return &steamAuthClient{httpClient: &http.Client{Timeout: 30 * time.Second}}
}

// post sends a form-encoded request and decodes the JSON response.
func (c *steamAuthClient) post(path string, form url.Values, out any) error {
	endpoint := fmt.Sprintf("%s/%s/", steamAuthAPIBase, path)
	resp, err := c.httpClient.PostForm(endpoint, form)
	if err != nil {
		return fmt.Errorf("steam auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("steam auth %s: status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode steam auth response: %w", err)
	}
	return nil
}

// BeginQRSession starts a QR sign-in and returns the challenge to display.
func (c *steamAuthClient) BeginQRSession() (*qrSession, error) {
	form := url.Values{
		"device_friendly_name": {deviceFriendlyName},
		"platform_type":        {platformTypeMobileApp},
	}
	var result beginAuthResponse
	if err := c.post(authServiceBegin, form, &result); err != nil {
		return nil, err
	}

	response := result.Response
	if strings.TrimSpace(response.ClientID) == "" ||
		strings.TrimSpace(response.RequestID) == "" ||
		strings.TrimSpace(response.ChallengeURL) == "" {
		return nil, fmt.Errorf("steam did not return a usable sign-in challenge")
	}

	interval := int(response.Interval)
	if interval <= 0 {
		interval = 5
	}
	return &qrSession{
		ClientID:     response.ClientID,
		RequestID:    response.RequestID,
		ChallengeURL: response.ChallengeURL,
		IntervalSecs: interval,
	}, nil
}

// PollQRSession reports the outcome of a QR sign-in attempt. It returns
// errAuthPending while the player has not approved yet, and
// errAuthSessionExpired once the challenge is dead.
func (c *steamAuthClient) PollQRSession(clientID, requestID string) (refreshToken, accountName, steamID string, err error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(requestID) == "" {
		return "", "", "", fmt.Errorf("steam sign-in session is incomplete")
	}

	form := url.Values{"client_id": {clientID}, "request_id": {requestID}}
	var result pollAuthResponse
	if err := c.post(authServicePoll, form, &result); err != nil {
		return "", "", "", err
	}

	response := result.Response
	if token := strings.TrimSpace(response.RefreshToken); token != "" {
		steamID, err := steamIDFromRefreshToken(token)
		if err != nil {
			return "", "", "", err
		}
		return token, strings.TrimSpace(response.AccountName), steamID, nil
	}

	// Steam clears the client ID / issues no new challenge once the attempt is
	// dead. A live session explicitly returns had_remote_interaction:false,
	// which must remain distinct from an actually empty response object.
	if strings.TrimSpace(response.NewChallengeURL) == "" && response.HadRemoteInteraction == nil && response.AccessToken == "" && response.AccountName == "" && clientIDCleared(result) {
		return "", "", "", errAuthSessionExpired
	}
	return "", "", "", errAuthPending
}

// clientIDCleared reports whether Steam signalled a dead session. Steam returns
// an empty response object for an expired client ID.
func clientIDCleared(result pollAuthResponse) bool {
	return result.Response == pollAuthResponse{}.Response
}

// AccessTokenFor mints a short-lived access token from a stored refresh token.
func (c *steamAuthClient) AccessTokenFor(refreshToken, steamID string) (string, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return "", fmt.Errorf("steam refresh token is missing")
	}
	if strings.TrimSpace(steamID) == "" {
		return "", fmt.Errorf("steam ID is required to renew an access token")
	}
	tokenSteamID, err := steamIDFromRefreshToken(refreshToken)
	if err != nil {
		return "", errAccessTokenRejected
	}
	if tokenSteamID != strings.TrimSpace(steamID) {
		return "", fmt.Errorf("steam sign-in belongs to a different account")
	}

	form := url.Values{
		"refresh_token": {refreshToken},
		"steamid":       {steamID},
	}
	var result renewTokenResponse
	if err := c.post(authServiceRenew, form, &result); err != nil {
		return "", err
	}
	token := strings.TrimSpace(result.Response.AccessToken)
	if token == "" {
		// A rejected refresh token yields no access token. Surface it as the
		// recoverable "sign in again" condition rather than a hard failure so
		// the owned-games scan is unaffected.
		return "", errAccessTokenRejected
	}
	return token, nil
}
