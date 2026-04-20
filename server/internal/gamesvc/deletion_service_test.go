package gamesvc

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
)

type deletionTestConfig struct {
	dbPath string
}

func (c deletionTestConfig) Get(key string) string {
	if key == "DB_PATH" {
		return c.dbPath
	}
	return ""
}

func (deletionTestConfig) GetInt(string) int   { return 0 }
func (deletionTestConfig) GetBool(string) bool { return false }
func (deletionTestConfig) Validate() error     { return nil }

type deletionTestLogger struct{}

func (deletionTestLogger) Info(string, ...any)         {}
func (deletionTestLogger) Error(string, error, ...any) {}
func (deletionTestLogger) Debug(string, ...any)        {}
func (deletionTestLogger) Warn(string, ...any)         {}

type deletionTestIntegrationRepo struct {
	items map[string]*core.Integration
}

func (r deletionTestIntegrationRepo) Create(context.Context, *core.Integration) error { return nil }
func (r deletionTestIntegrationRepo) Update(context.Context, *core.Integration) error { return nil }
func (r deletionTestIntegrationRepo) Delete(context.Context, string) error            { return nil }
func (r deletionTestIntegrationRepo) List(context.Context) ([]*core.Integration, error) {
	out := make([]*core.Integration, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, item)
	}
	return out, nil
}
func (r deletionTestIntegrationRepo) GetByID(_ context.Context, id string) (*core.Integration, error) {
	return r.items[id], nil
}
func (r deletionTestIntegrationRepo) ListByPluginID(_ context.Context, pluginID string) ([]*core.Integration, error) {
	var out []*core.Integration
	for _, item := range r.items {
		if item != nil && item.PluginID == pluginID {
			out = append(out, item)
		}
	}
	return out, nil
}

type deletionTestPluginCaller struct {
	calls []struct {
		pluginID string
		method   string
		params   map[string]any
	}
	err error
}

func (c *deletionTestPluginCaller) Call(_ context.Context, pluginID string, method string, params any, result any) error {
	body, _ := json.Marshal(params)
	decoded := map[string]any{}
	_ = json.Unmarshal(body, &decoded)
	c.calls = append(c.calls, struct {
		pluginID string
		method   string
		params   map[string]any
	}{pluginID: pluginID, method: method, params: decoded})
	if c.err != nil {
		return c.err
	}
	if result != nil {
		payload, _ := json.Marshal(map[string]any{"deleted_count": 1})
		_ = json.Unmarshal(payload, result)
	}
	return nil
}

func TestDeletionServiceDeletesOneEligibleSourceRecord(t *testing.T) {
	ctx := context.Background()
	store := newDeletionTestStore(t)
	canonicalID := persistDeletionTestSources(t, ctx, store, true)

	repo := deletionTestIntegrationRepo{items: map[string]*core.Integration{
		"source-a": {
			ID:         "source-a",
			PluginID:   "game-source-smb",
			ConfigJSON: `{"host":"test","share":"games","username":"u","password":"p","include_paths":[{"path":"Games","recursive":true}]}`,
		},
		"source-b": {
			ID:         "source-b",
			PluginID:   "game-source-smb",
			ConfigJSON: `{"host":"test","share":"games","username":"u","password":"p","include_paths":[{"path":"Games","recursive":true}]}`,
		},
	}}
	caller := &deletionTestPluginCaller{}
	service := NewDeletionService(store, repo, caller, deletionTestLogger{})

	result, err := service.DeleteSourceGame(ctx, canonicalID, "scan:source-a")
	if err != nil {
		t.Fatalf("DeleteSourceGame: %v", err)
	}
	if !result.CanonicalExists {
		t.Fatal("expected canonical game to remain after deleting one sibling source")
	}
	if result.CanonicalGame == nil || len(result.CanonicalGame.SourceGames) != 1 || result.CanonicalGame.SourceGames[0].ID != "scan:source-b" {
		t.Fatalf("remaining canonical game = %+v, want only scan:source-b", result.CanonicalGame)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("plugin delete calls = %d, want 1", len(caller.calls))
	}
	if caller.calls[0].method != sourceFilesystemDeleteMethod {
		t.Fatalf("plugin delete method = %q, want %q", caller.calls[0].method, sourceFilesystemDeleteMethod)
	}
}

