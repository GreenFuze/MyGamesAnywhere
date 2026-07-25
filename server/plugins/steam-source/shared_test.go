package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// familyTestServer serves the three endpoints fetchSharedGames touches:
// GetFamilyGroupForUser, GetSharedLibraryApps, and store appdetails. Callers
// point steamFamilyAPIBase and storeAPIBase at it.
type familyTestConfig struct {
	familyStatus int
	familyBody   string
	sharedBody   string
	appTypes     map[string]string // appid -> store "type" (default "game")
	// renewBody overrides the access-token renewal response. Empty means a
	// valid token is minted from the refresh token.
	renewBody string
}

func startFamilyTestServer(t *testing.T, cfg familyTestConfig) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/IAuthenticationService/GenerateAccessTokenForApp/"):
			if cfg.renewBody != "" {
				fmt.Fprint(w, cfg.renewBody)
				return
			}
			fmt.Fprint(w, `{"response":{"access_token":"minted-access-token"}}`)
		case strings.HasPrefix(r.URL.Path, "/IFamilyGroupsService/GetFamilyGroupForUser/"):
			if cfg.familyStatus != 0 && cfg.familyStatus != http.StatusOK {
				w.WriteHeader(cfg.familyStatus)
				return
			}
			fmt.Fprint(w, cfg.familyBody)
		case strings.HasPrefix(r.URL.Path, "/IFamilyGroupsService/GetSharedLibraryApps/"):
			fmt.Fprint(w, cfg.sharedBody)
		case r.URL.Path == "/appdetails":
			appID := r.URL.Query().Get("appids")
			appType := "game"
			if cfg.appTypes != nil {
				if v, ok := cfg.appTypes[appID]; ok {
					appType = v
				}
			}
			fmt.Fprintf(w, `{"%s":{"success":true,"data":{"type":"%s","name":"App %s","steam_appid":%s,"short_description":"desc %s"}}}`,
				appID, appType, appID, appID, appID)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	originalFamily, originalStore, originalAuth := steamFamilyAPIBase, storeAPIBase, steamAuthAPIBase
	steamFamilyAPIBase = server.URL
	storeAPIBase = server.URL
	steamAuthAPIBase = server.URL
	t.Cleanup(func() {
		steamFamilyAPIBase = originalFamily
		storeAPIBase = originalStore
		steamAuthAPIBase = originalAuth
	})
	return server
}

func TestFetchSharedGamesMergesFiltersAndDedups(t *testing.T) {
	// app 10: shared from OWNER1 (kept). app 20: owned only by SELF (filtered
	// defensively). app 30: shared but already owned locally (deduped).
	startFamilyTestServer(t, familyTestConfig{
		familyBody: `{"response":{"family_groupid":"9001"}}`,
		sharedBody: `{"response":{"apps":[
			{"appid":10,"name":"Borrowed Game","owner_steamids":["OWNER1"]},
			{"appid":20,"name":"Self Game","owner_steamids":["SELF"]},
			{"appid":30,"name":"Already Owned","owner_steamids":["OWNER1"]}
		]}}`,
	})

	cfg := steamConfig{APIKey: "k", SteamID: "SELF", RefreshToken: "refresh-tok"}
	shared, err := fetchSharedGames(cfg, map[int]bool{30: true})
	if err != nil {
		t.Fatalf("fetchSharedGames error: %v", err)
	}
	if len(shared) != 1 {
		t.Fatalf("shared count = %d, want 1 (%+v)", len(shared), shared)
	}
	got := shared[0]
	if got.ExternalID != "10" || !got.Shared || got.SharedOwner != "OWNER1" {
		t.Fatalf("shared entry = %+v, want appid 10, Shared true, owner OWNER1", got)
	}
	if got.Description != "desc 10" {
		t.Fatalf("shared entry not enriched: description = %q", got.Description)
	}
}

func TestFetchSharedGamesDropsNonGameTypes(t *testing.T) {
	// A DLC shared app must be dropped by store-type filtering, like owned games.
	startFamilyTestServer(t, familyTestConfig{
		familyBody: `{"response":{"family_groupid":"9001"}}`,
		sharedBody: `{"response":{"apps":[{"appid":40,"name":"Some DLC","owner_steamids":["OWNER1"]}]}}`,
		appTypes:   map[string]string{"40": "dlc"},
	})

	cfg := steamConfig{APIKey: "k", SteamID: "SELF", RefreshToken: "refresh-tok"}
	shared, err := fetchSharedGames(cfg, map[int]bool{})
	if err != nil {
		t.Fatalf("fetchSharedGames error: %v", err)
	}
	if len(shared) != 0 {
		t.Fatalf("shared count = %d, want 0 (DLC filtered)", len(shared))
	}
}

