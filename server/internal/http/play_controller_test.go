package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/auth"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	dbpkg "github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
	"github.com/go-chi/chi/v5"
)

type fakePlayIntegrationRepo struct {
	byID map[string]*core.Integration
}

func (f *fakePlayIntegrationRepo) Create(context.Context, *core.Integration) error { return nil }
func (f *fakePlayIntegrationRepo) Update(context.Context, *core.Integration) error { return nil }
func (f *fakePlayIntegrationRepo) Delete(context.Context, string) error            { return nil }
func (f *fakePlayIntegrationRepo) List(context.Context) ([]*core.Integration, error) {
	return nil, nil
}
func (f *fakePlayIntegrationRepo) GetByID(_ context.Context, id string) (*core.Integration, error) {
	if f == nil || f.byID == nil {
		return nil, nil
	}
	return f.byID[id], nil
}
func (f *fakePlayIntegrationRepo) ListByPluginID(context.Context, string) ([]*core.Integration, error) {
	return nil, nil
}

type fakePlayProfileRepo struct {
	byID map[string]*core.Profile
}

func (f fakePlayProfileRepo) Create(context.Context, *core.Profile) error { return nil }
func (f fakePlayProfileRepo) Update(context.Context, *core.Profile) error { return nil }
func (f fakePlayProfileRepo) Delete(context.Context, string) error        { return nil }
func (f fakePlayProfileRepo) List(context.Context) ([]*core.Profile, error) {
	return nil, nil
}
func (f fakePlayProfileRepo) GetByID(_ context.Context, id string) (*core.Profile, error) {
	return f.byID[id], nil
}
func (f fakePlayProfileRepo) Count(context.Context) (int, error)       { return 0, nil }
func (f fakePlayProfileRepo) CountAdmins(context.Context) (int, error) { return 0, nil }
func (f fakePlayProfileRepo) EnsureDefaultForExistingData(context.Context) (*core.Profile, error) {
	return nil, nil
}

func TestCanonicalToGameDetailIncludesPlayMetadataAndFileIDs(t *testing.T) {
	root := t.TempDir()
	game := &core.CanonicalGame{
		ID:       "game-1",
		Title:    "Castlevania",
		Platform: core.PlatformPS1,
		Kind:     core.GameKindBaseGame,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-1",
				Platform:  core.PlatformPS1,
				GroupKind: core.GroupKindSelfContained,
				RootPath:  root,
				Status:    "found",
				CreatedAt: time.Unix(1700000000, 0),
				Files: []core.GameFile{
					{GameID: "source-1", Path: "Castlevania.cue", Role: core.GameFileRoleRoot, FileKind: "disc_meta", Size: 128},
					{GameID: "source-1", Path: "Castlevania (Track 1).bin", Role: core.GameFileRoleRequired, Size: 4096},
				},
			},
		},
	}

	detail := canonicalToGameDetail(game)
	if detail.Play == nil {
		t.Fatal("expected play metadata")
	}
	if !detail.Play.PlatformSupported {
		t.Fatal("expected platform_supported=true")
	}
	if !detail.Play.Available {
		t.Fatal("expected available=true")
	}
	if len(detail.Files) != 2 || detail.Files[0].ID == "" {
		t.Fatalf("expected file ids for merged files, got %+v", detail.Files)
	}
	if len(detail.SourceGames) != 1 || detail.SourceGames[0].Play == nil || !detail.SourceGames[0].Play.Launchable {
		t.Fatalf("expected launchable source game, got %+v", detail.SourceGames)
	}
	if len(detail.Play.LaunchSources) != 1 {
		t.Fatalf("expected 1 launch source, got %d", len(detail.Play.LaunchSources))
	}
	if len(detail.Play.LaunchCandidates) != 1 {
		t.Fatalf("expected 1 launch candidate, got %d", len(detail.Play.LaunchCandidates))
	}
	if len(detail.Play.Options) != 1 || detail.Play.Options[0].Kind != "browser" || detail.Play.Options[0].SourceGameID != "source-1" {
		t.Fatalf("expected browser launch option for source-1, got %+v", detail.Play.Options)
	}
	if detail.Play.Options[0].Save == nil || detail.Play.Options[0].Save.Access != "mga_managed" || !detail.Play.Options[0].Save.MGAWrite {
		t.Fatalf("browser save capability = %+v", detail.Play.Options[0].Save)
	}
	if detail.Play.LaunchCandidates[0].FileID != detail.SourceGames[0].Play.RootFileID {
		t.Fatalf("launch candidate/root mismatch: %+v vs %+v", detail.Play.LaunchCandidates[0], detail.SourceGames[0].Play)
	}
}