func TestDeletionServiceRejectsIneligibleSourceWithoutMutation(t *testing.T) {
	ctx := context.Background()
	store := newDeletionTestStore(t)
	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "source-epic",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:epic-source",
			IntegrationID: "source-epic",
			PluginID:      "game-source-epic",
			ExternalID:    "epic-1",
			RawTitle:      "Epic Game",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			RootPath:      "Games/Epic",
			Status:        "found",
			Files: []core.GameFile{{
				GameID:   "scan:epic-source",
				Path:     "Epic.exe",
				FileName: "Epic.exe",
				Role:     core.GameFileRoleRoot,
				FileKind: "exe",
				Size:     4096,
			}},
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:epic-source": {{
				PluginID:   "metadata-steam",
				Title:      "Epic Game",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "steam-epic",
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	games, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("canonical games = %d, want 1", len(games))
	}

	repo := deletionTestIntegrationRepo{items: map[string]*core.Integration{
		"source-epic": {ID: "source-epic", PluginID: "game-source-epic", ConfigJSON: `{}`},
	}}
	caller := &deletionTestPluginCaller{}
	service := NewDeletionService(store, repo, caller, deletionTestLogger{})

	_, err = service.DeleteSourceGame(ctx, games[0].ID, "scan:epic-source")
	if !errors.Is(err, core.ErrSourceGameDeleteNotEligible) {
		t.Fatalf("error = %v, want %v", err, core.ErrSourceGameDeleteNotEligible)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("plugin delete calls = %d, want 0", len(caller.calls))
	}

	remaining, err := store.GetCanonicalGameByID(ctx, games[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if remaining == nil || len(remaining.SourceGames) != 1 {
		t.Fatalf("remaining canonical game = %+v, want unchanged game", remaining)
	}
}

func newDeletionTestStore(t *testing.T) core.GameStore {
	t.Helper()

	cfg := deletionTestConfig{dbPath: filepath.Join(t.TempDir(), "deletion.sqlite")}
	database := db.NewSQLiteDatabase(deletionTestLogger{}, cfg)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	return db.NewGameStore(database, deletionTestLogger{})
}

func persistDeletionTestSources(t *testing.T, ctx context.Context, store core.GameStore, withSibling bool) string {
	t.Helper()

	sourceA := &core.SourceGame{
		ID:            "scan:source-a",
		IntegrationID: "source-a",
		PluginID:      "game-source-smb",
		ExternalID:    "source-a",
		RawTitle:      "Alpha",
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		RootPath:      "Games/Alpha",
		Status:        "found",
		Files: []core.GameFile{{
			GameID:   "scan:source-a",
			Path:     "Alpha.exe",
			FileName: "Alpha.exe",
			Role:     core.GameFileRoleRoot,
			FileKind: "exe",
			Size:     4096,
		}},
	}
	matchesA := map[string][]core.ResolverMatch{
		sourceA.ID: {{
			PluginID:   "metadata-steam",
			Title:      "Alpha",
			Platform:   string(core.PlatformWindowsPC),
			ExternalID: "match-alpha",
		}},
	}
	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID:   sourceA.IntegrationID,
		SourceGames:     []*core.SourceGame{sourceA},
		ResolverMatches: matchesA,
		MediaItems:      map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}
	if withSibling {
		sourceB := &core.SourceGame{
			ID:            "scan:source-b",
			IntegrationID: "source-b",
			PluginID:      "game-source-smb",
			ExternalID:    "source-b",
			RawTitle:      "Alpha (Version B)",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			RootPath:      "Games/Beta",
			Status:        "found",
			Files: []core.GameFile{{
				GameID:   "scan:source-b",
				Path:     "Beta.exe",
				FileName: "Beta.exe",
				Role:     core.GameFileRoleRoot,
				FileKind: "exe",
				Size:     4096,
			}},
		}
		if err := store.PersistScanResults(ctx, &core.ScanBatch{
			IntegrationID: sourceB.IntegrationID,
			SourceGames:   []*core.SourceGame{sourceB},
			ResolverMatches: map[string][]core.ResolverMatch{
				sourceB.ID: {{
					PluginID:   "metadata-steam",
					Title:      "Alpha",
					Platform:   string(core.PlatformWindowsPC),
					ExternalID: "match-alpha",
				}},
			},
			MediaItems: map[string][]core.MediaRef{},
		}); err != nil {
			t.Fatal(err)
		}
	}

	games, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("canonical games = %d, want 1", len(games))
	}
	return games[0].ID
}
