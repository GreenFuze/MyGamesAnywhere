package scan

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	dbstore "github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
)

type manualReviewTestConfig struct {
	dbPath string
}

func (c manualReviewTestConfig) Get(key string) string {
	if key == "DB_PATH" {
		return c.dbPath
	}
	return ""
}

func (manualReviewTestConfig) GetInt(string) int   { return 0 }
func (manualReviewTestConfig) GetBool(string) bool { return false }
func (manualReviewTestConfig) Validate() error     { return nil }

type manualReviewTestDiscovery struct {
	pluginIDs []string
}

func (d manualReviewTestDiscovery) GetPluginIDs() []string { return d.pluginIDs }
func (manualReviewTestDiscovery) GetPlugin(string) (*core.Plugin, bool) {
	return nil, false
}
func (d manualReviewTestDiscovery) GetPluginIDsProviding(method string) []string {
	if method == metadataGameLookupMethod {
		return d.pluginIDs
	}
	return nil
}

type manualReviewTestIntegrationRepo struct {
	items []*core.Integration
}

func (r manualReviewTestIntegrationRepo) Create(context.Context, *core.Integration) error { return nil }
func (r manualReviewTestIntegrationRepo) Update(context.Context, *core.Integration) error { return nil }
func (r manualReviewTestIntegrationRepo) Delete(context.Context, string) error            { return nil }
func (r manualReviewTestIntegrationRepo) List(context.Context) ([]*core.Integration, error) {
	return r.items, nil
}
func (r manualReviewTestIntegrationRepo) GetByID(_ context.Context, id string) (*core.Integration, error) {
	for _, item := range r.items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, nil
}
func (r manualReviewTestIntegrationRepo) ListByPluginID(_ context.Context, pluginID string) ([]*core.Integration, error) {
	var out []*core.Integration
	for _, item := range r.items {
		if item != nil && item.PluginID == pluginID {
			out = append(out, item)
		}
	}
	return out, nil
}

type countingMediaDownloadQueue struct {
	calls int
	err   error
}

func (q *countingMediaDownloadQueue) EnqueuePending(context.Context) error {
	q.calls++
	return q.err
}

func newManualReviewTestStore(t *testing.T) core.GameStore {
	t.Helper()
	store, _ := newManualReviewTestStoreWithDB(t)
	return store
}

