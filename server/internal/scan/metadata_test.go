package scan

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// mockCaller simulates plugin IPC for testing.
type mockCaller struct {
	responses map[string]any // pluginID → response object
}

func (m *mockCaller) Call(_ context.Context, pluginID string, method string, params any, result any) error {
	resp, ok := m.responses[pluginID]
	if !ok {
		return nil
	}
	b, _ := json.Marshal(resp)
	return json.Unmarshal(b, result)
}

type testLogger struct{}

func (l testLogger) Info(msg string, args ...any)            {}
func (l testLogger) Error(msg string, err error, args ...any) {}
func (l testLogger) Debug(msg string, args ...any)            {}
func (l testLogger) Warn(msg string, args ...any)             {}

func TestMetadataResolver_FirstMatchSetsTitle(t *testing.T) {
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
		{Title: "dkong3", Platform: core.PlatformArcade, Kind: core.GameKindBaseGame},
		{Title: "pacman", Platform: core.PlatformUnknown, Kind: core.GameKindBaseGame},
		{Title: "bublbobl", Platform: core.PlatformArcade, Kind: core.GameKindBaseGame},
	}

	sources := []MetadataSource{
		{PluginID: "plugin-a", Config: map[string]any{}},
		{PluginID: "plugin-b", Config: map[string]any{}},
	}

	resolver.Enrich(context.Background(), games, sources)

	// Game 0: first match from plugin-a sets title, plugin-b adds ExternalID but doesn't override title.
	if games[0].Title != "Donkey Kong 3" {
		t.Errorf("game 0 title: got %q, want %q", games[0].Title, "Donkey Kong 3")
	}
	if len(games[0].ExternalIDs) != 2 {
		t.Fatalf("game 0 external IDs: got %d, want 2", len(games[0].ExternalIDs))
	}
	if games[0].ExternalIDs[0].Source != "plugin-a" {
		t.Errorf("game 0 ext[0] source: got %q, want %q", games[0].ExternalIDs[0].Source, "plugin-a")
	}
	if games[0].ExternalIDs[1].Source != "plugin-b" {
		t.Errorf("game 0 ext[1] source: got %q, want %q", games[0].ExternalIDs[1].Source, "plugin-b")
	}
	if games[0].ExternalIDs[1].URL != "https://example.com/99999" {
		t.Errorf("game 0 ext[1] URL: got %q", games[0].ExternalIDs[1].URL)
	}

	// Game 1: only matched by plugin-b, which sets title and platform.
	if games[1].Title != "Pac-Man" {
		t.Errorf("game 1 title: got %q, want %q", games[1].Title, "Pac-Man")
	}
	if games[1].Platform != core.PlatformArcade {
		t.Errorf("game 1 platform: got %q, want %q", games[1].Platform, core.PlatformArcade)
	}
	if len(games[1].ExternalIDs) != 1 {
		t.Fatalf("game 1 external IDs: got %d, want 1", len(games[1].ExternalIDs))
	}

	// Game 2: matched only by plugin-a.
	if games[2].Title != "Bubble Bobble" {
		t.Errorf("game 2 title: got %q, want %q", games[2].Title, "Bubble Bobble")
	}
	if len(games[2].ExternalIDs) != 1 {
		t.Fatalf("game 2 external IDs: got %d, want 1", len(games[2].ExternalIDs))
	}
}

func TestMetadataResolver_FillBlanks(t *testing.T) {
	caller := &mockCaller{
		responses: map[string]any{
			"plugin-a": metadataLookupResponse{
				Results: []metadataMatch{
					{Index: 0, Title: "Game A", ExternalID: "a1"},
				},
			},
			"plugin-b": metadataLookupResponse{
				Results: []metadataMatch{
					{Index: 0, Title: "Game A Override", Platform: "ps1", Kind: "dlc", ParentGameID: "parent-1", ExternalID: "b1"},
				},
			},
		},
	}

	resolver := NewMetadataResolver(caller, testLogger{})

	games := []*core.Game{
		{Title: "raw-name", Platform: core.PlatformUnknown, Kind: core.GameKindBaseGame},
	}

	sources := []MetadataSource{
		{PluginID: "plugin-a", Config: map[string]any{}},
		{PluginID: "plugin-b", Config: map[string]any{}},
	}

	resolver.Enrich(context.Background(), games, sources)

	// Title set by plugin-a (first), NOT overridden by plugin-b.
	if games[0].Title != "Game A" {
		t.Errorf("title: got %q, want %q", games[0].Title, "Game A")
	}

	// Platform was unknown, filled by plugin-b.
	if games[0].Platform != core.PlatformPS1 {
		t.Errorf("platform: got %q, want %q", games[0].Platform, core.PlatformPS1)
	}

	// Kind was base_game (default), filled by plugin-b with dlc.
	if games[0].Kind != core.GameKindDLC {
		t.Errorf("kind: got %q, want %q", games[0].Kind, core.GameKindDLC)
	}

	// ParentGameID was empty, filled by plugin-b.
	if games[0].ParentGameID != "parent-1" {
		t.Errorf("parent_game_id: got %q, want %q", games[0].ParentGameID, "parent-1")
	}

	// Both external IDs present.
	if len(games[0].ExternalIDs) != 2 {
		t.Fatalf("external IDs: got %d, want 2", len(games[0].ExternalIDs))
	}
}

func TestMetadataResolver_SkippedIndex(t *testing.T) {
	caller := &mockCaller{
		responses: map[string]any{
			"plugin-a": metadataLookupResponse{
				Results: []metadataMatch{
					{Index: 0, Title: "Matched", ExternalID: "x1"},
					// Index 1 skipped — not matched.
					{Index: 2, Title: "Also Matched", ExternalID: "x3"},
				},
			},
		},
	}

	resolver := NewMetadataResolver(caller, testLogger{})

	games := []*core.Game{
		{Title: "raw0", Platform: core.PlatformUnknown, Kind: core.GameKindBaseGame},
		{Title: "raw1", Platform: core.PlatformUnknown, Kind: core.GameKindBaseGame},
		{Title: "raw2", Platform: core.PlatformUnknown, Kind: core.GameKindBaseGame},
	}

	sources := []MetadataSource{{PluginID: "plugin-a", Config: map[string]any{}}}
	resolver.Enrich(context.Background(), games, sources)

	if games[0].Title != "Matched" {
		t.Errorf("game 0: got %q, want %q", games[0].Title, "Matched")
	}
	if games[1].Title != "raw1" {
		t.Errorf("game 1: got %q, want %q (should be unchanged)", games[1].Title, "raw1")
	}
	if games[2].Title != "Also Matched" {
		t.Errorf("game 2: got %q, want %q", games[2].Title, "Also Matched")
	}

	if len(games[1].ExternalIDs) != 0 {
		t.Errorf("game 1 external IDs: got %d, want 0", len(games[1].ExternalIDs))
	}
}
