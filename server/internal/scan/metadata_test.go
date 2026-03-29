package scan

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// mockCaller simulates plugin IPC. callFn receives (pluginID, method, params)
// and returns (response, error). If callFn is nil, falls back to the
// static responses map.
type mockCaller struct {
	callFn    func(pluginID, method string, params any) (any, error)
	responses map[string]any
}

func (m *mockCaller) Call(_ context.Context, pluginID string, method string, params any, result any) error {
	var resp any
	if m.callFn != nil {
		r, err := m.callFn(pluginID, method, params)
		if err != nil {
			return err
		}
		resp = r
	} else {
		resp = m.responses[pluginID]
	}
	if resp == nil {
		return nil
	}
	b, _ := json.Marshal(resp)
	return json.Unmarshal(b, result)
}

type testLogger struct{}

func (l testLogger) Info(msg string, args ...any)             {}
func (l testLogger) Error(msg string, err error, args ...any) {}
func (l testLogger) Debug(msg string, args ...any)            {}
func (l testLogger) Warn(msg string, args ...any)             {}

// ── normalizeForConsensus unit tests ────────────────────────────────

func TestNormalizeForConsensus(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Donkey Kong 3", "donkey kong 3"},
		{"DOOM", "doom"},
		{"Doom", "doom"},
		{"Half-Life 2", "half life 2"},
		{"Castlevania: Symphony of the Night", "castlevania symphony of the night"},
		{"  Spaced  Out  ", "spaced out"},
	}
	for _, tc := range tests {
		got := normalizeForConsensus(tc.in)
		if got != tc.want {
			t.Errorf("normalizeForConsensus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ── Phase 2: consensus unit tests ───────────────────────────────────

func TestConsensus_Unanimous(t *testing.T) {
	g := &core.Game{
		Title:    "raw",
		RawTitle: "raw",
		Platform: core.PlatformArcade,
		Kind:     core.GameKindBaseGame,
		ResolverMatches: []core.ResolverMatch{
			{PluginID: "plugin-a", Title: "Donkey Kong", ExternalID: "a1"},
			{PluginID: "plugin-b", Title: "Donkey Kong", ExternalID: "b1"},
		},
	}
	sources := []MetadataSource{
		{PluginID: "plugin-a"},
		{PluginID: "plugin-b"},
	}
	runConsensus(g, sources)

	if g.Title != "Donkey Kong" {
		t.Errorf("title: got %q, want %q", g.Title, "Donkey Kong")
	}
	if g.Status != "identified" {
		t.Errorf("status: got %q, want %q", g.Status, "identified")
	}
	for _, m := range g.ResolverMatches {
		if m.Outvoted {
			t.Errorf("match from %s should NOT be outvoted", m.PluginID)
		}
	}
	if len(g.ExternalIDs) != 2 {
		t.Errorf("external IDs: got %d, want 2", len(g.ExternalIDs))
	}
}

func TestConsensus_MajorityWins(t *testing.T) {
	g := &core.Game{
		Title:    "raw",
		RawTitle: "raw",
		Platform: core.PlatformArcade,
		Kind:     core.GameKindBaseGame,
		ResolverMatches: []core.ResolverMatch{
			{PluginID: "plugin-a", Title: "Pac-Man", ExternalID: "a1"},
			{PluginID: "plugin-b", Title: "Pac-Man", ExternalID: "b1"},
			{PluginID: "plugin-c", Title: "Ms. Pac-Man", ExternalID: "c1"},
		},
	}
	sources := []MetadataSource{
		{PluginID: "plugin-a"},
		{PluginID: "plugin-b"},
		{PluginID: "plugin-c"},
	}
	runConsensus(g, sources)

	if g.Title != "Pac-Man" {
		t.Errorf("title: got %q, want %q", g.Title, "Pac-Man")
	}
	if !g.ResolverMatches[2].Outvoted {
		t.Error("plugin-c match should be outvoted")
	}
	if g.ResolverMatches[0].Outvoted || g.ResolverMatches[1].Outvoted {
		t.Error("plugin-a and plugin-b matches should NOT be outvoted")
	}
	// Only non-outvoted external IDs.
	if len(g.ExternalIDs) != 2 {
		t.Errorf("external IDs: got %d, want 2", len(g.ExternalIDs))
	}
}

func TestConsensus_TieBrokenByPriority(t *testing.T) {
	g := &core.Game{
		Title:    "raw",
		RawTitle: "raw",
		Platform: core.PlatformUnknown,
		Kind:     core.GameKindBaseGame,
		ResolverMatches: []core.ResolverMatch{
			{PluginID: "plugin-a", Title: "Correct Title", ExternalID: "a1"},
			{PluginID: "plugin-b", Title: "Wrong Title", ExternalID: "b1"},
		},
	}
	sources := []MetadataSource{
		{PluginID: "plugin-a"},
		{PluginID: "plugin-b"},
	}
	runConsensus(g, sources)

	if g.Title != "Correct Title" {
		t.Errorf("title: got %q, want %q (higher priority should win tie)", g.Title, "Correct Title")
	}
	if g.ResolverMatches[0].Outvoted {
		t.Error("plugin-a should NOT be outvoted (higher priority)")
	}
	if !g.ResolverMatches[1].Outvoted {
		t.Error("plugin-b should be outvoted (lower priority)")
	}
}

func TestConsensus_CaseInsensitive(t *testing.T) {
	g := &core.Game{
		Title:    "raw",
		RawTitle: "raw",
		Platform: core.PlatformWindowsPC,
		Kind:     core.GameKindBaseGame,
		ResolverMatches: []core.ResolverMatch{
			{PluginID: "plugin-a", Title: "DOOM", ExternalID: "a1"},
			{PluginID: "plugin-b", Title: "Doom", ExternalID: "b1"},
			{PluginID: "plugin-c", Title: "doom", ExternalID: "c1"},
		},
	}
	sources := []MetadataSource{
		{PluginID: "plugin-a"},
		{PluginID: "plugin-b"},
		{PluginID: "plugin-c"},
	}
	runConsensus(g, sources)

	// All three normalize to "doom" — unanimous.
	for _, m := range g.ResolverMatches {
		if m.Outvoted {
			t.Errorf("match from %s should NOT be outvoted (all normalize to 'doom')", m.PluginID)
		}
	}
	// Title comes from highest priority (plugin-a).
	if g.Title != "DOOM" {
		t.Errorf("title: got %q, want %q (highest priority literal)", g.Title, "DOOM")
	}
	if len(g.ExternalIDs) != 3 {
		t.Errorf("external IDs: got %d, want 3", len(g.ExternalIDs))
	}
}

func TestConsensus_FillBlanks(t *testing.T) {
	g := &core.Game{
		Title:    "raw",
		RawTitle: "raw",
		Platform: core.PlatformUnknown,
		Kind:     core.GameKindBaseGame,
		ResolverMatches: []core.ResolverMatch{
			{PluginID: "plugin-a", Title: "Game A", ExternalID: "a1"},
			{PluginID: "plugin-b", Title: "Game A", Platform: "ps1", Kind: "dlc", ParentGameID: "parent-1", ExternalID: "b1"},
		},
	}
	sources := []MetadataSource{
		{PluginID: "plugin-a"},
		{PluginID: "plugin-b"},
	}
	runConsensus(g, sources)

	if g.Title != "Game A" {
		t.Errorf("title: got %q, want %q", g.Title, "Game A")
	}
	if g.Platform != core.PlatformPS1 {
		t.Errorf("platform: got %q, want %q", g.Platform, core.PlatformPS1)
	}
	if g.Kind != core.GameKindDLC {
		t.Errorf("kind: got %q, want %q", g.Kind, core.GameKindDLC)
	}
	if g.ParentGameID != "parent-1" {
		t.Errorf("parent: got %q, want %q", g.ParentGameID, "parent-1")
	}
}

// ── Full pipeline tests (all 3 phases) ──────────────────────────────

func TestEnrich_BasicPipeline(t *testing.T) {
	caller := &mockCaller{
		responses: map[string]any{
			"plugin-a": metadataLookupResponse{
				Results: []metadataMatch{
					{Index: 0, Title: "Donkey Kong 3", ExternalID: "dkong3", Platform: "arcade"},
					{Index: 2, Title: "Bubble Bobble", ExternalID: "bublbobl", Platform: "arcade"},
				},
			},
			"plugin-b": metadataLookupResponse{
				Results: []metadataMatch{
					{Index: 0, Title: "DK3 (alternate)", ExternalID: "99999", URL: "https://example.com/99999"},
					{Index: 1, Title: "Pac-Man", ExternalID: "pm-001", Platform: "arcade"},
				},
			},
		},
	}

	resolver := NewMetadataResolver(caller, testLogger{})

	games := []*core.Game{
		{Title: "dkong3", RawTitle: "dkong3", Platform: core.PlatformArcade, Kind: core.GameKindBaseGame},
		{Title: "pacman", RawTitle: "pacman", Platform: core.PlatformUnknown, Kind: core.GameKindBaseGame},
		{Title: "bublbobl", RawTitle: "bublbobl", Platform: core.PlatformArcade, Kind: core.GameKindBaseGame},
	}

	sources := []MetadataSource{
		{PluginID: "plugin-a", Config: map[string]any{}},
		{PluginID: "plugin-b", Config: map[string]any{}},
	}

	resolver.Enrich(context.Background(), "test-integration", games, sources)

	// Game 0: plugin-a says "Donkey Kong 3", plugin-b says "DK3 (alternate)".
	// 1 vs 1 tie → plugin-a wins by priority.
	if games[0].Title != "Donkey Kong 3" {
		t.Errorf("game 0 title: got %q, want %q", games[0].Title, "Donkey Kong 3")
	}
	if games[0].Status != "identified" {
		t.Errorf("game 0 status: got %q, want %q", games[0].Status, "identified")
	}
	// plugin-a match kept, plugin-b outvoted → 1 external ID from phase 1.
	// Phase 3 will re-query plugin-b with "Donkey Kong 3".
	// The static mock will return the same "DK3 (alternate)" response,
	// but game 0 raw title is "dkong3" and phase 3 sends "Donkey Kong 3".
	// The mock doesn't distinguish → it adds a fill match.
	// We primarily verify the structure is correct.
	if len(games[0].ResolverMatches) < 2 {
		t.Errorf("game 0 resolver matches: got %d, want at least 2", len(games[0].ResolverMatches))
	}

	// Game 1: only matched by plugin-b → sets title and platform.
	if games[1].Title != "Pac-Man" {
		t.Errorf("game 1 title: got %q, want %q", games[1].Title, "Pac-Man")
	}
	if games[1].Platform != core.PlatformArcade {
		t.Errorf("game 1 platform: got %q, want %q", games[1].Platform, core.PlatformArcade)
	}

	// Game 2: only matched by plugin-a.
	if games[2].Title != "Bubble Bobble" {
		t.Errorf("game 2 title: got %q, want %q", games[2].Title, "Bubble Bobble")
	}
}

func TestEnrich_Unidentified(t *testing.T) {
	caller := &mockCaller{
		responses: map[string]any{
			"plugin-a": metadataLookupResponse{Results: []metadataMatch{}},
		},
	}

	resolver := NewMetadataResolver(caller, testLogger{})

	games := []*core.Game{
		{Title: "unknown_file", RawTitle: "unknown_file", Platform: core.PlatformUnknown, Kind: core.GameKindBaseGame},
	}

	sources := []MetadataSource{{PluginID: "plugin-a", Config: map[string]any{}}}
	resolver.Enrich(context.Background(), "test-integration", games, sources)

	if games[0].Status != "unidentified" {
		t.Errorf("status: got %q, want %q", games[0].Status, "unidentified")
	}
	if games[0].Title != "unknown_file" {
		t.Errorf("title should remain unchanged: got %q", games[0].Title)
	}
	if len(games[0].ExternalIDs) != 0 {
		t.Errorf("external IDs: got %d, want 0", len(games[0].ExternalIDs))
	}
}

func TestEnrich_FillPhase(t *testing.T) {
	callCount := map[string]int{}

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			callCount[pluginID]++

			b, _ := json.Marshal(params)
			var p struct {
				Games []metadataGameQuery `json:"games"`
			}
			json.Unmarshal(b, &p)

			switch pluginID {
			case "plugin-a":
				// Plugin A always finds the game in both phases.
				return metadataLookupResponse{
					Results: []metadataMatch{
						{Index: 0, Title: "Half-Life 2", ExternalID: "igdb-220"},
					},
				}, nil
			case "plugin-b":
				// Plugin B only finds the game when given the canonical title.
				if len(p.Games) > 0 && p.Games[0].Title == "Half-Life 2" {
					return metadataLookupResponse{
						Results: []metadataMatch{
							{Index: 0, Title: "Half-Life 2", ExternalID: "steam-220", URL: "https://store.steampowered.com/app/220"},
						},
					}, nil
				}
				return metadataLookupResponse{Results: []metadataMatch{}}, nil
			}
			return nil, nil
		},
	}

	resolver := NewMetadataResolver(caller, testLogger{})

	games := []*core.Game{
		{Title: "hl2_setup", RawTitle: "hl2_setup", Platform: core.PlatformWindowsPC, Kind: core.GameKindBaseGame},
	}

	sources := []MetadataSource{
		{PluginID: "plugin-a", Config: map[string]any{}},
		{PluginID: "plugin-b", Config: map[string]any{}},
	}

	resolver.Enrich(context.Background(), "test-integration", games, sources)

	if games[0].Title != "Half-Life 2" {
		t.Errorf("title: got %q, want %q", games[0].Title, "Half-Life 2")
	}
	if games[0].Status != "identified" {
		t.Errorf("status: got %q, want %q", games[0].Status, "identified")
	}

	// Should have 2 external IDs: one from phase 1 (plugin-a) and one from phase 3 (plugin-b).
	if len(games[0].ExternalIDs) != 2 {
		t.Fatalf("external IDs: got %d, want 2", len(games[0].ExternalIDs))
	}
	if games[0].ExternalIDs[0].Source != "plugin-a" {
		t.Errorf("ext[0] source: got %q, want %q", games[0].ExternalIDs[0].Source, "plugin-a")
	}
	if games[0].ExternalIDs[1].Source != "plugin-b" {
		t.Errorf("ext[1] source: got %q, want %q", games[0].ExternalIDs[1].Source, "plugin-b")
	}
	if games[0].ExternalIDs[1].URL != "https://store.steampowered.com/app/220" {
		t.Errorf("ext[1] URL: got %q", games[0].ExternalIDs[1].URL)
	}

	// Plugin A: called in phase 1 (raw title) only — it already matched.
	if callCount["plugin-a"] != 1 {
		t.Errorf("plugin-a call count: got %d, want 1", callCount["plugin-a"])
	}
	// Plugin B: called in phase 1 (miss) + phase 3 (fill with canonical title).
	if callCount["plugin-b"] != 2 {
		t.Errorf("plugin-b call count: got %d, want 2", callCount["plugin-b"])
	}
}

func TestEnrich_OutvotedResolverRequeried(t *testing.T) {
	callCount := map[string]int{}

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			callCount[pluginID]++

			b, _ := json.Marshal(params)
			var p struct {
				Games []metadataGameQuery `json:"games"`
			}
			json.Unmarshal(b, &p)

			switch pluginID {
			case "plugin-a":
				return metadataLookupResponse{
					Results: []metadataMatch{
						{Index: 0, Title: "Doom", ExternalID: "a1"},
					},
				}, nil
			case "plugin-b":
				return metadataLookupResponse{
					Results: []metadataMatch{
						{Index: 0, Title: "Doom", ExternalID: "b1"},
					},
				}, nil
			case "plugin-c":
				// Phase 1: returns wrong title. Phase 3: corrects itself.
				if len(p.Games) > 0 && p.Games[0].Title == "Doom" {
					return metadataLookupResponse{
						Results: []metadataMatch{
							{Index: 0, Title: "DOOM", ExternalID: "c1-corrected"},
						},
					}, nil
				}
				return metadataLookupResponse{
					Results: []metadataMatch{
						{Index: 0, Title: "Doom Eternal", ExternalID: "c1-wrong"},
					},
				}, nil
			}
			return nil, nil
		},
	}

	resolver := NewMetadataResolver(caller, testLogger{})

	games := []*core.Game{
		{Title: "doom", RawTitle: "doom", Platform: core.PlatformWindowsPC, Kind: core.GameKindBaseGame},
	}

	sources := []MetadataSource{
		{PluginID: "plugin-a", Config: map[string]any{}},
		{PluginID: "plugin-b", Config: map[string]any{}},
		{PluginID: "plugin-c", Config: map[string]any{}},
	}

	resolver.Enrich(context.Background(), "test-integration", games, sources)

	if games[0].Title != "Doom" {
		t.Errorf("title: got %q, want %q", games[0].Title, "Doom")
	}

	// plugin-c was outvoted (said "Doom Eternal"), then re-queried in fill with "Doom".
	if callCount["plugin-c"] != 2 {
		t.Errorf("plugin-c call count: got %d, want 2 (phase 1 + phase 3 fill)", callCount["plugin-c"])
	}

	// Should have external IDs from a, b, and c's corrected fill response.
	hasCorrectC := false
	for _, eid := range games[0].ExternalIDs {
		if eid.Source == "plugin-c" && eid.ExternalID == "c1-corrected" {
			hasCorrectC = true
		}
		if eid.Source == "plugin-c" && eid.ExternalID == "c1-wrong" {
			t.Error("outvoted external ID 'c1-wrong' should NOT appear in ExternalIDs")
		}
	}
	if !hasCorrectC {
		t.Error("expected corrected external ID 'c1-corrected' from plugin-c fill phase")
	}
}

