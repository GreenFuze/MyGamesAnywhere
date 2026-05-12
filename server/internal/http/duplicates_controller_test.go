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
}

func TestDuplicateGamesStrictSeparatesPlatformVariants(t *testing.T) {
	controller := NewGameController(
		&fakeGameStore{duplicateRecords: []core.DuplicateGameSourceRecord{
			duplicateTestRecord("canon-a", "Doom", duplicateTestSource("scan:a", "drive-1", "game-source-google-drive", "Doom (USA)", core.PlatformGenesis, "Games/Genesis")),
			duplicateTestRecord("canon-a", "Doom", duplicateTestSource("scan:b", "drive-1", "game-source-google-drive", "Doom (Europe)", core.PlatformGenesis, "Games/Genesis")),
			duplicateTestRecord("canon-a", "Doom", duplicateTestSource("scan:c", "drive-1", "game-source-google-drive", "Doom (SNES)", core.PlatformSNES, "Games/SNES")),
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
