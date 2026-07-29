package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// startAuthTestServer serves the IAuthenticationService endpoints with canned
// responses keyed by method name.
func startAuthTestServer(t *testing.T, bodies map[string]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for method, body := range bodies {
			if strings.Contains(r.URL.Path, method) {
				if method == "BeginAuthSessionViaQR" && r.FormValue("platform_type") != platformTypeMobileApp {
					t.Errorf("platform_type = %q, want MobileApp %q", r.FormValue("platform_type"), platformTypeMobileApp)
				}
				if body == "" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				fmt.Fprint(w, body)
				return
			}
		}
		t.Errorf("unexpected auth path: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	original := steamAuthAPIBase
	steamAuthAPIBase = server.URL
	t.Cleanup(func() { steamAuthAPIBase = original })
	return server
}

func testSteamRefreshToken(t *testing.T, steamID string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, err := json.Marshal(map[string]any{
		"sub": steamID,
		"aud": []string{"web", "mobile", "derive"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".test-signature"
}

func TestBeginQRSessionReturnsChallenge(t *testing.T) {
	startAuthTestServer(t, map[string]string{
		"BeginAuthSessionViaQR": `{"response":{"client_id":"c1","request_id":"r1","challenge_url":"https://s.team/q/abc","interval":5.0}}`,
	})

	session, err := newSteamAuthClient().BeginQRSession()
	if err != nil {
		t.Fatalf("BeginQRSession error: %v", err)
	}
	if session.ClientID != "c1" || session.RequestID != "r1" {
		t.Fatalf("session identity = %+v", session)
	}
	if session.ChallengeURL != "https://s.team/q/abc" {
		t.Fatalf("challenge URL = %q", session.ChallengeURL)
	}
	if session.IntervalSecs != 5 {
		t.Fatalf("interval = %d, want 5", session.IntervalSecs)
	}
}

func TestBeginQRSessionRejectsIncompleteChallenge(t *testing.T) {
	startAuthTestServer(t, map[string]string{
		"BeginAuthSessionViaQR": `{"response":{"client_id":"c1"}}`,
	})

	if _, err := newSteamAuthClient().BeginQRSession(); err == nil {
		t.Fatal("expected an error for a challenge with no URL/request ID")
	}
}

func TestPollQRSessionPendingThenApproved(t *testing.T) {
	// Still waiting: Steam's real pre-scan response explicitly reports false.
	// Presence of the field distinguishes it from an expired empty response.
	startAuthTestServer(t, map[string]string{
		"PollAuthSessionStatus": `{"response":{"had_remote_interaction":false}}`,
	})
	if _, _, _, err := newSteamAuthClient().PollQRSession("c1", "r1"); !errors.Is(err, errAuthPending) {
		t.Fatalf("pending poll error = %v, want errAuthPending", err)
	}

	// Approved: a refresh token is returned.
	refreshToken := testSteamRefreshToken(t, "76561198000000001")
	startAuthTestServer(t, map[string]string{
		"PollAuthSessionStatus": fmt.Sprintf(`{"response":{"refresh_token":%q,"account_name":"orr"}}`, refreshToken),
	})
	token, account, steamID, err := newSteamAuthClient().PollQRSession("c1", "r1")
	if err != nil {
		t.Fatalf("approved poll error: %v", err)
	}
	if token != refreshToken || account != "orr" || steamID != "76561198000000001" {
		t.Fatalf("poll result = (%q, %q, %q)", token, account, steamID)
	}
}

func TestPollQRSessionExpiredSession(t *testing.T) {
	// Steam returns an empty response object once the attempt is dead.
	startAuthTestServer(t, map[string]string{
		"PollAuthSessionStatus": `{"response":{}}`,
	})
	if _, _, _, err := newSteamAuthClient().PollQRSession("c1", "r1"); !errors.Is(err, errAuthSessionExpired) {
		t.Fatalf("expired poll error = %v, want errAuthSessionExpired", err)
	}
}

func TestPollQRSessionRequiresSessionIdentity(t *testing.T) {
	if _, _, _, err := newSteamAuthClient().PollQRSession("", "r1"); err == nil {
		t.Fatal("expected an error for a missing client ID")
	}
}

func TestAccessTokenForMintsAndRejects(t *testing.T) {
	startAuthTestServer(t, map[string]string{
		"GenerateAccessTokenForApp": `{"response":{"access_token":"fresh-token"}}`,
	})
	token, err := newSteamAuthClient().AccessTokenFor(testSteamRefreshToken(t, "76561198000000001"), "76561198000000001")
	if err != nil {
		t.Fatalf("AccessTokenFor error: %v", err)
	}
	if token != "fresh-token" {
		t.Fatalf("token = %q, want fresh-token", token)
	}
	if _, err := newSteamAuthClient().AccessTokenFor(
		testSteamRefreshToken(t, "76561198000000001"),
		"76561198000000002",
	); err == nil || !strings.Contains(err.Error(), "different account") {
		t.Fatalf("mismatched account error = %v", err)
	}

	// A dead refresh token yields no access token: recoverable, not fatal.
	startAuthTestServer(t, map[string]string{
		"GenerateAccessTokenForApp": `{"response":{}}`,
	})
	if _, err := newSteamAuthClient().AccessTokenFor("dead", "76561198000000001"); !errors.Is(err, errAccessTokenRejected) {
		t.Fatalf("dead refresh error = %v, want errAccessTokenRejected", err)
	}

	// Missing inputs fail fast without a network call.
	client := newSteamAuthClient()
	if _, err := client.AccessTokenFor("", "steam"); err == nil {
		t.Fatal("expected an error for a missing refresh token")
	}
	if _, err := client.AccessTokenFor("refresh", ""); err == nil {
		t.Fatal("expected an error for a missing Steam ID")
	}
}

func TestHandleQRBeginAndPollIPC(t *testing.T) {
	startAuthTestServer(t, map[string]string{
		"BeginAuthSessionViaQR": `{"response":{"client_id":"c1","request_id":"r1","challenge_url":"https://s.team/q/abc","interval":5.0}}`,
	})
	result, errObj := handleQRBegin(nil)
	if errObj != nil {
		t.Fatalf("handleQRBegin error: %+v", errObj)
	}
	begun, _ := result.(map[string]any)
	if begun["challenge_url"] != "https://s.team/q/abc" || begun["status"] != "pending" {
		t.Fatalf("begin result = %+v", begun)
	}

	// Approved poll returns config updates for the server to persist.
	refreshToken := testSteamRefreshToken(t, "76561198000000001")
	startAuthTestServer(t, map[string]string{
		"PollAuthSessionStatus": fmt.Sprintf(`{"response":{"refresh_token":%q,"account_name":"orr"}}`, refreshToken),
	})
	params := json.RawMessage(`{"client_id":"c1","request_id":"r1"}`)
	result, errObj = handleQRPoll(params)
	if errObj != nil {
		t.Fatalf("handleQRPoll error: %+v", errObj)
	}
	polled, _ := result.(map[string]any)
	if polled["status"] != "ok" {
		t.Fatalf("poll status = %v, want ok", polled["status"])
	}
	updates, ok := polled["config_updates"].(map[string]any)
	if !ok {
		t.Fatalf("config_updates = %#v, want map", polled["config_updates"])
	}
	if updates["refresh_token"] != refreshToken {
		t.Fatalf("refresh_token update = %#v", updates["refresh_token"])
	}
	if updates["steam_id"] != "76561198000000001" {
		t.Fatalf("steam_id update = %#v", updates["steam_id"])
	}
	identity, ok := updates["provider_identity"].(providerIdentity)
	if !ok || identity.Subject != "76561198000000001" || identity.DisplayName != "orr" {
		t.Fatalf("provider_identity = %#v", updates["provider_identity"])
	}
}

func TestFetchSteamIdentityReturnsPersonaAndAvatar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISteamUser/GetPlayerSummaries/v2/" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "api-key" || r.URL.Query().Get("steamids") != "76561198000000001" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"response":{"players":[{"steamid":"76561198000000001","personaname":"Orr Player","avatarfull":"https://cdn.example/orr.jpg"}]}}`)
	}))
	t.Cleanup(server.Close)
	original := steamProfileAPIBase
	steamProfileAPIBase = server.URL
	t.Cleanup(func() { steamProfileAPIBase = original })

	identity := fetchSteamIdentity("api-key", "76561198000000001", "login-name")
	if identity.DisplayName != "Orr Player" || identity.AvatarURL != "https://cdn.example/orr.jpg" {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestHandleQRPollSurfacesPendingAndExpired(t *testing.T) {
	startAuthTestServer(t, map[string]string{
		"PollAuthSessionStatus": `{"response":{"had_remote_interaction":true}}`,
	})
	result, errObj := handleQRPoll(json.RawMessage(`{"client_id":"c1","request_id":"r1"}`))
	if errObj != nil {
		t.Fatalf("pending poll returned an error: %+v", errObj)
	}
	if pending, _ := result.(map[string]any); pending["status"] != "pending" {
		t.Fatalf("pending result = %+v", result)
	}

	startAuthTestServer(t, map[string]string{
		"PollAuthSessionStatus": `{"response":{}}`,
	})
	if _, errObj := handleQRPoll(json.RawMessage(`{"client_id":"c1","request_id":"r1"}`)); errObj == nil || errObj.Code != "AUTH_EXPIRED" {
		t.Fatalf("expired poll error = %+v, want AUTH_EXPIRED", errObj)
	}
}