func newManualReviewTestStoreWithDB(t *testing.T) (core.GameStore, *sql.DB) {
	t.Helper()
	cfg := manualReviewTestConfig{dbPath: filepath.Join(t.TempDir(), "manual-review.sqlite")}
	database := dbstore.NewSQLiteDatabase(testLogger{}, cfg)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	profile := &core.Profile{ID: "profile-1", DisplayName: "Player One", Role: core.ProfileRoleAdminPlayer, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := dbstore.NewProfileRepository(database).Create(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	return dbstore.NewGameStore(database, testLogger{}), database.GetDB()
}

func scanTestContext() context.Context {
	return core.WithProfile(context.Background(), &core.Profile{ID: "profile-1", DisplayName: "Player One", Role: core.ProfileRoleAdminPlayer})
}

func TestManualReviewServiceApplyPersistsSelectedMatchAndFillResult(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "source-1",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:manual-review-1",
			IntegrationID: "source-1",
			PluginID:      "game-source-steam",
			ExternalID:    "manual-review-1",
			RawTitle:      "mystery_setup",
			Platform:      core.PlatformUnknown,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindUnknown,
			RootPath:      "C:/Games/Mystery",
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			if method != metadataGameLookupMethod {
				return nil, nil
			}
			switch pluginID {
			case "metadata-other":
				return metadataLookupResponse{
					Results: []metadataMatch{{
						Index:      0,
						Title:      "Chosen Game",
						ExternalID: "other-1",
						Platform:   string(core.PlatformWindowsPC),
						Developer:  "Other Studio",
					}},
				}, nil
			default:
				return metadataLookupResponse{Results: nil}, nil
			}
		},
	}

	queue := &countingMediaDownloadQueue{}
	service := NewManualReviewService(
		caller,
		manualReviewTestDiscovery{pluginIDs: []string{"metadata-manual", "metadata-other"}},
		manualReviewTestIntegrationRepo{items: []*core.Integration{
			{ID: "meta-manual", PluginID: "metadata-manual", Label: "Manual Source", ConfigJSON: `{}`},
			{ID: "meta-other", PluginID: "metadata-other", Label: "Other Source", ConfigJSON: `{}`},
		}},
		store,
		queue,
		testLogger{},
	)

	err := service.Apply(ctx, "scan:manual-review-1", core.ManualReviewSelection{
		ProviderIntegrationID: "meta-manual",
		ProviderPluginID:      "metadata-manual",
		Title:                 "Chosen Game",
		Platform:              string(core.PlatformWindowsPC),
		Kind:                  string(core.GameKindBaseGame),
		ExternalID:            "manual-1",
		URL:                   "https://example.com/manual-1",
		Description:           "The selected record description.",
		ReleaseDate:           "1992",
		Genres:                []string{"Role-Playing"},
		Developer:             "Selected Studio",
		Publisher:             "Selected Publisher",
		Rating:                84,
		MaxPlayers:            2,
		ImageURL:              "https://example.com/manual-1-cover.png",
		Media: []core.MediaItem{{
			Type:   core.MediaTypeScreenshot,
			URL:    "https://example.com/manual-1-screen.png",
			Source: "metadata-manual",
		}},
	}, core.ManualReviewApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if queue.calls != 1 {
		t.Fatalf("enqueue calls = %d, want 1", queue.calls)
	}

	candidate, err := store.GetManualReviewCandidate(ctx, "scan:manual-review-1")
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil {
		t.Fatal("expected manual review candidate detail after apply")
	}
	if candidate.ReviewState != core.ManualReviewStateMatched {
		t.Fatalf("review_state = %q, want %q", candidate.ReviewState, core.ManualReviewStateMatched)
	}
	if len(candidate.ResolverMatches) != 2 {
		t.Fatalf("len(resolver_matches) = %d, want 2", len(candidate.ResolverMatches))
	}

	var manualMatch, filledMatch *core.ResolverMatch
	for i := range candidate.ResolverMatches {
		match := &candidate.ResolverMatches[i]
		if match.ManualSelection {
			manualMatch = match
		}
		if match.PluginID == "metadata-other" {
			filledMatch = match
		}
	}
	if manualMatch == nil || manualMatch.Title != "Chosen Game" || manualMatch.ExternalID != "manual-1" {
		t.Fatalf("manual match = %+v, want sticky chosen match", manualMatch)
	}
	if manualMatch.Description != "The selected record description." ||
		manualMatch.Developer != "Selected Studio" ||
		manualMatch.Publisher != "Selected Publisher" ||
		len(manualMatch.Media) != 2 {
		t.Fatalf("manual match metadata/media = %+v, want full selected evidence", manualMatch)
	}
	if filledMatch == nil || filledMatch.Outvoted {
		t.Fatalf("filled match = %+v, want corroborating fill result", filledMatch)
	}

	active, err := store.ListManualReviewCandidates(ctx, core.ManualReviewScopeActive, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("len(active) = %d, want 0 after apply", len(active))
	}

	game, err := store.GetCanonicalGameByID(ctx, candidate.CanonicalGameID)
	if err != nil {
		t.Fatal(err)
	}
	if game == nil {
		t.Fatal("expected canonical game after apply")
	}
	if game.Title != "Chosen Game" {
		t.Fatalf("canonical title = %q, want %q", game.Title, "Chosen Game")
	}
	if game.Description != "The selected record description." ||
		game.Developer != "Selected Studio" ||
		game.Publisher != "Selected Publisher" ||
		game.ReleaseDate != "1992" ||
		game.Rating != 84 ||
		game.MaxPlayers != 2 ||
		len(game.Media) != 2 {
		t.Fatalf("canonical metadata/media = %+v, want full selected evidence", game)
	}
}

func TestManualReviewServiceAuthoritativeReclassifyReplacesMatchesMovesCanonicalAndClearsOverrides(t *testing.T) {
	ctx := context.Background()
	store, sqlDB := newManualReviewTestStoreWithDB(t)

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "source-1",
		SourceGames: []*core.SourceGame{
			{
				ID:            "scan:target",
				IntegrationID: "source-1",
				PluginID:      "game-source-local",
				ExternalID:    "target",
				RawTitle:      "wrong game",
				Platform:      core.PlatformGenesis,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				RootPath:      "Games/Wrong",
				Status:        "found",
			},
			{
				ID:            "scan:old-mate",
				IntegrationID: "source-1",
				PluginID:      "game-source-local",
				ExternalID:    "old-mate",
				RawTitle:      "old mate",
				Platform:      core.PlatformGenesis,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				RootPath:      "Games/Old Mate",
				Status:        "found",
			},
			{
				ID:            "scan:new-mate",
				IntegrationID: "source-1",
				PluginID:      "game-source-local",
				ExternalID:    "new-mate",
				RawTitle:      "selected mate",
				Platform:      core.PlatformSNES,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				RootPath:      "Games/New Mate",
				Status:        "found",
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:target": {{
				PluginID:   "metadata-igdb",
				ExternalID: "old-game",
				Title:      "Wrong Game",
			}},
			"scan:old-mate": {{
				PluginID:   "metadata-igdb",
				ExternalID: "old-game",
				Title:      "Wrong Game",
			}},
			"scan:new-mate": {{
				PluginID:   "metadata-igdb",
				ExternalID: "selected-game",
				Title:      "Selected Game",
				Platform:   string(core.PlatformSNES),
			}},
		},
		MediaItems: map[string][]core.MediaRef{
			"scan:target": {
				{Type: core.MediaTypeCover, URL: "https://example.com/stale-cover.jpg", Source: "metadata-igdb"},
				{Type: core.MediaTypeArtwork, URL: "https://example.com/stale-art.jpg", Source: "metadata-igdb"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	oldCanonicalID := canonicalIDForSource(t, sqlDB, "scan:target")
	if got := canonicalIDForSource(t, sqlDB, "scan:old-mate"); got != oldCanonicalID {
		t.Fatalf("old mate canonical = %q, want initial target canonical %q", got, oldCanonicalID)
	}
	newMateCanonicalID := canonicalIDForSource(t, sqlDB, "scan:new-mate")
	if newMateCanonicalID == oldCanonicalID {
		t.Fatal("new mate unexpectedly started in target canonical")
	}

	coverAssetID := mediaAssetIDForSource(t, sqlDB, "scan:target", string(core.MediaTypeCover))
	artAssetID := mediaAssetIDForSource(t, sqlDB, "scan:target", string(core.MediaTypeArtwork))
	if err := store.SetCanonicalCoverOverride(ctx, oldCanonicalID, coverAssetID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCanonicalHoverOverride(ctx, oldCanonicalID, artAssetID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCanonicalBackgroundOverride(ctx, oldCanonicalID, artAssetID); err != nil {
		t.Fatal(err)
	}

	fillCalls := 0
	service := NewManualReviewService(
		&mockCaller{callFn: func(pluginID, method string, params any) (any, error) {
			if method == metadataGameLookupMethod && pluginID == "metadata-other" {
				fillCalls++
				return metadataLookupResponse{Results: []metadataMatch{{
					Index:      0,
					Title:      "Selected Game",
					ExternalID: "other-fill",
					Platform:   string(core.PlatformGenesis),
				}}}, nil
			}
			return metadataLookupResponse{}, nil
		}},
		manualReviewTestDiscovery{pluginIDs: []string{"metadata-igdb", "metadata-other"}},
		manualReviewTestIntegrationRepo{items: []*core.Integration{
			{ID: "meta-igdb", PluginID: "metadata-igdb", Label: "IGDB", ConfigJSON: `{}`},
			{ID: "meta-other", PluginID: "metadata-other", Label: "Other", ConfigJSON: `{}`},
		}},
		store,
		&countingMediaDownloadQueue{},
		testLogger{},
	)

	if err := service.Apply(ctx, "scan:target", core.ManualReviewSelection{
		ProviderIntegrationID: "meta-igdb",
		ProviderPluginID:      "metadata-igdb",
		Title:                 "Selected Game",
		Platform:              string(core.PlatformSNES),
		Kind:                  string(core.GameKindBaseGame),
		ExternalID:            "selected-game",
		URL:                   "https://example.com/selected",
		ImageURL:              "https://example.com/selected-cover.jpg",
		Media: []core.MediaItem{{
			Type:   core.MediaTypeScreenshot,
			URL:    "https://example.com/selected-screenshot.jpg",
			Source: "metadata-igdb",
		}},
	}, core.ManualReviewApplyOptions{AuthoritativeReclassify: true}); err != nil {
		t.Fatal(err)
	}
	if fillCalls != 0 {
		t.Fatalf("fill calls = %d, want 0 for authoritative reclassify", fillCalls)
	}

	targetCanonicalID := canonicalIDForSource(t, sqlDB, "scan:target")
	newMateCanonicalIDAfter := canonicalIDForSource(t, sqlDB, "scan:new-mate")
	if targetCanonicalID != newMateCanonicalIDAfter {
		t.Fatalf("target canonical = %q, want same canonical as selected match source %q", targetCanonicalID, newMateCanonicalIDAfter)
	}
	if got := canonicalIDForSource(t, sqlDB, "scan:old-mate"); got == targetCanonicalID {
		t.Fatal("target stayed grouped with old stale match")
	}
	targetCandidate, err := store.GetManualReviewCandidate(ctx, "scan:target")
	if err != nil {
		t.Fatal(err)
	}
	if targetCandidate == nil || targetCandidate.Platform != core.PlatformSNES {
		t.Fatalf("target platform = %+v, want selected platform %q", targetCandidate, core.PlatformSNES)
	}

	matches := resolverMatchesForSource(t, sqlDB, "scan:target")
	if len(matches) != 1 {
		t.Fatalf("target matches = %+v, want only selected match", matches)
	}
	if matches[0].pluginID != "metadata-igdb" || matches[0].externalID != "selected-game" || !matches[0].manualSelection || matches[0].outvoted {
		t.Fatalf("target selected match = %+v, want authoritative selected IGDB match", matches[0])
	}

	mediaURLs := mediaURLsForSource(t, sqlDB, "scan:target")
	if len(mediaURLs) != 2 || mediaURLs[0] != "https://example.com/selected-cover.jpg" || mediaURLs[1] != "https://example.com/selected-screenshot.jpg" {
		t.Fatalf("target media urls = %+v, want selected media plus cover fallback", mediaURLs)
	}

	assertNoCanonicalMediaOverrideRows(t, sqlDB, oldCanonicalID)
	assertNoCanonicalMediaOverrideRows(t, sqlDB, targetCanonicalID)
}

func TestManualReviewServiceApplyRejectsInvalidSelection(t *testing.T) {
	service := NewManualReviewService(
		&mockCaller{},
		manualReviewTestDiscovery{},
		manualReviewTestIntegrationRepo{},
		newManualReviewTestStore(t),
		nil,
		testLogger{},
	)

	err := service.Apply(context.Background(), "scan:any", core.ManualReviewSelection{}, core.ManualReviewApplyOptions{})
	if !errors.Is(err, core.ErrManualReviewSelectionInvalid) {
		t.Fatalf("error = %v, want %v", err, core.ErrManualReviewSelectionInvalid)
	}
}

func TestManualReviewServiceRedetectPersistsIdentifiedCandidateAndLeavesActiveQueue(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	seedManualReviewCandidate(t, ctx, store, &core.SourceGame{
		ID:            "scan:redetect-1",
		IntegrationID: "source-1",
		PluginID:      "game-source-local",
		ExternalID:    "redetect-1",
		RawTitle:      "aladdin_u",
		Platform:      core.PlatformUnknown,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		RootPath:      "C:/Games/Aladdin",
		Status:        "found",
	})

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			if method != metadataGameLookupMethod {
				return nil, nil
			}
			return metadataLookupResponse{
				Results: []metadataMatch{{
					Index:      0,
					Title:      "Aladdin",
					ExternalID: "igdb-aladdin",
					Platform:   string(core.PlatformWindowsPC),
					Kind:       string(core.GameKindBaseGame),
				}},
			}, nil
		},
	}
	queue := &countingMediaDownloadQueue{}
	service := NewManualReviewService(
		caller,
		manualReviewTestDiscovery{pluginIDs: []string{"metadata-igdb"}},
		manualReviewTestIntegrationRepo{items: []*core.Integration{
			{ID: "meta-igdb", PluginID: "metadata-igdb", Label: "IGDB", ConfigJSON: `{}`},
		}},
		store,
		queue,
		testLogger{},
	)

	result, err := service.Redetect(ctx, "scan:redetect-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ManualReviewRedetectStatusMatched {
		t.Fatalf("status = %q, want %q", result.Status, core.ManualReviewRedetectStatusMatched)
	}
	if result.MatchCount != 1 || result.ProviderCount != 1 {
		t.Fatalf("result = %+v, want one match and provider", result)
	}
	if queue.calls != 1 {
		t.Fatalf("enqueue calls = %d, want 1", queue.calls)
	}

	active, err := store.ListManualReviewCandidates(ctx, core.ManualReviewScopeActive, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("len(active) = %d, want 0 after re-detect", len(active))
	}

	candidate, err := store.GetManualReviewCandidate(ctx, "scan:redetect-1")
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil {
		t.Fatal("expected candidate detail")
	}
	if candidate.ResolverMatchCount != 1 {
		t.Fatalf("resolver_match_count = %d, want 1", candidate.ResolverMatchCount)
	}
	if candidate.ReviewState != core.ManualReviewStatePending {
		t.Fatalf("review_state = %q, want pending automatic match", candidate.ReviewState)
	}
}

func TestManualReviewServiceRedetectLeavesUnidentifiedPendingWithoutPersistence(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	seedManualReviewCandidate(t, ctx, store, &core.SourceGame{
		ID:            "scan:redetect-none",
		IntegrationID: "source-1",
		PluginID:      "game-source-local",
		ExternalID:    "redetect-none",
		RawTitle:      "unknown_game",
		Platform:      core.PlatformUnknown,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindUnknown,
		RootPath:      "C:/Games/Unknown",
		Status:        "found",
	})

	queue := &countingMediaDownloadQueue{}
	service := NewManualReviewService(
		&mockCaller{responses: map[string]any{"metadata-igdb": metadataLookupResponse{Results: nil}}},
		manualReviewTestDiscovery{pluginIDs: []string{"metadata-igdb"}},
		manualReviewTestIntegrationRepo{items: []*core.Integration{
			{ID: "meta-igdb", PluginID: "metadata-igdb", Label: "IGDB", ConfigJSON: `{}`},
		}},
		store,
		queue,
		testLogger{},
	)

	result, err := service.Redetect(ctx, "scan:redetect-none")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ManualReviewRedetectStatusUnidentified {
		t.Fatalf("status = %q, want %q", result.Status, core.ManualReviewRedetectStatusUnidentified)
	}
	if queue.calls != 0 {
		t.Fatalf("enqueue calls = %d, want 0", queue.calls)
	}

	candidate, err := store.GetManualReviewCandidate(ctx, "scan:redetect-none")
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil || candidate.ResolverMatchCount != 0 {
		t.Fatalf("candidate = %+v, want pending candidate with no persisted matches", candidate)
	}
	if candidate.ReviewState != core.ManualReviewStatePending {
		t.Fatalf("review_state = %q, want pending", candidate.ReviewState)
	}
}

func TestManualReviewServiceRedetectFailsFastWithoutPersistingPartialMatches(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	seedManualReviewCandidate(t, ctx, store, &core.SourceGame{
		ID:            "scan:redetect-fail",
		IntegrationID: "source-1",
		PluginID:      "game-source-local",
		ExternalID:    "redetect-fail",
		RawTitle:      "failing_game",
		Platform:      core.PlatformUnknown,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindUnknown,
		RootPath:      "C:/Games/Failing",
		Status:        "found",
	})

	queue := &countingMediaDownloadQueue{}
	service := NewManualReviewService(
		&mockCaller{callFn: func(pluginID, method string, params any) (any, error) {
			return nil, errors.New("provider offline")
		}},
		manualReviewTestDiscovery{pluginIDs: []string{"metadata-igdb"}},
		manualReviewTestIntegrationRepo{items: []*core.Integration{
			{ID: "meta-igdb", PluginID: "metadata-igdb", Label: "IGDB", ConfigJSON: `{}`},
		}},
		store,
		queue,
		testLogger{},
	)

	_, err := service.Redetect(ctx, "scan:redetect-fail")
	if !errors.Is(err, core.ErrMetadataProvidersUnavailable) {
		t.Fatalf("error = %v, want %v", err, core.ErrMetadataProvidersUnavailable)
	}
	if queue.calls != 0 {
		t.Fatalf("enqueue calls = %d, want 0", queue.calls)
	}

	candidate, err := store.GetManualReviewCandidate(ctx, "scan:redetect-fail")
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil || candidate.ResolverMatchCount != 0 {
		t.Fatalf("candidate = %+v, want no persisted resolver matches", candidate)
	}
}

func TestManualReviewServiceRedetectActiveFailsFastAfterPersistingEarlierSuccess(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	var sourceGames []*core.SourceGame
	for _, item := range []struct {
		id        string
		title     string
		groupKind core.GroupKind
	}{
		{id: "scan:redetect-batch-1", title: "aaa_known_game", groupKind: core.GroupKindSelfContained},
		{id: "scan:redetect-batch-2", title: "bbb_failing_game", groupKind: core.GroupKindUnknown},
		{id: "scan:redetect-batch-3", title: "ccc_should_not_run", groupKind: core.GroupKindUnknown},
	} {
		sourceGames = append(sourceGames, &core.SourceGame{
			ID:            item.id,
			IntegrationID: "source-1",
			PluginID:      "game-source-local",
			ExternalID:    item.id,
			RawTitle:      item.title,
			Platform:      core.PlatformUnknown,
			Kind:          core.GameKindBaseGame,
			GroupKind:     item.groupKind,
			Status:        "found",
		})
	}
	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID:   "source-1",
		SourceGames:     sourceGames,
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	var attempted []string
	queue := &countingMediaDownloadQueue{}
	service := NewManualReviewService(
		&mockCaller{callFn: func(pluginID, method string, params any) (any, error) {
			if method != metadataGameLookupMethod {
				return nil, nil
			}
			request, ok := params.(map[string]any)
			if !ok {
				return nil, errors.New("missing metadata lookup games")
			}
			games, ok := request["games"].([]metadataGameQuery)
			if !ok || len(games) == 0 {
				return nil, errors.New("missing metadata lookup games")
			}
			title := games[0].Title
			attempted = append(attempted, title)
			switch title {
			case "aaa_known_game":
				return metadataLookupResponse{Results: []metadataMatch{{
					Index:      0,
					Title:      "Known Game",
					ExternalID: "known-game",
					Platform:   string(core.PlatformWindowsPC),
					Kind:       string(core.GameKindBaseGame),
				}}}, nil
			case "bbb_failing_game":
				return nil, errors.New("provider offline")
			default:
				t.Fatalf("unexpected batch lookup after fail-fast: %s", title)
				return nil, nil
			}
		}},
		manualReviewTestDiscovery{pluginIDs: []string{"metadata-igdb"}},
		manualReviewTestIntegrationRepo{items: []*core.Integration{
			{ID: "meta-igdb", PluginID: "metadata-igdb", Label: "IGDB", ConfigJSON: `{}`},
		}},
		store,
		queue,
		testLogger{},
	)

	result, err := service.RedetectActive(ctx)
	if !errors.Is(err, core.ErrMetadataProvidersUnavailable) {
		t.Fatalf("error = %v, want %v", err, core.ErrMetadataProvidersUnavailable)
	}
	if result == nil {
		t.Fatal("expected partial batch result")
	}
	if result.Attempted != 2 || result.Matched != 1 || result.FailedCandidateID != "scan:redetect-batch-2" {
		t.Fatalf("result = %+v, want attempted 2, matched 1, failed second candidate", result)
	}
	if len(result.Results) != 1 || result.Results[0].CandidateID != "scan:redetect-batch-1" {
		t.Fatalf("results = %+v, want only first candidate result", result.Results)
	}
	if len(attempted) != 2 || attempted[0] != "aaa_known_game" || attempted[1] != "bbb_failing_game" {
		t.Fatalf("attempted lookups = %v, want first then second only", attempted)
	}
	if queue.calls != 1 {
		t.Fatalf("enqueue calls = %d, want 1 for earlier success only", queue.calls)
	}

	first, err := store.GetManualReviewCandidate(ctx, "scan:redetect-batch-1")
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || first.ResolverMatchCount != 1 {
		t.Fatalf("first candidate = %+v, want persisted match", first)
	}
	second, err := store.GetManualReviewCandidate(ctx, "scan:redetect-batch-2")
	if err != nil {
		t.Fatal(err)
	}
	if second == nil || second.ResolverMatchCount != 0 {
		t.Fatalf("second candidate = %+v, want no persisted partial match", second)
	}
	third, err := store.GetManualReviewCandidate(ctx, "scan:redetect-batch-3")
	if err != nil {
		t.Fatal(err)
	}
	if third == nil || third.ResolverMatchCount != 0 {
		t.Fatalf("third candidate = %+v, want unattempted candidate", third)
	}
}

func TestManualReviewServiceRedetectRejectsArchivedAndMatchedCandidates(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	seedManualReviewCandidate(t, ctx, store, &core.SourceGame{
		ID:            "scan:redetect-archived",
		IntegrationID: "source-1",
		PluginID:      "game-source-local",
		ExternalID:    "redetect-archived",
		RawTitle:      "archived_game",
		Platform:      core.PlatformUnknown,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindUnknown,
		Status:        "found",
	})
	seedManualReviewCandidate(t, ctx, store, &core.SourceGame{
		ID:            "scan:redetect-matched",
		IntegrationID: "source-2",
		PluginID:      "game-source-local",
		ExternalID:    "redetect-matched",
		RawTitle:      "matched_game",
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		Status:        "found",
	})
	if err := store.SetManualReviewState(ctx, "scan:redetect-archived", core.ManualReviewStateNotAGame); err != nil {
		t.Fatal(err)
	}
	if err := store.SetManualReviewState(ctx, "scan:redetect-matched", core.ManualReviewStateMatched); err != nil {
		t.Fatal(err)
	}

	service := NewManualReviewService(
		&mockCaller{},
		manualReviewTestDiscovery{},
		manualReviewTestIntegrationRepo{},
		store,
		nil,
		testLogger{},
	)

	for _, id := range []string{"scan:redetect-archived", "scan:redetect-matched"} {
		_, err := service.Redetect(ctx, id)
		if !errors.Is(err, core.ErrManualReviewCandidateNotEligible) {
			t.Fatalf("Redetect(%q) error = %v, want %v", id, err, core.ErrManualReviewCandidateNotEligible)
		}
	}
}

func TestManualReviewServiceRedetectResolvesN64AndDoesNotDropExistingDetectedGames(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "source-1",
		SourceGames: []*core.SourceGame{
			{
				ID:            "scan:existing-detected",
				IntegrationID: "source-1",
				PluginID:      "game-source-local",
				ExternalID:    "existing-detected",
				RawTitle:      "Existing Game",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			},
			{
				ID:            "scan:redetect-bomberman64",
				IntegrationID: "source-1",
				PluginID:      "game-source-local",
				ExternalID:    "redetect-bomberman64",
				RawTitle:      "Bomberman 64",
				Platform:      core.PlatformUnknown,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
				Files: []core.GameFile{{
					GameID:   "scan:redetect-bomberman64",
					Path:     "Roms/Nintendo 64/Bomberman 64.z64",
					FileName: "Bomberman 64.z64",
					Role:     core.GameFileRoleRoot,
					FileKind: "rom",
					Size:     1024,
				}},
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:existing-detected": {{
				PluginID:   "metadata-steam",
				Title:      "Existing Game",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "existing-match",
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	before, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 {
		t.Fatalf("canonical games before = %d, want 1", len(before))
	}

	service := NewManualReviewService(
		&mockCaller{callFn: func(pluginID, method string, params any) (any, error) {
			if method != metadataGameLookupMethod {
				return nil, nil
			}
			return metadataLookupResponse{
				Results: []metadataMatch{{
					Index:      0,
					Title:      "Bomberman 64",
					Platform:   string(core.PlatformN64),
					Kind:       string(core.GameKindBaseGame),
					ExternalID: "launchbox-bomberman64",
				}},
			}, nil
		}},
		manualReviewTestDiscovery{pluginIDs: []string{"metadata-launchbox"}},
		manualReviewTestIntegrationRepo{items: []*core.Integration{
			{ID: "meta-launchbox", PluginID: "metadata-launchbox", Label: "LaunchBox", ConfigJSON: `{}`},
		}},
		store,
		&countingMediaDownloadQueue{},
		testLogger{},
	)

	result, err := service.Redetect(ctx, "scan:redetect-bomberman64")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ManualReviewRedetectStatusMatched {
		t.Fatalf("status = %q, want %q", result.Status, core.ManualReviewRedetectStatusMatched)
	}

	after, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 {
		t.Fatalf("canonical games after = %d, want 2", len(after))
	}

	candidate, err := store.GetManualReviewCandidate(ctx, "scan:redetect-bomberman64")
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil {
		t.Fatal("expected updated candidate")
	}
	if candidate.Platform != core.PlatformN64 {
		t.Fatalf("candidate platform = %q, want %q", candidate.Platform, core.PlatformN64)
	}
}

func seedManualReviewCandidate(t *testing.T, ctx context.Context, store core.GameStore, sourceGame *core.SourceGame) {
	t.Helper()
	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID:   sourceGame.IntegrationID,
		SourceGames:     []*core.SourceGame{sourceGame},
		ResolverMatches: map[string][]core.ResolverMatch{},
		MediaItems:      map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}
}

func canonicalIDForSource(t *testing.T, db *sql.DB, sourceGameID string) string {
	t.Helper()
	var canonicalID string
	if err := db.QueryRow(`SELECT canonical_id FROM canonical_source_games_link WHERE source_game_id=?`, sourceGameID).Scan(&canonicalID); err != nil {
		t.Fatalf("canonical id for %s: %v", sourceGameID, err)
	}
	return canonicalID
}

func mediaAssetIDForSource(t *testing.T, db *sql.DB, sourceGameID string, mediaType string) int {
	t.Helper()
	var assetID int
	if err := db.QueryRow(`SELECT media_asset_id FROM source_game_media WHERE source_game_id=? AND type=?`, sourceGameID, mediaType).Scan(&assetID); err != nil {
		t.Fatalf("media asset id for %s/%s: %v", sourceGameID, mediaType, err)
	}
	return assetID
}

type storedResolverMatch struct {
	pluginID        string
	externalID      string
	outvoted        bool
	manualSelection bool
}

func resolverMatchesForSource(t *testing.T, db *sql.DB, sourceGameID string) []storedResolverMatch {
	t.Helper()
	rows, err := db.Query(`SELECT plugin_id, external_id, outvoted, manual_selection FROM metadata_resolver_matches WHERE source_game_id=? ORDER BY plugin_id, external_id`, sourceGameID)
	if err != nil {
		t.Fatalf("resolver matches for %s: %v", sourceGameID, err)
	}
	defer rows.Close()

	var out []storedResolverMatch
	for rows.Next() {
		var item storedResolverMatch
		var outvoted, manualSelection int
		if err := rows.Scan(&item.pluginID, &item.externalID, &outvoted, &manualSelection); err != nil {
			t.Fatalf("scan resolver match: %v", err)
		}
		item.outvoted = outvoted != 0
		item.manualSelection = manualSelection != 0
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("resolver match rows: %v", err)
	}
	return out
}

func mediaURLsForSource(t *testing.T, db *sql.DB, sourceGameID string) []string {
	t.Helper()
	rows, err := db.Query(`
		SELECT ma.url
		FROM source_game_media sgm
		JOIN media_assets ma ON ma.id=sgm.media_asset_id
		WHERE sgm.source_game_id=?
		ORDER BY ma.url`, sourceGameID)
	if err != nil {
		t.Fatalf("media urls for %s: %v", sourceGameID, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			t.Fatalf("scan media url: %v", err)
		}
		out = append(out, url)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("media url rows: %v", err)
	}
	return out
}

func assertNoCanonicalMediaOverrideRows(t *testing.T, db *sql.DB, canonicalID string) {
	t.Helper()
	for _, table := range []string{
		"canonical_game_cover_overrides",
		"canonical_game_hover_overrides",
		"canonical_game_background_overrides",
	} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE canonical_id=?`, canonicalID).Scan(&count); err != nil {
			t.Fatalf("count %s for %s: %v", table, canonicalID, err)
		}
		if count != 0 {
			t.Fatalf("%s rows for %s = %d, want 0", table, canonicalID, count)
		}
	}
}
