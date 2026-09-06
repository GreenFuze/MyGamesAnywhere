package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildSteamAchievementEntriesKeepsMixedStates(t *testing.T) {
	schema := []schemaAchievement{
		{Name: "ach-1", DisplayName: "Unlocked One", Description: "First", Icon: "u1", IconGray: "l1"},
		{Name: "ach-2", DisplayName: "Locked Two", Description: "Second", Icon: "u2", IconGray: "l2"},
	}
	playerMap := map[string]playerAchievement{
		"ach-1": {APIName: "ach-1", Achieved: 1, UnlockTime: 1710000000},
		"ach-2": {APIName: "ach-2", Achieved: 0, UnlockTime: 1710001111},
	}
	globalRarity := map[string]float64{
		"ach-1": 12.5,
		"ach-2": 48.0,
	}

	entries, unlocked := buildSteamAchievementEntries(schema, playerMap, globalRarity)

	if unlocked != 1 {
		t.Fatalf("unlocked = %d, want 1", unlocked)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if !entries[0].Unlocked || entries[0].UnlockedAt != 1710000000 {
		t.Fatalf("entry[0] = %+v, want unlocked with unix timestamp", entries[0])
	}
	if entries[1].Unlocked {
		t.Fatalf("entry[1] should remain locked: %+v", entries[1])
	}
	if entries[1].UnlockedAt != 0 {
		t.Fatalf("locked entry should not keep unlock time, got %d", entries[1].UnlockedAt)
	}
}

func TestAchievementsUseTheAPIKeyEvenWhenSignedIn(t *testing.T) {
	// Steam settled this on 2026-09-06. Asking GetSchemaForGame with an access
	// token returns "400 Bad Request: Required parameter 'key' is missing" for
	// every game — measured against a real library, where preferring the token
	// failed all 35 of them. The library is read as the account; achievements
	// are not, and this test exists so that stops being rediscovered.
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record := func() {
			query := r.URL.Query()
			switch {
			case query.Get("access_token") != "":
				seen = append(seen, r.URL.Path+" access_token")
			case query.Get("key") != "":
				seen = append(seen, r.URL.Path+" key")
			default:
				seen = append(seen, r.URL.Path+" none")
			}
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/IAuthenticationService/GenerateAccessTokenForApp/"):
			fmt.Fprint(w, `{"response":{"access_token":"minted-access-token"}}`)
		case strings.HasPrefix(r.URL.Path, "/ISteamUserStats/GetSchemaForGame/"):
			record()
			fmt.Fprint(w, `{"game":{"availableGameStats":{"achievements":[{"name":"A","displayName":"First","description":"d"}]}}}`)
		case strings.HasPrefix(r.URL.Path, "/ISteamUserStats/GetPlayerAchievements/"):
			record()
			fmt.Fprint(w, `{"playerstats":{"success":true,"achievements":[{"apiname":"A","achieved":1,"unlocktime":1700000000}]}}`)
		case strings.HasPrefix(r.URL.Path, "/ISteamUserStats/GetGlobalAchievementPercentagesForApp/"):
			fmt.Fprint(w, `{"achievementpercentages":{"achievements":[]}}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	originalAPI, originalAuth, originalSchema := steamAPIBase, steamAuthAPIBase, fetchAchievementSchemaBaseURL
	steamAPIBase, steamAuthAPIBase, fetchAchievementSchemaBaseURL = server.URL, server.URL, server.URL
	defer func() {
		steamAPIBase, steamAuthAPIBase, fetchAchievementSchemaBaseURL = originalAPI, originalAuth, originalSchema
	}()

	const steamID = "76561198012345678"
	params := fmt.Sprintf(
		`{"external_game_id":"440","config":{"api_key":"publisher-key","steam_id":%q,"refresh_token":%q}}`,
		steamID, testSteamRefreshToken(t, steamID),
	)
	if _, errObj := handleAchievementsGet(json.RawMessage(params)); errObj != nil {
		t.Fatalf("achievements failed: %+v", errObj)
	}

	if len(seen) < 2 {
		t.Fatalf("expected the schema and the player calls, saw %v", seen)
	}
	for _, call := range seen {
		if !strings.HasSuffix(call, " key") {
			t.Errorf("%s did not use the API key; Steam rejects anything else here", call)
		}
	}
}

func TestAchievementsSayTheyNeedAKeyRatherThanFailing(t *testing.T) {
	// A connection that is only a QR sign-in reads its whole library. Saying
	// that achievements are unavailable is the truth; failing 35 times with a
	// Steam parse error is not.
	steamID := "76561198012345678"
	params := fmt.Sprintf(
		`{"external_game_id":"440","config":{"steam_id":%q,"refresh_token":%q}}`,
		steamID, testSteamRefreshToken(t, steamID),
	)
	_, errObj := handleAchievementsGet(json.RawMessage(params))
	if errObj == nil {
		t.Fatal("a connection with no API key reported achievements as available")
	}
	if errObj.Code != "NOT_CONFIGURED" {
		t.Errorf("code = %q, want NOT_CONFIGURED", errObj.Code)
	}
	if !strings.Contains(errObj.Message, "API key") {
		t.Errorf("message does not say what is missing: %q", errObj.Message)
	}
}

func TestAchievementsStillWorkWithOnlyAnAPIKey(t *testing.T) {
	// The key remains the fallback for a connection that has not signed in.
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/ISteamUserStats/GetSchemaForGame/"):
			seen = append(seen, r.URL.Query().Encode())
			fmt.Fprint(w, `{"game":{"availableGameStats":{"achievements":[{"name":"A","displayName":"First"}]}}}`)
		case strings.HasPrefix(r.URL.Path, "/ISteamUserStats/GetPlayerAchievements/"):
			fmt.Fprint(w, `{"playerstats":{"success":true,"achievements":[]}}`)
		case strings.HasPrefix(r.URL.Path, "/ISteamUserStats/GetGlobalAchievementPercentagesForApp/"):
			fmt.Fprint(w, `{"achievementpercentages":{"achievements":[]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	originalAPI, originalSchema := steamAPIBase, fetchAchievementSchemaBaseURL
	steamAPIBase, fetchAchievementSchemaBaseURL = server.URL, server.URL
	defer func() { steamAPIBase, fetchAchievementSchemaBaseURL = originalAPI, originalSchema }()

	params := `{"external_game_id":"440","config":{"api_key":"publisher-key","steam_id":"76561198012345678"}}`
	if _, errObj := handleAchievementsGet(json.RawMessage(params)); errObj != nil {
		t.Fatalf("achievements with only a key failed: %+v", errObj)
	}
	if len(seen) == 0 || !strings.Contains(seen[0], "key=publisher-key") {
		t.Errorf("the schema call did not use the key: %v", seen)
	}
}
