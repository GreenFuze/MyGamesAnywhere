package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/logger"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
)

const (
	envKey     = "ORCHESTRATOR_INTEGRATION"
	mockSource = "file-list-mock"
)

// ── Mocks ───────────────────────────────────────────────────────────

type mockConfig struct {
	values map[string]string
}

func (c *mockConfig) Get(key string) string   { return c.values[key] }
func (c *mockConfig) GetInt(key string) int   { return 0 }
func (c *mockConfig) GetBool(key string) bool { return false }
func (c *mockConfig) Validate() error         { return nil }

type mockIntegrationRepo struct {
	integrations []*core.Integration
}

func (r *mockIntegrationRepo) Create(context.Context, *core.Integration) error { return nil }
func (r *mockIntegrationRepo) Update(context.Context, *core.Integration) error { return nil }
func (r *mockIntegrationRepo) Delete(context.Context, string) error            { return nil }
func (r *mockIntegrationRepo) GetByID(context.Context, string) (*core.Integration, error) {
	return nil, nil
}
func (r *mockIntegrationRepo) List(context.Context) ([]*core.Integration, error) {
	return r.integrations, nil
}

func (r *mockIntegrationRepo) ListByPluginID(_ context.Context, pluginID string) ([]*core.Integration, error) {
	if pluginID == "" {
		return nil, nil
	}
	var out []*core.Integration
	for _, in := range r.integrations {
		if in != nil && in.PluginID == pluginID {
			out = append(out, in)
		}
	}
	return out, nil
}

// callerWrapper intercepts calls for the fake source plugin and delegates
// everything else to the real PluginHost.
type callerWrapper struct {
	host    plugins.PluginHost
	tv2Data json.RawMessage
}

func (w *callerWrapper) Call(ctx context.Context, pluginID, method string, params any, result any) error {
	if pluginID == mockSource && method == "source.filesystem.list" {
		wrapped := map[string]json.RawMessage{"files": w.tv2Data}
		data, err := json.Marshal(wrapped)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, result)
	}
	return w.host.Call(ctx, pluginID, method, params, result)
}

// discoveryWrapper adds the fake source plugin to the real host's set.
type discoveryWrapper struct {
	host plugins.PluginHost
}

func (w *discoveryWrapper) GetPluginIDs() []string {
	return append(w.host.GetPluginIDs(), mockSource)
}

func (w *discoveryWrapper) GetPlugin(pluginID string) (*core.Plugin, bool) {
	if pluginID == mockSource {
		return &core.Plugin{
			Manifest: core.PluginManifest{
				ID:       mockSource,
				Provides: []string{"source.filesystem.list"},
			},
			Enabled: true,
		}, true
	}
	return w.host.GetPlugin(pluginID)
}

func (w *discoveryWrapper) GetPluginIDsProviding(method string) []string {
	ids := w.host.GetPluginIDsProviding(method)
	if method == "source.filesystem.list" {
		ids = append(ids, mockSource)
	}
	return ids
}

// ── Test ────────────────────────────────────────────────────────────

