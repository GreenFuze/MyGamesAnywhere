package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

func TestDuplicateGamesLooseGroupsAcrossCanonicalGames(t *testing.T) {
	controller := NewGameController(
		&fakeGameStore{duplicateRecords: []core.DuplicateGameSourceRecord{
			duplicateTestRecord("canon-a", "Desert Strike", duplicateTestSource("scan:a", "drive-1", "game-source-google-drive", "Desert Strike (USA)", core.PlatformGenesis, "Games/Genesis")),
			duplicateTestRecord("canon-b", "Desert Strike", duplicateTestSource("scan:b", "steam-1", "game-source-steam", "Desert Strike [Europe]", core.PlatformSNES, "")),
		}, gamesByID: map[string]*core.CanonicalGame{
			"canon-a": duplicateTestCanonicalGame("canon-a", "Desert Strike", core.PlatformGenesis, 101),
			"canon-b": duplicateTestCanonicalGame("canon-b", "Desert Strike", core.PlatformSNES, 202),
		}},
		nil,
		nil,
		&fakeIntegrationRepo{items: []*core.Integration{
			{ID: "drive-1", Label: "Drive", PluginID: "game-source-google-drive"},
			{ID: "steam-1", Label: "Steam", PluginID: "game-source-steam"},
		}},
		&fakeCacheService{entries: []*core.SourceCacheEntry{{SourceGameID: "scan:a", Status: "ready"}}},
		noopLogger{},
	)
	router := chi.NewRouter()
	router.Get("/api/duplicates/games", controller.DuplicateGames)

	req := httptest.NewRequest(http.MethodGet, "/api/duplicates/games?mode=loose", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp DuplicateGamesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Mode != duplicateModeLoose {
		t.Fatalf("mode = %q, want %q", resp.Mode, duplicateModeLoose)
	}
	if len(resp.Groups) != 1 {
		t.Fatalf("groups = %d, want 1: %+v", len(resp.Groups), resp.Groups)
	}
	group := resp.Groups[0]
	if len(group.CanonicalIDs) != 2 || len(group.Sources) != 2 {
		t.Fatalf("group = %+v, want two canonical/source records", group)
	}
	if !group.Sources[0].Cached {
		t.Fatalf("first source cached = false, want true: %+v", group.Sources[0])
	}
	if group.Sources[0].Source.HardDelete == nil || !group.Sources[0].Source.HardDelete.Eligible {
		t.Fatalf("drive source hard delete = %+v, want eligible", group.Sources[0].Source.HardDelete)
	}
	if group.Sources[1].Source.HardDelete == nil || group.Sources[1].Source.HardDelete.Eligible {
		t.Fatalf("steam source hard delete = %+v, want disabled", group.Sources[1].Source.HardDelete)
	}
	if group.Sources[0].Game == nil || group.Sources[0].Game.ID != "canon-a" || group.Sources[0].Game.CoverOverride == nil || group.Sources[0].Game.CoverOverride.AssetID != 101 {
		t.Fatalf("first source game display = %+v, want canon-a with cover override 101", group.Sources[0].Game)
	}
	if group.Sources[1].Game == nil || group.Sources[1].Game.ID != "canon-b" || group.Sources[1].Game.CoverOverride == nil || group.Sources[1].Game.CoverOverride.AssetID != 202 {
		t.Fatalf("second source game display = %+v, want canon-b with cover override 202", group.Sources[1].Game)
	}
}

func TestDuplicateGamesStrictSeparatesPlatformVariants(t *testing.T) {
	controller := NewGameController(
		&fakeGameStore{duplicateRecords: []core.DuplicateGameSourceRecord{
			duplicateTestRecord("canon-a", "Doom", duplicateTestSource("scan:a", "drive-1", "game-source-google-drive", "Doom (USA)", core.PlatformGenesis, "Games/Genesis")),
			duplicateTestRecord("canon-a", "Doom", duplicateTestSource("scan:b", "drive-1", "game-source-google-drive", "Doom (Europe)", core.PlatformGenesis, "Games/Genesis")),
			duplicateTestRecord("canon-a", "Doom", duplicateTestSource("scan:c", "drive-1", "game-source-google-drive", "Doom (SNES)", core.PlatformSNES, "Games/SNES")),
		}, gamesByID: map[string]*core.CanonicalGame{
			"canon-a": duplicateTestCanonicalGame("canon-a", "Doom", core.PlatformGenesis, 0),
		}},
		nil,
		nil,
		&fakeIntegrationRepo{items: []*core.Integration{{ID: "drive-1", Label: "Drive", PluginID: "game-source-google-drive"}}},
		nil,
		noopLogger{},
	)
	router := chi.NewRouter()
	router.Get("/api/duplicates/games", controller.DuplicateGames)

	req := httptest.NewRequest(http.MethodGet, "/api/duplicates/games?mode=strict", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp DuplicateGamesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Groups) != 1 {
		t.Fatalf("groups = %d, want only same-platform duplicate group: %+v", len(resp.Groups), resp.Groups)
	}
	if len(resp.Groups[0].Sources) != 2 {
		t.Fatalf("strict sources = %d, want 2", len(resp.Groups[0].Sources))
	}
	for _, source := range resp.Groups[0].Sources {
		if source.Source.Platform != string(core.PlatformGenesis) {
			t.Fatalf("strict source platform = %q, want genesis", source.Source.Platform)
		}
		if source.Game == nil || source.Game.ID != "canon-a" {
			t.Fatalf("strict source game display = %+v, want canon-a", source.Game)
		}
		if source.Game.CoverOverride != nil || len(source.Game.Media) != 0 {
			t.Fatalf("strict source game display media = %+v/%+v, want no media", source.Game.CoverOverride, source.Game.Media)
		}
	}
}

func TestDuplicateGamesLoadsDisplayOnlyForDuplicateGroups(t *testing.T) {
	store := &fakeGameStore{duplicateRecords: []core.DuplicateGameSourceRecord{
		duplicateTestRecord("canon-a", "Frogger", duplicateTestSource("scan:a", "drive-1", "game-source-google-drive", "Frogger (USA)", core.PlatformSNES, "Games/SNES")),
		duplicateTestRecord("canon-b", "Frogger", duplicateTestSource("scan:b", "drive-1", "game-source-google-drive", "Frogger [Europe]", core.PlatformSNES, "Games/SNES")),
		duplicateTestRecord("canon-unique", "Chrono Trigger", duplicateTestSource("scan:c", "drive-1", "game-source-google-drive", "Chrono Trigger", core.PlatformSNES, "Games/SNES")),
	}, gamesByID: map[string]*core.CanonicalGame{
		"canon-a":      duplicateTestCanonicalGame("canon-a", "Frogger", core.PlatformSNES, 101),
		"canon-b":      duplicateTestCanonicalGame("canon-b", "Frogger", core.PlatformSNES, 202),
		"canon-unique": duplicateTestCanonicalGame("canon-unique", "Chrono Trigger", core.PlatformSNES, 303),
	}}
	controller := NewGameController(
		store,
		nil,
		nil,
		&fakeIntegrationRepo{items: []*core.Integration{{ID: "drive-1", Label: "Drive", PluginID: "game-source-google-drive"}}},
		nil,
		noopLogger{},
	)
	router := chi.NewRouter()
	router.Get("/api/duplicates/games", controller.DuplicateGames)

	req := httptest.NewRequest(http.MethodGet, "/api/duplicates/games?mode=loose", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := store.requestedCanonicalIDs; len(got) != 2 || got[0] != "canon-a" || got[1] != "canon-b" {
		t.Fatalf("display canonical ids = %+v, want only duplicate canonical IDs canon-a/canon-b", got)
	}
}

func TestDuplicateGamesSkipsDisplayLoadWhenOnlyUniqueRowsExist(t *testing.T) {
	store := &fakeGameStore{duplicateRecords: []core.DuplicateGameSourceRecord{
		duplicateTestRecord("canon-a", "Frogger", duplicateTestSource("scan:a", "drive-1", "game-source-google-drive", "Frogger", core.PlatformSNES, "Games/SNES")),
		duplicateTestRecord("canon-b", "Chrono Trigger", duplicateTestSource("scan:b", "drive-1", "game-source-google-drive", "Chrono Trigger", core.PlatformSNES, "Games/SNES")),
	}}
	controller := NewGameController(
		store,
		nil,
		nil,
		&fakeIntegrationRepo{items: []*core.Integration{{ID: "drive-1", Label: "Drive", PluginID: "game-source-google-drive"}}},
		nil,
		noopLogger{},
	)
	router := chi.NewRouter()
	router.Get("/api/duplicates/games", controller.DuplicateGames)

	req := httptest.NewRequest(http.MethodGet, "/api/duplicates/games?mode=loose", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(store.requestedCanonicalIDs) != 0 {
		t.Fatalf("display canonical ids = %+v, want no display load for unique rows", store.requestedCanonicalIDs)
	}
}

func TestDuplicateGamesRejectsInvalidMode(t *testing.T) {
	controller := NewGameController(&fakeGameStore{}, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.Get("/api/duplicates/games", controller.DuplicateGames)

	req := httptest.NewRequest(http.MethodGet, "/api/duplicates/games?mode=wide", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDuplicateGamesEmptyLibraryReturnsNoGroups(t *testing.T) {
	controller := NewGameController(&fakeGameStore{}, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.Get("/api/duplicates/games", controller.DuplicateGames)

	req := httptest.NewRequest(http.MethodGet, "/api/duplicates/games", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp DuplicateGamesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Groups) != 0 {
		t.Fatalf("groups = %d, want 0", len(resp.Groups))
	}
}

func duplicateTestRecord(canonicalID string, canonicalTitle string, source *core.SourceGame) core.DuplicateGameSourceRecord {
	var total int64
	for _, file := range source.Files {
		total += file.Size
	}
	return core.DuplicateGameSourceRecord{
		CanonicalGameID: canonicalID,
		CanonicalTitle:  canonicalTitle,
		SourceGame:      source,
		FileCount:       len(source.Files),
		TotalSize:       total,
	}
}

func duplicateTestSource(id string, integrationID string, pluginID string, rawTitle string, platform core.Platform, rootPath string) *core.SourceGame {
	now := time.Unix(1710000000, 0).UTC()
	return &core.SourceGame{
		ID:            id,
		IntegrationID: integrationID,
		PluginID:      pluginID,
		ExternalID:    id + "-external",
		RawTitle:      rawTitle,
		Platform:      platform,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		RootPath:      rootPath,
		Status:        "found",
		CreatedAt:     now,
		Files: []core.GameFile{{
			Path:     rawTitle + ".zip",
			Role:     core.GameFileRoleRoot,
			FileKind: "archive",
			Size:     1024,
		}},
	}
}

func duplicateTestCanonicalGame(id string, title string, platform core.Platform, coverAssetID int) *core.CanonicalGame {
	game := &core.CanonicalGame{
		ID:       id,
		Title:    title,
		Platform: platform,
		Kind:     core.GameKindBaseGame,
	}
	if coverAssetID > 0 {
		game.Media = []core.MediaRef{{
			AssetID: coverAssetID + 1000,
			Type:    core.MediaTypeCover,
			URL:     "https://example.com/" + id + "-cover.jpg",
			Source:  "metadata-test",
		}}
		game.CoverOverride = &core.MediaRef{
			AssetID: coverAssetID,
			Type:    core.MediaTypeCover,
			URL:     "https://example.com/" + id + "-override.jpg",
			Source:  "metadata-test",
		}
	}
	return game
}
