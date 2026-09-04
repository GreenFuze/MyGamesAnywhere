package http

// Play metadata is still part of the live library JSON: a frontend needs to
// know whether a copy is streamable and which files back it, even though MGA
// itself no longer plays anything. These cover that projection, plus the guard
// that the retired play route serves a typed refusal rather than bytes.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/auth"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
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

func TestRetiredBrowserPlayRouteReturnsGoneWithoutServingFile(t *testing.T) {
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

	if rr.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d (%s)", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Length"); got != "" {
		t.Fatalf("retired route exposed file content-length %q", got)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("expected empty head response body, got %q", rr.Body.String())
	}
}