func TestCanonicalToGameDetailIncludesSourceBackedXcloudOptions(t *testing.T) {
	game := &core.CanonicalGame{
		ID:              "game-xcloud",
		Title:           "Final Fantasy",
		Platform:        core.PlatformWindowsPC,
		Kind:            core.GameKindBaseGame,
		XcloudAvailable: true,
		XcloudURL:       "https://xbox.example/play/primary",
		SourceGames: []*core.SourceGame{{
			ID:            "source-xbox",
			IntegrationID: "xbox-1",
			PluginID:      "game-source-xbox",
			ExternalID:    "product-1",
			RawTitle:      "FINAL FANTASY",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
			CreatedAt:     time.Unix(1700000000, 0),
			ResolverMatches: []core.ResolverMatch{{
				PluginID:        "game-source-xbox",
				Title:           "FINAL FANTASY",
				ExternalID:      "product-1",
				XcloudAvailable: true,
				XcloudURL:       "https://xbox.example/play/source-xbox",
			}},
		}},
	}

	ctrl := &GameController{}
	detail := ctrl.canonicalToGameDetailWithIntegrationLabels(context.Background(), game, map[string]string{"xbox-1": "Xbox"})
	var xcloudOptions []GameLaunchOptionDTO
	for _, option := range detail.Play.Options {
		if option.Kind == "xcloud" {
			xcloudOptions = append(xcloudOptions, option)
		}
	}
	if len(xcloudOptions) != 1 {
		t.Fatalf("xcloud options = %+v, want 1", detail.Play.Options)
	}
	if xcloudOptions[0].URL != "https://xbox.example/play/source-xbox" || xcloudOptions[0].SourceGameID != "source-xbox" {
		t.Fatalf("xcloud option = %+v, want source-backed URL", xcloudOptions[0])
	}
	if xcloudOptions[0].IntegrationLabel != "Xbox" || xcloudOptions[0].SourceTitle != "FINAL FANTASY" {
		t.Fatalf("xcloud source context = %+v, want Xbox FINAL FANTASY", xcloudOptions[0])
	}
	if detail.SourceGames[0].Save == nil || detail.SourceGames[0].Save.Access != "provider_opaque" || xcloudOptions[0].Save == nil || xcloudOptions[0].Save.Access != "provider_opaque" {
		t.Fatalf("Xbox save capabilities = source %+v, xCloud %+v", detail.SourceGames[0].Save, xcloudOptions[0].Save)
	}
}

func TestCanonicalToGameDetailAllowsRootlessScummVMLaunch(t *testing.T) {
	root := t.TempDir()
	game := &core.CanonicalGame{
		ID:       "game-2",
		Title:    "Quest for Glory",
		Platform: core.PlatformScummVM,
		Kind:     core.GameKindBaseGame,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-2",
				Platform:  core.PlatformScummVM,
				GroupKind: core.GroupKindSelfContained,
				RootPath:  root,
				Status:    "found",
				CreatedAt: time.Unix(1700000000, 0),
				Files: []core.GameFile{
					{GameID: "source-2", Path: "RESOURCE.MAP", Role: core.GameFileRoleRequired, Size: 1024},
					{GameID: "source-2", Path: "RESOURCE.001", Role: core.GameFileRoleRequired, Size: 2048},
				},
			},
		},
	}

	detail := canonicalToGameDetail(game)
	if detail.Play == nil || !detail.Play.Available {
		t.Fatalf("expected rootless scummvm source to be launchable, got %+v", detail.Play)
	}
	if len(detail.Play.LaunchCandidates) != 0 {
		t.Fatalf("expected no root-file launch candidates, got %+v", detail.Play.LaunchCandidates)
	}
	if detail.SourceGames[0].Play == nil || detail.SourceGames[0].Play.RootFileID != "" {
		t.Fatalf("expected no root file id for rootless scummvm source, got %+v", detail.SourceGames[0].Play)
	}
}

