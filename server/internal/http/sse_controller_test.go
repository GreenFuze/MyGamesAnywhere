package http

import (
	"encoding/json"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
)

func TestSSEEventVisibleToProfileFiltersProfilePayloads(t *testing.T) {
	eventForProfileOne := testSSEEvent(t, "scan_complete", map[string]any{"profile_id": "profile-1"})
	globalEvent := testSSEEvent(t, "oauth_complete", map[string]any{"state": "oauth-state"})

	if !sseEventVisibleToProfile(eventForProfileOne, "profile-1") {
		t.Fatal("profile one event should be visible to profile one")
	}
	if sseEventVisibleToProfile(eventForProfileOne, "profile-2") {
		t.Fatal("profile one event should not be visible to profile two")
	}
	if sseEventVisibleToProfile(eventForProfileOne, "") {
		t.Fatal("profile event should not be visible to global setup stream")
	}
	if !sseEventVisibleToProfile(globalEvent, "") {
		t.Fatal("global event should be visible to setup stream")
	}
	if !sseEventVisibleToProfile(globalEvent, "profile-1") {
		t.Fatal("global event should be visible to profile stream")
	}
}

func testSSEEvent(t *testing.T, typ string, payload map[string]any) events.Event {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return events.Event{Type: typ, Data: data}
}