func TestEnrich_SkippedIndex(t *testing.T) {
	caller := &mockCaller{
		responses: map[string]any{
			"plugin-a": metadataLookupResponse{
				Results: []metadataMatch{
					{Index: 0, Title: "Matched", ExternalID: "x1"},
					{Index: 2, Title: "Also Matched", ExternalID: "x3"},
				},
			},
		},
	}

	resolver := NewMetadataResolver(caller, testLogger{})

	games := []*core.Game{
		{Title: "raw0", RawTitle: "raw0", Platform: core.PlatformUnknown, Kind: core.GameKindBaseGame},
		{Title: "raw1", RawTitle: "raw1", Platform: core.PlatformUnknown, Kind: core.GameKindBaseGame},
		{Title: "raw2", RawTitle: "raw2", Platform: core.PlatformUnknown, Kind: core.GameKindBaseGame},
	}

	sources := []MetadataSource{{PluginID: "plugin-a", Config: map[string]any{}}}
	resolver.Enrich(context.Background(), "test-integration", games, sources)

	if games[0].Title != "Matched" {
		t.Errorf("game 0: got %q, want %q", games[0].Title, "Matched")
	}
	if games[1].Status != "unidentified" {
		t.Errorf("game 1 status: got %q, want %q", games[1].Status, "unidentified")
	}
	if games[1].Title != "raw1" {
		t.Errorf("game 1 title should be unchanged: got %q", games[1].Title)
	}
	if games[2].Title != "Also Matched" {
		t.Errorf("game 2: got %q, want %q", games[2].Title, "Also Matched")
	}
}