func TestCanonicalToGameDetailExcludesNonStreamableBrowserPlaySource(t *testing.T) {
	game := &core.CanonicalGame{
		ID:       "game-transport",
		Title:    "Aria of Sorrow",
		Platform: core.PlatformGBA,
		Kind:     core.GameKindBaseGame,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-drive",
				PluginID:  "game-source-google-drive",
				Platform:  core.PlatformGBA,
				GroupKind: core.GroupKindSelfContained,
				RootPath:  "Games/Platforms/Nintendo Game Boy Advance",
				Status:    "found",
				CreatedAt: time.Unix(1700000000, 0),
				Files: []core.GameFile{
					{GameID: "source-drive", Path: "Castlevania.zip", Role: core.GameFileRoleRoot, Size: 128},
				},
			},
		},
	}

	detail := canonicalToGameDetail(game)
	if detail.Play == nil {
		t.Fatal("expected play metadata")
	}
	if detail.Play.Available {
		t.Fatalf("expected non-streamable source to be excluded from launch sources, got %+v", detail.Play)
	}
	if len(detail.Play.LaunchSources) != 1 || detail.Play.LaunchSources[0].Launchable {
		t.Fatalf("expected non-streamable launch source to remain non-launchable, got %+v", detail.Play.LaunchSources)
	}
	if detail.SourceGames[0].Play == nil || detail.SourceGames[0].Play.Launchable {
		t.Fatalf("expected source to be marked non-launchable, got %+v", detail.SourceGames[0].Play)
	}
	if len(detail.Play.Options) != 1 || detail.Play.Options[0].Save != nil {
		t.Fatalf("non-launchable browser placeholder must not advertise save backup: %+v", detail.Play.Options)
	}
}

func TestGameControllerServePlayFileSupportsRange(t *testing.T) {
	root := t.TempDir()
	fullPath := filepath.Join(root, "roms", "game.bin")
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &fakeGameStore{
		game: &core.CanonicalGame{
			ID:       "game-1",
			Platform: core.PlatformPS1,
			SourceGames: []*core.SourceGame{
				{
					ID:        "source-1",
					Platform:  core.PlatformPS1,
					GroupKind: core.GroupKindSelfContained,
					RootPath:  root,
					Status:    "found",
					Files: []core.GameFile{
						{GameID: "source-1", Path: "roms/game.bin", Role: core.GameFileRoleRoot, Size: 6},
					},
				},
			},
		},
	}
	ctrl := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	r := chi.NewRouter()
	r.Get("/api/games/{id}/play", ctrl.ServePlayFile)

	req := httptest.NewRequest(http.MethodGet, "/api/games/game-1/play?file_id="+encodeGameFileID("source-1", "roms/game.bin"), nil)
	req.Header.Set("Range", "bytes=1-3")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusPartialContent {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "bcd" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Disposition"); got != `inline; filename=game.bin` {
		t.Fatalf("content-disposition = %q, want filename game.bin", got)
	}
}

func TestGameControllerServePlayFileSupportsHead(t *testing.T) {
	root := t.TempDir()
	fullPath := filepath.Join(root, "roms", "game.bin")
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &fakeGameStore{
		game: &core.CanonicalGame{
			ID:       "game-head",
			Platform: core.PlatformGBA,
			SourceGames: []*core.SourceGame{
				{
					ID:        "source-head",
					Platform:  core.PlatformGBA,
					GroupKind: core.GroupKindSelfContained,
					RootPath:  root,
					Status:    "found",
					Files: []core.GameFile{
						{GameID: "source-head", Path: "roms/game.bin", Role: core.GameFileRoleRoot, Size: 6},
					},
				},
			},
		},
	}
	ctrl := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	profiles := fakePlayProfileRepo{byID: map[string]*core.Profile{
		"profile-1": {ID: "profile-1", Role: core.ProfileRoleAdminPlayer},
	}}
	authService, err := auth.NewService(newLANAuthStore(), profiles)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	router := BuildRouter(
		&RouteBuilder{
			GameCtrl:        ctrl,
			MediaCtrl:       &MediaController{},
			DiscoCtrl:       &DiscoveryController{},
			AboutCtrl:       &AboutController{},
			ConfigCtrl:      &ConfigController{},
			PluginCtrl:      &PluginController{},
			ReviewCtrl:      &ReviewController{},
			AchievementCtrl: &AchievementController{},
			SyncCtrl:        &SyncController{},
			SaveSyncCtrl:    &SaveSyncController{},
			SSECtrl:         &SSEController{},
			OAuthCtrl:       &OAuthController{},
			ProfileRepo:     profiles,
			AuthService:     authService,
		},
		0,
		"",
	)

	req := httptest.NewRequest(http.MethodHead, "/api/games/game-head/play?profile_id=profile-1&file_id="+encodeGameFileID("source-head", "roms/game.bin"), nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Length"); got != "6" {
		t.Fatalf("expected content-length 6, got %q", got)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("expected empty head response body, got %q", rr.Body.String())
	}
}