func TestOrchestrator_FullPipeline(t *testing.T) {
	if os.Getenv(envKey) != "1" {
		t.Skip("set ORCHESTRATOR_INTEGRATION=1 to run")
	}

	binDir, err := filepath.Abs(filepath.Join("..", "bin"))
	if err != nil {
		t.Fatal(err)
	}
	pluginsDir := filepath.Join(binDir, "plugins")

	tv2Path := filepath.Join("..", "internal", "scan", "scanner", "testdata", "tv2_games.json")
	tv2Data, err := os.ReadFile(tv2Path)
	if err != nil {
		t.Fatalf("read tv2_games.json: %v", err)
	}

	log := logger.NewLogService()

	cfg := &mockConfig{values: map[string]string{
		"PLUGINS_DIR": pluginsDir,
	}}
	pm := plugins.NewProcessManager()
	host := plugins.NewPluginHost(log, cfg, pm, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	if err := host.Discover(ctx); err != nil {
		t.Fatalf("discover plugins: %v", err)
	}
	defer host.Close()

	metaPlugins := host.GetPluginIDsProviding("metadata.game.lookup")
	sort.Strings(metaPlugins)
	t.Logf("Discovered %d metadata plugins: %v", len(metaPlugins), metaPlugins)

	now := time.Now()
	integrations := []*core.Integration{
		{
			ID: "source-tv2", PluginID: mockSource,
			Label: "TV2 Test Data", IntegrationType: "source",
			CreatedAt: now, UpdatedAt: now,
		},
	}
	for _, pid := range metaPlugins {
		integrations = append(integrations, &core.Integration{
			ID: "meta-" + pid, PluginID: pid,
			Label: pid, IntegrationType: "metadata",
			CreatedAt: now, UpdatedAt: now,
		})
	}

	// Set up in-memory SQLite for the GameStore.
	testCfg := &mockConfig{values: map[string]string{
		"PLUGINS_DIR": pluginsDir,
		"DB_PATH":     ":memory:",
	}}
	testDB := db.NewSQLiteDatabase(log, testCfg)
	if err := testDB.Connect(); err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer testDB.Close()
	if err := testDB.EnsureSchema(); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	gameStore := db.NewGameStore(testDB, log)

	caller := &callerWrapper{host: host, tv2Data: tv2Data}
	discovery := &discoveryWrapper{host: host}
	repo := &mockIntegrationRepo{integrations: integrations}

	orch := scan.NewOrchestrator(caller, discovery, repo, gameStore, log)

	t.Log("Starting full pipeline run (this may take 15-30 minutes)...")
	start := time.Now()

	canonical, err := orch.RunScan(ctx, nil)
	if err != nil {
		t.Fatalf("RunScan: %v", err)
	}

	elapsed := time.Since(start)
	t.Logf("Pipeline completed in %s", elapsed.Round(time.Second))

	// ── Summary ──

	t.Logf("\n=== RESULTS ===")
	t.Logf("Total canonical games: %d", len(canonical))

	totalExtIDs := 0
	resolverHits := map[string]int{}
	platformCounts := map[string]int{}

	for _, cg := range canonical {
		totalExtIDs += len(cg.ExternalIDs)
		for _, sg := range cg.SourceGames {
			for _, m := range sg.ResolverMatches {
				if !m.Outvoted {
					resolverHits[m.PluginID]++
				}
			}
		}
		platformCounts[string(cg.Platform)]++
	}

	t.Logf("Total external IDs: %d", totalExtIDs)

	t.Logf("\nPlatform breakdown:")
	for p, n := range platformCounts {
		t.Logf("  %-16s %d", p, n)
	}

	t.Logf("\nPer-resolver hit counts (non-outvoted matches):")
	for _, pid := range metaPlugins {
		t.Logf("  %-24s %d", pid, resolverHits[pid])
	}

	// ── Samples ──

	t.Logf("\nSample canonical games (first 30):")
	for i, cg := range canonical {
		if i >= 30 {
			break
		}
		var resolvers []string
		for _, sg := range cg.SourceGames {
			for _, m := range sg.ResolverMatches {
				tag := m.PluginID
				if m.Outvoted {
					tag += "(outvoted)"
				}
				resolvers = append(resolvers, tag)
			}
		}
		t.Logf("  %q [%s] resolvers: [%s]",
			cg.Title, cg.Platform, strings.Join(resolvers, ", "))
	}

	// ── Enrichment coverage ──

	hasDesc, hasDate, hasGenres := 0, 0, 0
	hasDev, hasPub, hasMedia := 0, 0, 0
	hasRating, hasPlayers := 0, 0
	totalMediaItems := 0

	for _, cg := range canonical {
		if cg.Description != "" {
			hasDesc++
		}
		if cg.ReleaseDate != "" {
			hasDate++
		}
		if len(cg.Genres) > 0 {
			hasGenres++
		}
		if cg.Developer != "" {
			hasDev++
		}
		if cg.Publisher != "" {
			hasPub++
		}
		if len(cg.Media) > 0 {
			hasMedia++
			totalMediaItems += len(cg.Media)
		}
		if cg.Rating > 0 {
			hasRating++
		}
		if cg.MaxPlayers > 0 {
			hasPlayers++
		}
	}

	total := len(canonical)
	t.Logf("\nEnrichment coverage (of %d canonical games):", total)
	t.Logf("  Description:  %d (%.0f%%)", hasDesc, pct(hasDesc, total))
	t.Logf("  ReleaseDate:  %d (%.0f%%)", hasDate, pct(hasDate, total))
	t.Logf("  Genres:       %d (%.0f%%)", hasGenres, pct(hasGenres, total))
	t.Logf("  Developer:    %d (%.0f%%)", hasDev, pct(hasDev, total))
	t.Logf("  Publisher:    %d (%.0f%%)", hasPub, pct(hasPub, total))
	t.Logf("  Media:        %d (%.0f%%), total items: %d", hasMedia, pct(hasMedia, total), totalMediaItems)
	t.Logf("  Rating:       %d (%.0f%%)", hasRating, pct(hasRating, total))
	t.Logf("  MaxPlayers:   %d (%.0f%%)", hasPlayers, pct(hasPlayers, total))

	// ── Assertions (loose, just sanity checks) ──

	if len(canonical) == 0 {
		t.Fatal("no games discovered from TV2 data")
	}
	t.Logf("\nTotal canonical games: %d", len(canonical))
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}

func init() {
	// Ensure test binary can find its way relative to the workspace.
	if wd, err := os.Getwd(); err == nil {
		fmt.Fprintf(os.Stderr, "test working directory: %s\n", wd)
	}
}