func TestEnrich_EmitsMetadataIntegrationIdentity(t *testing.T) {
	caller := &mockCaller{
		responses: map[string]any{
			"metadata-steam": metadataLookupResponse{
				Results: []metadataMatch{{Index: 0, Title: "Doom", ExternalID: "steam-1"}},
			},
		},
	}

	resolver := NewMetadataResolver(caller, testLogger{})

	type emittedEvent struct {
		typ    string
		fields map[string]any
	}
	var events []emittedEvent
	resolver.SetScanEventPublisher(func(_ context.Context, typ string, payload any) {
		fields, _ := payload.(map[string]any)
		events = append(events, emittedEvent{typ: typ, fields: fields})
	})

	games := []*core.Game{
		{Title: "doom", RawTitle: "doom", Platform: core.PlatformWindowsPC, Kind: core.GameKindBaseGame},
	}
	sources := []MetadataSource{
		{
			IntegrationID: "metadata-steam-primary",
			Label:         "Steam Metadata",
			PluginID:      "metadata-steam",
			Config:        map[string]any{},
		},
	}

	resolver.Enrich(context.Background(), "source-1", games, sources)

	if len(events) == 0 {
		t.Fatal("expected metadata events")
	}

	for _, ev := range events {
		switch ev.typ {
		case "scan_metadata_plugin_started", "scan_metadata_game_progress", "scan_metadata_plugin_complete":
			if ev.fields["metadata_integration_id"] != "metadata-steam-primary" {
				t.Fatalf("%s metadata_integration_id = %v, want metadata-steam-primary", ev.typ, ev.fields["metadata_integration_id"])
			}
			if ev.fields["metadata_label"] != "Steam Metadata" {
				t.Fatalf("%s metadata_label = %v, want Steam Metadata", ev.typ, ev.fields["metadata_label"])
			}
		}
	}
}
