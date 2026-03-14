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

func (c *mockConfig) Get(key string) string  { return c.values[key] }
func (c *mockConfig) GetInt(key string) int   { return 0 }
func (c *mockConfig) GetBool(key string) bool { return false }
func (c *mockConfig) Validate() error         { return nil }

type mockIntegrationRepo struct {
	integrations []*core.Integration
}

func (r *mockIntegrationRepo) Create(context.Context, *core.Integration) error              { return nil }
func (r *mockIntegrationRepo) Delete(context.Context, string) error                         { return nil }
func (r *mockIntegrationRepo) GetByID(context.Context, string) (*core.Integration, error)   { return nil, nil }
func (r *mockIntegrationRepo) List(context.Context) ([]*core.Integration, error) {
	return r.integrations, nil
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
	host := plugins.NewPluginHost(log, cfg, pm)

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

	caller := &callerWrapper{host: host, tv2Data: tv2Data}
	discovery := &discoveryWrapper{host: host}
	repo := &mockIntegrationRepo{integrations: integrations}

	orch := scan.NewOrchestrator(caller, discovery, repo, log)

	t.Log("Starting full pipeline run (this may take 15-30 minutes)...")
	start := time.Now()

	games, err := orch.RunScan(ctx, nil)
	if err != nil {
		t.Fatalf("RunScan: %v", err)
	}

	elapsed := time.Since(start)
	t.Logf("Pipeline completed in %s", elapsed.Round(time.Second))

	// ── Summary ──

	t.Logf("\n=== RESULTS ===")
	t.Logf("Total games discovered: %d", len(games))

	identified, unidentified := 0, 0
	totalExtIDs := 0
	resolverHits := map[string]int{}
	platformCounts := map[string]int{}
	groupKindCounts := map[string]int{}

	for _, g := range games {
		if g.Status == "identified" {
			identified++
		} else {
			unidentified++
		}
		totalExtIDs += len(g.ExternalIDs)
		for _, m := range g.ResolverMatches {
			if !m.Outvoted {
				resolverHits[m.PluginID]++
			}
		}
		platformCounts[string(g.Platform)]++
		groupKindCounts[string(g.GroupKind)]++
	}

	t.Logf("Identified:   %d (%.1f%%)", identified, pct(identified, len(games)))
	t.Logf("Unidentified: %d (%.1f%%)", unidentified, pct(unidentified, len(games)))
	t.Logf("Total external IDs: %d", totalExtIDs)

	t.Logf("\nPlatform breakdown:")
	for p, n := range platformCounts {
		t.Logf("  %-16s %d", p, n)
	}

	t.Logf("\nGroupKind breakdown:")
	for k, n := range groupKindCounts {
		t.Logf("  %-20s %d", k, n)
	}

	t.Logf("\nPer-resolver hit counts (non-outvoted matches):")
	for _, pid := range metaPlugins {
		t.Logf("  %-24s %d", pid, resolverHits[pid])
	}

	// ── Consensus details ──

	consensusUnanimous, consensusVoted := 0, 0
	for _, g := range games {
		if g.Status != "identified" {
			continue
		}
		outvotedCount := 0
		for _, m := range g.ResolverMatches {
			if m.Outvoted {
				outvotedCount++
			}
		}
		if outvotedCount > 0 {
			consensusVoted++
		} else {
			consensusUnanimous++
		}
	}
	t.Logf("\nConsensus: %d unanimous, %d with outvoted matches", consensusUnanimous, consensusVoted)

	// Show games where consensus had conflicts.
	if consensusVoted > 0 {
		t.Logf("\nGames with consensus conflicts:")
		shown := 0
		for _, g := range games {
			if g.Status != "identified" || shown >= 20 {
				break
			}
			hasOutvoted := false
			for _, m := range g.ResolverMatches {
				if m.Outvoted {
					hasOutvoted = true
					break
				}
			}
			if !hasOutvoted {
				continue
			}
			shown++
			t.Logf("  %q => %q", g.RawTitle, g.Title)
			for _, m := range g.ResolverMatches {
				tag := ""
				if m.Outvoted {
					tag = " [OUTVOTED]"
				}
				t.Logf("    %-24s %q  extID=%s%s", m.PluginID, m.Title, m.ExternalID, tag)
			}
		}
	}

	// ── Samples ──

	t.Logf("\nSample identified games (first 30):")
	showN(t, games, "identified", 30, func(t *testing.T, g *core.Game) {
		var resolvers []string
		for _, m := range g.ResolverMatches {
			tag := m.PluginID
			if m.Outvoted {
				tag += "(outvoted)"
			}
			resolvers = append(resolvers, tag)
		}
		t.Logf("  %q => %q [%s] resolvers: [%s]",
			g.RawTitle, g.Title, g.Platform, strings.Join(resolvers, ", "))
	})

	t.Logf("\nSample unidentified games (first 30):")
	showN(t, games, "unidentified", 30, func(t *testing.T, g *core.Game) {
		t.Logf("  [%s] %q (platform: %s, kind: %s)", g.GroupKind, g.RawTitle, g.Platform, g.Kind)
	})

	// ── Assertions (loose, just sanity checks) ──

	if len(games) == 0 {
		t.Fatal("no games discovered from TV2 data")
	}
	if identified == 0 {
		t.Fatal("no games were identified — all resolvers may have failed")
	}
	idRate := pct(identified, len(games))
	t.Logf("\nOverall identification rate: %.1f%%", idRate)
	if idRate < 10 {
		t.Errorf("identification rate too low (%.1f%%), expected at least 10%%", idRate)
	}
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}

func showN(t *testing.T, games []*core.Game, status string, limit int, fn func(*testing.T, *core.Game)) {
	t.Helper()
	shown := 0
	target := status
	if status == "unidentified" {
		target = ""
	}
	for _, g := range games {
		if shown >= limit {
			break
		}
		match := (status == "identified" && g.Status == "identified") ||
			(status == "unidentified" && g.Status != "identified")
		if !match {
			continue
		}
		_ = target
		fn(t, g)
		shown++
	}
	if shown == 0 {
		t.Logf("  (none)")
	}
}

func init() {
	// Ensure test binary can find its way relative to the workspace.
	if wd, err := os.Getwd(); err == nil {
		fmt.Fprintf(os.Stderr, "test working directory: %s\n", wd)
	}
}
