package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestHandleGamesListRequiresConfiguration(t *testing.T) {
	// A connection with nothing at all cannot scan. Which of the two things is
	// missing is reported as the sign-in, because that is what a player is
	// asked for first.
	if _, errObj := handleGamesList(nil); errObj == nil || errObj.Code != "AUTH_REQUIRED" {
		t.Fatalf("empty config: got %+v, want AUTH_REQUIRED", errObj)
	}

	if _, errObj := handleGamesList(json.RawMessage(`{"api_key":"key-only"}`)); errObj == nil || errObj.Code != "AUTH_REQUIRED" {
		t.Fatalf("profile key without Steam identity: got %+v, want AUTH_REQUIRED", errObj)
	}

	// A Steam identity with neither credential is refused for the credential,
	// and the message names both ways out rather than only the key.
	_, errObj := handleGamesList(json.RawMessage(`{"steam_id":"76561198012345678"}`))
	if errObj == nil || errObj.Code != "AUTH_REQUIRED" {
		t.Fatalf("identity without a credential: got %+v, want AUTH_REQUIRED", errObj)
	}
	if !strings.Contains(errObj.Message, "Steam app") || !strings.Contains(errObj.Message, "API key") {
		t.Errorf("message names only one way to fix it: %q", errObj.Message)
	}
}