func TestGameControllerServePlayFileRequiresProfileContext(t *testing.T) {
	ctrl := NewGameController(&fakeGameStore{}, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.With(ProfileContextMiddleware(fakePlayProfileRepo{byID: map[string]*core.Profile{
		"profile-1": {ID: "profile-1", Role: core.ProfileRolePlayer},
	}})).Get("/api/games/{id}/play", ctrl.ServePlayFile)

	for _, tc := range []struct {
		name string
		url  string
	}{
		{name: "missing", url: "/api/games/game-1/play?file_id=" + encodeGameFileID("source-1", "roms/game.bin")},
		{name: "invalid", url: "/api/games/game-1/play?profile_id=missing&file_id=" + encodeGameFileID("source-1", "roms/game.bin")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 body=%q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGameControllerServePlayFileRejectsCrossProfileGame(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "play.sqlite")
	database := dbpkg.NewSQLiteDatabase(noopLogger{}, restoreConfig{"DB_PATH": dbPath})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	profileRepo := dbpkg.NewProfileRepository(database)
	now := time.Now()
	for _, profile := range []*core.Profile{
		{ID: "profile-1", DisplayName: "Profile One", AvatarKey: "player-1", Role: core.ProfileRoleAdminPlayer, CreatedAt: now, UpdatedAt: now},
		{ID: "profile-2", DisplayName: "Profile Two", AvatarKey: "player-2", Role: core.ProfileRolePlayer, CreatedAt: now, UpdatedAt: now},
	} {
		if err := profileRepo.Create(context.Background(), profile); err != nil {
			t.Fatal(err)
		}
	}

	root := t.TempDir()
	fullPath := filepath.Join(root, "roms", "game.bin")
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("profile-two"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := dbpkg.NewGameStore(database, noopLogger{})
	profileTwoCtx := core.WithProfile(context.Background(), &core.Profile{ID: "profile-2", Role: core.ProfileRolePlayer})
	if err := store.PersistScanResults(profileTwoCtx, &core.ScanBatch{
		IntegrationID: "integration-2",
		SourceGames: []*core.SourceGame{{
			ID:            "source-2",
			IntegrationID: "integration-2",
			PluginID:      "game-source-steam",
			ExternalID:    "external-2",
			RawTitle:      "Profile Two Game",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			RootPath:      root,
			Status:        "found",
			Files: []core.GameFile{
				{GameID: "source-2", Path: "roms/game.bin", Role: core.GameFileRoleRoot, FileKind: "rom", Size: 11},
			},
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"source-2": {{PluginID: "metadata-steam", ExternalID: "match-2", Title: "Profile Two Game"}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}
	ids, err := store.GetVisibleCanonicalIDs(profileTwoCtx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("canonical ids = %+v, want 1", ids)
	}

	ctrl := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	router := chi.NewRouter()
	router.With(ProfileContextMiddleware(profileRepo)).Get("/api/games/{id}/play", ctrl.ServePlayFile)
	fileID := encodeGameFileID("source-2", "roms/game.bin")

	crossReq := httptest.NewRequest(http.MethodGet, "/api/games/"+ids[0]+"/play?profile_id=profile-1&file_id="+fileID, nil)
	crossRec := httptest.NewRecorder()
	router.ServeHTTP(crossRec, crossReq)
	if crossRec.Code != http.StatusNotFound {
		t.Fatalf("cross-profile status = %d, want 404 body=%q", crossRec.Code, crossRec.Body.String())
	}

	ownReq := httptest.NewRequest(http.MethodGet, "/api/games/"+ids[0]+"/play?profile_id=profile-2&file_id="+fileID, nil)
	ownRec := httptest.NewRecorder()
	router.ServeHTTP(ownRec, ownReq)
	if ownRec.Code != http.StatusOK {
		t.Fatalf("own-profile status = %d, want 200 body=%q", ownRec.Code, ownRec.Body.String())
	}
	if ownRec.Body.String() != "profile-two" {
		t.Fatalf("own-profile body = %q, want profile-two", ownRec.Body.String())
	}
}

func TestGameControllerServePlayFileServesCachedMaterializedFile(t *testing.T) {
	cacheRoot := t.TempDir()
	cachedFile := filepath.Join(cacheRoot, "prepared", "roms", "game.gba")
	if err := os.MkdirAll(filepath.Dir(cachedFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachedFile, []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &fakeGameStore{
		game: &core.CanonicalGame{
			ID:       "game-cache",
			Platform: core.PlatformGBA,
			SourceGames: []*core.SourceGame{
				{
					ID:        "source-cache",
					PluginID:  "game-source-google-drive",
					Platform:  core.PlatformGBA,
					GroupKind: core.GroupKindSelfContained,
					RootPath:  "Drive/Game",
					Status:    "found",
					Files: []core.GameFile{
						{GameID: "source-cache", Path: "roms/game.gba", Role: core.GameFileRoleRoot, Size: 6},
					},
				},
			},
		},
	}
	ctrl := NewGameController(store, nil, nil, nil, &fakeCacheService{resolvedPath: cachedFile}, noopLogger{})
	router := chi.NewRouter()
	router.Get("/api/games/{id}/play", ctrl.ServePlayFile)

	req := httptest.NewRequest(http.MethodGet, "/api/games/game-cache/play?file_id="+encodeGameFileID("source-cache", "roms/game.gba")+"&profile=browser.emulatorjs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "cached" {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestGameControllerServePlayFileRejectsInvalidFileID(t *testing.T) {
	ctrl := NewGameController(&fakeGameStore{}, nil, nil, nil, nil, noopLogger{})
	r := chi.NewRouter()
	r.Get("/api/games/{id}/play", ctrl.ServePlayFile)

	req := httptest.NewRequest(http.MethodGet, "/api/games/game-1/play?file_id=not-base64", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestGameControllerServePlayFileRejectsUnknownOwnedFile(t *testing.T) {
	store := &fakeGameStore{
		game: &core.CanonicalGame{
			ID:       "game-1",
			Platform: core.PlatformPS1,
			SourceGames: []*core.SourceGame{
				{
					ID:        "source-1",
					Platform:  core.PlatformPS1,
					GroupKind: core.GroupKindSelfContained,
					RootPath:  t.TempDir(),
					Status:    "found",
					Files: []core.GameFile{
						{GameID: "source-1", Path: "roms/game.bin", Role: core.GameFileRoleRoot, Size: 6},
					},
				},
			},
		},
	}
	ctrl := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	r := chi.NewRouter()
	r.Get("/api/games/{id}/play", ctrl.ServePlayFile)

	req := httptest.NewRequest(http.MethodGet, "/api/games/game-1/play?file_id="+encodeGameFileID("source-2", "roms/other.bin"), nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestGameControllerServePlayFileRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	store := &fakeGameStore{
		game: &core.CanonicalGame{
			ID:       "game-1",
			Platform: core.PlatformPS1,
			SourceGames: []*core.SourceGame{
				{
					ID:        "source-1",
					Platform:  core.PlatformPS1,
					GroupKind: core.GroupKindSelfContained,
					RootPath:  root,
					Status:    "found",
					Files: []core.GameFile{
						{GameID: "source-1", Path: "../evil.bin", Role: core.GameFileRoleRoot, Size: 6},
					},
				},
			},
		},
	}
	ctrl := NewGameController(store, nil, nil, nil, nil, noopLogger{})
	r := chi.NewRouter()
	r.Get("/api/games/{id}/play", ctrl.ServePlayFile)

	req := httptest.NewRequest(http.MethodGet, "/api/games/game-1/play?file_id="+encodeGameFileID("source-1", "../evil.bin"), nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestResolveSMBSharePathUsesIntegrationBasePath(t *testing.T) {
	got, err := resolveSMBSharePath("", "Mame/megaman.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Mame/megaman.zip" {
		t.Fatalf("got %q, want %q", got, "Mame/megaman.zip")
	}

	got, err = resolveSMBSharePath("Retro", "Roms/MS DOS/bonus/BON.EXE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Retro/Roms/MS DOS/bonus/BON.EXE" {
		t.Fatalf("got %q", got)
	}
}

func TestGameControllerOpenSMBPlayFileRejectsInvalidConfig(t *testing.T) {
	cfg, err := json.Marshal(map[string]any{
		"host": "TV2",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctrl := NewGameController(
		&fakeGameStore{},
		nil,
		nil,
		&fakePlayIntegrationRepo{
			byID: map[string]*core.Integration{
				"integ-1": {
					ID:         "integ-1",
					PluginID:   "game-source-smb",
					ConfigJSON: string(cfg),
				},
			},
		},
		nil,
		noopLogger{},
	)

	_, _, err = ctrl.openSMBPlayFile(context.Background(), &core.SourceGame{
		ID:            "source-1",
		IntegrationID: "integ-1",
		PluginID:      "game-source-smb",
	}, &core.GameFile{Path: "Mame/megaman.zip"})
	if err == nil {
		t.Fatal("expected invalid smb config error")
	}
}
