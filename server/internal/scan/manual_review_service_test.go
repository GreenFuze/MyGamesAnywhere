package scan

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

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

func newManualReviewTestStore(t *testing.T) core.GameStore {
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
	return dbstore.NewGameStore(database, testLogger{})
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

	service := NewManualReviewService(
		caller,
		manualReviewTestDiscovery{pluginIDs: []string{"metadata-manual", "metadata-other"}},
		manualReviewTestIntegrationRepo{items: []*core.Integration{
			{ID: "meta-manual", PluginID: "metadata-manual", Label: "Manual Source", ConfigJSON: `{}`},
			{ID: "meta-other", PluginID: "metadata-other", Label: "Other Source", ConfigJSON: `{}`},
		}},
		store,
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
		ImageURL:              "https://example.com/manual-1-cover.png",
	})
	if err != nil {
		t.Fatal(err)
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
}

func TestManualReviewServiceApplyRejectsInvalidSelection(t *testing.T) {
	service := NewManualReviewService(
		&mockCaller{},
		manualReviewTestDiscovery{},
		manualReviewTestIntegrationRepo{},
		newManualReviewTestStore(t),
		testLogger{},
	)

	err := service.Apply(context.Background(), "scan:any", core.ManualReviewSelection{})
	if !errors.Is(err, core.ErrManualReviewSelectionInvalid) {
		t.Fatalf("error = %v, want %v", err, core.ErrManualReviewSelectionInvalid)
	}
}