func TestOwnedGamesPrefersTheSignedInTokenOverTheAPIKey(t *testing.T) {
	// The two credentials are not equivalent: a publisher API key is a third
	// party asking about an account, and Steam answers it subject to that
	// account's privacy settings. The token minted from a QR sign-in is the
	// account's own credential. Wherever a player has signed in, that is what
	// the library call must carry.
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/IAuthenticationService/GenerateAccessTokenForApp/"):
			fmt.Fprint(w, `{"response":{"access_token":"minted-access-token"}}`)
		case strings.HasPrefix(r.URL.Path, "/IPlayerService/GetOwnedGames/"):
			query := r.URL.Query()
			switch {
			case query.Get("access_token") != "":
				seen = append(seen, "access_token="+query.Get("access_token"))
			case query.Get("key") != "":
				seen = append(seen, "key="+query.Get("key"))
			default:
				seen = append(seen, "none")
			}
			fmt.Fprint(w, `{"response":{"game_count":0,"games":[]}}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	originalAPI, originalAuth := steamAPIBase, steamAuthAPIBase
	steamAPIBase, steamAuthAPIBase = server.URL, server.URL
	defer func() { steamAPIBase, steamAuthAPIBase = originalAPI, originalAuth }()

	const steamID = "76561198012345678"
	signedIn := testSteamRefreshToken(t, steamID)
	both := steamConfig{APIKey: "publisher-key", RefreshToken: signedIn, SteamID: steamID}
	cred, err := credentialFor(both)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Param != "access_token" {
		t.Fatalf("with both credentials the call used %q, want the sign-in", cred.Param)
	}
	if _, err := fetchOwnedGames(cred, both.SteamID); err != nil {
		t.Fatal(err)
	}

	// Without a sign-in the key is still used, so an existing connection keeps
	// working.
	keyOnly := steamConfig{APIKey: "publisher-key", SteamID: steamID}
	cred, err = credentialFor(keyOnly)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Param != "key" {
		t.Fatalf("with only a key the call used %q, want the key", cred.Param)
	}
	if _, err := fetchOwnedGames(cred, keyOnly.SteamID); err != nil {
		t.Fatal(err)
	}

	// And a sign-in on its own needs no key at all.
	tokenOnly := steamConfig{RefreshToken: signedIn, SteamID: steamID}
	cred, err = credentialFor(tokenOnly)
	if err != nil {
		t.Fatalf("a signed-in connection was refused for having no API key: %v", err)
	}
	if cred.Param != "access_token" {
		t.Fatalf("signed-in connection used %q", cred.Param)
	}

	want := []string{"access_token=minted-access-token", "key=publisher-key"}
	if len(seen) != len(want) {
		t.Fatalf("owned games was called %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("call %d carried %q, want %q", i, seen[i], want[i])
		}
	}
}

func TestARejectedSignInFallsBackToTheKeyRatherThanFailingTheScan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Steam returns an empty response for a refresh token it will not renew.
		fmt.Fprint(w, `{"response":{}}`)
	}))
	defer server.Close()
	originalAuth := steamAuthAPIBase
	steamAuthAPIBase = server.URL
	defer func() { steamAuthAPIBase = originalAuth }()

	stale := testSteamRefreshToken(t, "76561198012345678")
	withKey := steamConfig{APIKey: "publisher-key", RefreshToken: stale, SteamID: "76561198012345678"}
	cred, err := credentialFor(withKey)
	if err != nil {
		t.Fatalf("a stale sign-in with a key available should not fail: %v", err)
	}
	if cred.Param != "key" {
		t.Errorf("fell back to %q, want the key", cred.Param)
	}

	// With nothing to fall back to, it fails rather than pretending.
	if _, err := credentialFor(steamConfig{RefreshToken: stale, SteamID: "76561198012345678"}); err == nil {
		t.Error("a stale sign-in with no key was accepted")
	}
}

func TestHandleGamesListUsesProfileOwnedConfig(t *testing.T) {
	withTempWorkingDir(t)

	params := json.RawMessage(`{"api_key":"profile-key"}`)
	if _, errObj := handleGamesList(params); errObj == nil || errObj.Code != "AUTH_REQUIRED" {
		t.Fatalf("profile config api_key was not used: got %+v, want AUTH_REQUIRED", errObj)
	}
}

func TestHandleAchievementsGetUsesNestedProfileOwnedConfig(t *testing.T) {
	withTempWorkingDir(t)

	params := json.RawMessage(`{"external_game_id":"not-numeric","config":{"api_key":"profile-key","steam_id":"76561198012345678"}}`)
	if _, errObj := handleAchievementsGet(params); errObj == nil || errObj.Code != "INVALID_PARAMS" {
		t.Fatalf("nested profile config was not used before validation: got %+v, want INVALID_PARAMS", errObj)
	}
}

func TestHandleAchievementsGetTreatsSteamSchema400EmptyObjectAsNoAchievements(t *testing.T) {
	withTempWorkingDir(t)

	originalFetchAchievementSchemaBaseURL := fetchAchievementSchemaBaseURL
	t.Cleanup(func() {
		fetchAchievementSchemaBaseURL = originalFetchAchievementSchemaBaseURL
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISteamUserStats/GetSchemaForGame/v2/" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("appid"); got != "7807" {
			t.Fatalf("appid = %q, want 7807", got)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	fetchAchievementSchemaBaseURL = server.URL
	params := json.RawMessage(`{"external_game_id":"7807","config":{"api_key":"profile-key","steam_id":"76561198012345678"}}`)
	result, errObj := handleAchievementsGet(params)
	if errObj != nil {
		t.Fatalf("handleAchievementsGet() error = %+v, want no-achievements result", errObj)
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", result)
	}
	if resultMap["total_count"] != 0 || resultMap["unlocked_count"] != 0 {
		t.Fatalf("counts = total %#v unlocked %#v, want 0/0", resultMap["total_count"], resultMap["unlocked_count"])
	}
	achievements, ok := resultMap["achievements"].([]any)
	if !ok || len(achievements) != 0 {
		t.Fatalf("achievements = %#v, want empty []any", resultMap["achievements"])
	}
}

func TestLoadConfigDoesNotReadLegacyTokenFile(t *testing.T) {
	withTempWorkingDir(t)

	if err := os.WriteFile(configFile, []byte(`{"api_key":"key","steam_id":"profile-steam-id"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("tokens.json", []byte(`{"steam_id":"legacy-token-id"}`), 0600); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SteamID != "profile-steam-id" {
		t.Fatalf("steam id = %q, want only config.json identity", loaded.SteamID)
	}
}

func TestHandleOAuthCallbackReturnsConfigUpdate(t *testing.T) {
	withTempWorkingDir(t)

	originalPending := oauthPending
	t.Cleanup(func() {
		oauthPending = originalPending
	})

	oauthPending = map[string]bool{"state-1": true}
	payload := map[string]any{
		"state": "state-1",
		"params": map[string]string{
			"openid.claimed_id": "https://steamcommunity.com/openid/id/76561198012345678",
		},
	}
	params, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	result, errObj := handleOAuthCallback(params)
	if errObj != nil {
		t.Fatalf("callback error: %+v", errObj)
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", result)
	}
	updates, ok := resultMap["config_updates"].(map[string]any)
	if !ok {
		t.Fatalf("config_updates = %#v, want map", resultMap["config_updates"])
	}
	if updates["steam_id"] != "76561198012345678" {
		t.Fatalf("steam_id update = %#v", updates["steam_id"])
	}
}

func withTempWorkingDir(t *testing.T) {
	t.Helper()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}

// ipcCall sends a single IPC request and reads the response via stdin/stdout
// of the given process.
func ipcCall(stdin io.Writer, stdout io.Reader, method string, params any) (*Response, error) {
	req := Request{
		ID:     fmt.Sprintf("test-%d", time.Now().UnixNano()),
		Method: method,
	}
	if params != nil {
		b, _ := json.Marshal(params)
		req.Params = b
	}
	payload, _ := json.Marshal(req)

	if err := binary.Write(stdin, binary.BigEndian, uint32(len(payload))); err != nil {
		return nil, fmt.Errorf("write length: %w", err)
	}
	if _, err := stdin.Write(payload); err != nil {
		return nil, fmt.Errorf("write payload: %w", err)
	}

	var respLen uint32
	if err := binary.Read(stdout, binary.BigEndian, &respLen); err != nil {
		return nil, fmt.Errorf("read resp length: %w", err)
	}
	respData := make([]byte, respLen)
	if _, err := io.ReadFull(stdout, respData); err != nil {
		return nil, fmt.Errorf("read resp payload: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp, nil
}

func TestSteamSourcePlugin(t *testing.T) {
	if os.Getenv("STEAM_SOURCE_INTEGRATION") != "1" {
		t.Skip("set STEAM_SOURCE_INTEGRATION=1 to run")
	}

	exePath := "./steam-source.exe"
	if _, err := os.Stat(exePath); err != nil {
		t.Fatalf("build the plugin first: go build -o steam-source.exe .")
	}

	cmd := exec.Command(exePath)
	cmd.Dir, _ = os.Getwd()
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stdin.Close()
		cmd.Wait()
	}()

	// 1. plugin.init
	t.Log("calling plugin.init...")
	resp, err := ipcCall(stdin, stdout, "plugin.init", nil)
	if err != nil {
		t.Fatalf("plugin.init: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("plugin.init error: %s: %s", resp.Error.Code, resp.Error.Message)
	}
	t.Logf("init result: %v", resp.Result)

	// 2. source.games.list
	t.Log("calling source.games.list...")
	resp, err = ipcCall(stdin, stdout, "source.games.list", nil)
	if err != nil {
		t.Fatalf("source.games.list: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("source.games.list error: %s: %s", resp.Error.Code, resp.Error.Message)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result struct {
		Games []gameEntry `json:"games"`
	}
	json.Unmarshal(resultBytes, &result)

	t.Logf("=== Steam Game Source Results ===")
	t.Logf("Total games returned: %d", len(result.Games))

	withDesc := 0
	withRelease := 0
	withGenres := 0
	withDev := 0
	withMedia := 0

	for _, g := range result.Games {
		if g.Description != "" {
			withDesc++
		}
		if g.ReleaseDate != "" {
			withRelease++
		}
		if len(g.Genres) > 0 {
			withGenres++
		}
		if g.Developer != "" {
			withDev++
		}
		if len(g.Media) > 0 {
			withMedia++
		}
	}

	t.Logf("\nEnrichment coverage:")
	t.Logf("  Description:  %d/%d (%.0f%%)", withDesc, len(result.Games), pct(withDesc, len(result.Games)))
	t.Logf("  ReleaseDate:  %d/%d (%.0f%%)", withRelease, len(result.Games), pct(withRelease, len(result.Games)))
	t.Logf("  Genres:       %d/%d (%.0f%%)", withGenres, len(result.Games), pct(withGenres, len(result.Games)))
	t.Logf("  Developer:    %d/%d (%.0f%%)", withDev, len(result.Games), pct(withDev, len(result.Games)))
	t.Logf("  Media:        %d/%d (%.0f%%)", withMedia, len(result.Games), pct(withMedia, len(result.Games)))

	t.Logf("\nSample games (first 10):")
	count := 10
	if len(result.Games) < count {
		count = len(result.Games)
	}
	for i := 0; i < count; i++ {
		g := result.Games[i]
		t.Logf("  [%d] %s (appid=%s)", i+1, g.Title, g.ExternalID)
		t.Logf("      Developer: %s | Publisher: %s", g.Developer, g.Publisher)
		t.Logf("      Genres: %v | Release: %s", g.Genres, g.ReleaseDate)
		t.Logf("      Playtime: %d min | Media items: %d",
			g.PlaytimeMinutes, len(g.Media))
	}

	if len(result.Games) == 0 {
		t.Error("expected at least some games from Steam library")
	}
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}