func TestFetchSharedGamesTokenExpired(t *testing.T) {
	startFamilyTestServer(t, familyTestConfig{familyStatus: http.StatusUnauthorized})

	cfg := steamConfig{APIKey: "k", SteamID: "SELF", RefreshToken: "expired-refresh"}
	_, err := fetchSharedGames(cfg, map[int]bool{})
	if !errors.Is(err, errAccessTokenRejected) {
		t.Fatalf("error = %v, want errAccessTokenRejected", err)
	}
}

func TestFetchFamilyGroupIDNotMember(t *testing.T) {
	startFamilyTestServer(t, familyTestConfig{
		familyBody: `{"response":{"is_not_member_of_any_group":true}}`,
	})

	_, err := fetchFamilyGroupID("tok", "SELF")
	if !errors.Is(err, errNoFamilyGroup) {
		t.Fatalf("error = %v, want errNoFamilyGroup", err)
	}
}

func TestIsSharedFromOtherAndOwnerLabel(t *testing.T) {
	self := "SELF"
	cases := []struct {
		name      string
		app       sharedApp
		wantShare bool
		wantOwner string
	}{
		{"lender", sharedApp{OwnerSteamIDs: []string{"OWNER1"}}, true, "OWNER1"},
		{"self only", sharedApp{OwnerSteamIDs: []string{self}}, false, ""},
		{"self and lender", sharedApp{OwnerSteamIDs: []string{self, "OWNER2"}}, true, "OWNER2"},
		{"empty", sharedApp{OwnerSteamIDs: nil}, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSharedFromOther(tc.app, self); got != tc.wantShare {
				t.Fatalf("isSharedFromOther = %v, want %v", got, tc.wantShare)
			}
			if got := sharedOwnerLabel(tc.app, self); got != tc.wantOwner {
				t.Fatalf("sharedOwnerLabel = %q, want %q", got, tc.wantOwner)
			}
		})
	}
}

func TestConfigFromMapReadsRefreshToken(t *testing.T) {
	cfg := configFromMap(map[string]any{"api_key": "k", "steam_id": "s", "refresh_token": " tok "})
	if cfg.RefreshToken != "tok" {
		t.Fatalf("refresh_token = %q, want trimmed 'tok'", cfg.RefreshToken)
	}
	// The retired manual access_token field must no longer be honoured.
	legacy := configFromMap(map[string]any{"api_key": "k", "access_token": "manual"})
	if legacy.RefreshToken != "" {
		t.Fatalf("legacy access_token was accepted as %q, want ignored", legacy.RefreshToken)
	}
}

func TestFetchSharedGamesRejectedRefreshTokenDegrades(t *testing.T) {
	// Steam returns no access token for a dead refresh token.
	startFamilyTestServer(t, familyTestConfig{renewBody: `{"response":{}}`})

	cfg := steamConfig{APIKey: "k", SteamID: "SELF", RefreshToken: "dead-refresh"}
	_, err := fetchSharedGames(cfg, map[int]bool{})
	if !errors.Is(err, errAccessTokenRejected) {
		t.Fatalf("error = %v, want errAccessTokenRejected", err)
	}
}

func TestFetchSharedGamesUsesMintedAccessTokenNotRefreshToken(t *testing.T) {
	var familyTokens []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/IAuthenticationService/GenerateAccessTokenForApp/"):
			fmt.Fprint(w, `{"response":{"access_token":"minted-access-token"}}`)
		case strings.HasPrefix(r.URL.Path, "/IFamilyGroupsService/"):
			familyTokens = append(familyTokens, r.URL.Query().Get("access_token"))
			if strings.Contains(r.URL.Path, "GetFamilyGroupForUser") {
				fmt.Fprint(w, `{"response":{"family_groupid":"9001"}}`)
				return
			}
			fmt.Fprint(w, `{"response":{"apps":[]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	originalFamily, originalAuth := steamFamilyAPIBase, steamAuthAPIBase
	steamFamilyAPIBase, steamAuthAPIBase = server.URL, server.URL
	t.Cleanup(func() { steamFamilyAPIBase, steamAuthAPIBase = originalFamily, originalAuth })

	cfg := steamConfig{APIKey: "k", SteamID: "SELF", RefreshToken: "secret-refresh"}
	if _, err := fetchSharedGames(cfg, map[int]bool{}); err != nil {
		t.Fatalf("fetchSharedGames error: %v", err)
	}
	if len(familyTokens) == 0 {
		t.Fatal("no family API calls were made")
	}
	for _, token := range familyTokens {
		if token != "minted-access-token" {
			t.Fatalf("family call used token %q, want the minted access token", token)
		}
		if token == "secret-refresh" {
			t.Fatal("refresh token leaked to the family API")
		}
	}
}
