package http

import (
	"encoding/json"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
)

func TestSSEEventVisibleToProfileFiltersProfilePayloads(t *testing.T) {
	eventForProfileOne := testSSEEvent(t, "scan_complete", map[string]any{"profile_id": "profile-1"})
	foreignOAuthEvent := testSSEEvent(t, "oauth_complete", map[string]any{"state": "oauth-state"})
	globalEvent := testSSEEvent(t, "update_download_started", map[string]any{"status": "started"})

	if !sseEventVisibleToProfile(eventForProfileOne, "profile-1") {
		t.Fatal("profile one event should be visible to profile one")
	}
	if sseEventVisibleToProfile(eventForProfileOne, "profile-2") {
		t.Fatal("profile one event should not be visible to profile two")
	}
	if sseEventVisibleToProfile(eventForProfileOne, "") {
		t.Fatal("profile event should not be visible to global setup stream")
	}
	if sseEventVisibleToProfile(foreignOAuthEvent, "profile-1") {
		t.Fatal("ownerless OAuth event must fail closed")
	}
	if !sseEventVisibleToProfile(globalEvent, "") {
		t.Fatal("global event should be visible to setup stream")
	}
	if !sseEventVisibleToProfile(globalEvent, "profile-1") {
		t.Fatal("global event should be visible to profile stream")
	}
}

func TestTwoProfileSSEVisibilityMatrix(t *testing.T) {
	eventsToCheck := []events.Event{
		testSSEEvent(t, "scan_started", map[string]any{"profile_id": "profile-a", "job_id": "job-a", "title": "A only"}),
		testSSEEvent(t, "oauth_error", map[string]any{"profile_id": "profile-b", "state": "state-b", "error": "B only"}),
		testSSEEvent(t, "update_available", map[string]any{"version": "9.9.9"}),
	}
	want := map[string][]bool{
		"profile-a": {true, false, true},
		"profile-b": {false, true, true},
	}
	for profileID, expected := range want {
		for index, event := range eventsToCheck {
			if got := sseEventVisibleToProfile(event, profileID); got != expected[index] {
				t.Fatalf("profile %s event %s visibility = %v, want %v", profileID, event.Type, got, expected[index])
			}
		}
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
