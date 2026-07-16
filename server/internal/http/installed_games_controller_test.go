package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
	"github.com/go-chi/chi/v5"
)

type installedGamesDeviceLister struct {
	endpoints []devices.Endpoint
	profileID string
}

func (l *installedGamesDeviceLister) ListEndpoints(_ context.Context, profileID string) ([]devices.Endpoint, error) {
	l.profileID = profileID
	return l.endpoints, nil
}

func TestInstalledGamesShelfFiltersDeduplicatesAndSorts(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	store := &fakeGameStore{gamesByID: map[string]*core.CanonicalGame{
		"game-z": {ID: "game-z", Title: "  Same   Title ", Platform: core.PlatformWindowsPC, Kind: core.GameKindBaseGame},
		"game-a": {ID: "game-a", Title: "alpha", Platform: core.PlatformWindowsPC, Kind: core.GameKindBaseGame},
		"game-b": {ID: "game-b", Title: "Same Title", Platform: core.PlatformWindowsPC, Kind: core.GameKindBaseGame},
	}}
	controller := &GameController{gameStore: store, logger: noopLogger{}}
	response, err := controller.buildInstalledGamesResponse(context.Background(), devices.Endpoint{
		ID: "device-1", DisplayName: "Gaming PC", Status: devicev1.EndpointReady,
		AccessLevel: devicev1.AccessPlay, Capabilities: []string{devicev1.CapabilityGameLaunch},
		Installations: []devices.GameInstallation{
			{GameID: "game-a", SourceGameID: "source-new-no-target", InstallKind: devicev1.InstallKindManagedArchive, InstallState: devicev1.InstallStateInstalled, UpdatedAt: now.Add(time.Minute)},
			{GameID: "game-a", SourceGameID: "source-target", InstallKind: devicev1.InstallKindGogInno, InstallState: devicev1.InstallStateInstalled, LaunchTarget: "Alpha.exe", InstalledAt: now, UpdatedAt: now},
			{GameID: "game-b", SourceGameID: "source-z", InstallKind: devicev1.InstallKindGogInno, InstallState: devicev1.InstallStateInstalled, LaunchTarget: "B.exe", InstalledAt: now, UpdatedAt: now},
			{GameID: "game-b", SourceGameID: "source-a", InstallKind: devicev1.InstallKindGogInno, InstallState: devicev1.InstallStateInstalled, LaunchTarget: "B.exe", InstalledAt: now, UpdatedAt: now},
			{GameID: "game-z", SourceGameID: "source-z", InstallKind: devicev1.InstallKindManagedArchive, InstallState: devicev1.InstallStateInstalled, LaunchTarget: "Z.exe", InstalledAt: now, UpdatedAt: now},
			{GameID: "attention", SourceGameID: "failed-1", InstallState: devicev1.InstallStateAttentionRequired},
			{GameID: "attention", SourceGameID: "failed-2", InstallState: devicev1.InstallStateCleanupRequired},
			{GameID: "ignored", SourceGameID: "ignored", InstallState: devicev1.InstallStateIgnoredFailure},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.AttentionCount != 2 {
		t.Fatalf("attention_count=%d, want 2 canonical games", response.AttentionCount)
	}
	if len(response.Games) != 3 {
		t.Fatalf("games=%#v", response.Games)
	}
	if response.Games[0].Game.ID != "game-a" || response.Games[0].SourceGameID != "source-target" || response.Games[0].InstallKind != devicev1.InstallKindGogInno {
		t.Fatalf("target-bearing copy was not selected: %#v", response.Games[0])
	}
	if response.Games[1].Game.ID != "game-b" || response.Games[1].SourceGameID != "source-a" {
		t.Fatalf("lexical source tie-break failed: %#v", response.Games[1])
	}
	if response.Games[2].Game.ID != "game-z" {
		t.Fatalf("stable title/id sort failed: %#v", response.Games)
	}
	for _, game := range response.Games {
		if !game.LaunchSupported || !game.CanPlay || game.InstallState != devicev1.InstallStateInstalled {
			t.Fatalf("playability=%#v", game)
		}
	}
}

func TestInstalledGamesShelfCanPlayRequiresExactEndpointFacts(t *testing.T) {
	t.Parallel()
	base := devices.Endpoint{
		ID: "device-1", DisplayName: "PC", Status: devicev1.EndpointReady, AccessLevel: devicev1.AccessPlay,
		Capabilities:  []string{devicev1.CapabilityGameLaunch},
		Installations: []devices.GameInstallation{{GameID: "game-1", SourceGameID: "source-1", InstallKind: devicev1.InstallKindManagedArchive, InstallState: devicev1.InstallStateInstalled, LaunchTarget: "Game.exe"}},
	}
	for _, test := range []struct {
		name     string
		mutate   func(*devices.Endpoint)
		wantPlay bool
	}{
		{name: "ready", mutate: func(*devices.Endpoint) {}, wantPlay: true},
		{name: "offline", mutate: func(endpoint *devices.Endpoint) { endpoint.Status = devicev1.EndpointOffline }},
		{name: "update required", mutate: func(endpoint *devices.Endpoint) { endpoint.Status = devicev1.EndpointUpdateRequired }},
		{name: "view only", mutate: func(endpoint *devices.Endpoint) { endpoint.AccessLevel = devicev1.AccessView }},
		{name: "missing capability", mutate: func(endpoint *devices.Endpoint) { endpoint.Capabilities = nil }},
		{name: "missing target", mutate: func(endpoint *devices.Endpoint) { endpoint.Installations[0].LaunchTarget = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			endpoint := base
			endpoint.Capabilities = append([]string(nil), base.Capabilities...)
			endpoint.Installations = append([]devices.GameInstallation(nil), base.Installations...)
			test.mutate(&endpoint)
			controller := &GameController{gameStore: &fakeGameStore{gamesByID: map[string]*core.CanonicalGame{
				"game-1": {ID: "game-1", Title: "Game", Platform: core.PlatformWindowsPC, Kind: core.GameKindBaseGame},
			}}, logger: noopLogger{}}
			response, err := controller.buildInstalledGamesResponse(context.Background(), endpoint)
			if err != nil || len(response.Games) != 1 {
				t.Fatalf("response=%#v error=%v", response, err)
			}
			if response.Games[0].CanPlay != test.wantPlay {
				t.Fatalf("can_play=%v, want %v", response.Games[0].CanPlay, test.wantPlay)
			}
		})
	}
}

func TestListInstalledGamesUsesActiveProfileAndHidesUnauthorizedDevice(t *testing.T) {
	t.Parallel()
	lister := &installedGamesDeviceLister{endpoints: []devices.Endpoint{{ID: "allowed", DisplayName: "PC", AccessLevel: devicev1.AccessView}}}
	controller := &GameController{gameStore: &fakeGameStore{gamesByID: map[string]*core.CanonicalGame{}}, deviceLister: lister, logger: noopLogger{}}
	router := chi.NewRouter()
	router.Get("/api/play/devices/{id}/installed-games", controller.ListInstalledGames)

	request := httptest.NewRequest(http.MethodGet, "/api/play/devices/allowed/installed-games", nil)
	request = request.WithContext(core.WithProfile(request.Context(), &core.Profile{ID: "profile-1"}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || lister.profileID != "profile-1" {
		t.Fatalf("status=%d profile=%q body=%q", recorder.Code, lister.profileID, recorder.Body.String())
	}
	var response InstalledGamesResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Games == nil {
		t.Fatalf("response=%#v error=%v", response, err)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/play/devices/not-authorized/installed-games", nil)
	request = request.WithContext(core.WithProfile(request.Context(), &core.Profile{ID: "profile-1"}))
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unauthorized device status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}
