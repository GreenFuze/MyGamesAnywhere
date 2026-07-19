package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventBusFailsClosedForOwnerlessProfileEvent(t *testing.T) {
	bus := New()
	sub := bus.Subscribe()
	defer bus.Unsubscribe(sub)

	bus.Publish(Event{Type: "scan_started", Data: json.RawMessage(`{"job_id":"foreign"}`)})
	select {
	case event := <-sub:
		t.Fatalf("ownerless profile event was broadcast: %+v", event)
	case <-time.After(25 * time.Millisecond):
	}
}

func TestEventBusAcceptsOwnedAndAllowlistedGlobalEventsOnly(t *testing.T) {
	bus := New()
	sub := bus.Subscribe()
	defer bus.Unsubscribe(sub)

	bus.Publish(Event{Type: "connection_updated", Data: json.RawMessage(`{"profile_id":"profile-a","integration_id":"a"}`)})
	bus.Publish(Event{Type: "update_available", Data: json.RawMessage(`{"version":"9.9.9"}`)})
	bus.Publish(Event{Type: "not_really_global", Global: true, Data: json.RawMessage(`{}`)})
	bus.Publish(Event{Type: "update_profile_secret", Data: json.RawMessage(`{"profile":"foreign"}`)})

	first := <-sub
	if first.ProfileID != "profile-a" || first.Global {
		t.Fatalf("owned event classification = %+v", first)
	}
	second := <-sub
	if second.Type != "update_available" || !second.Global || second.ProfileID != "" {
		t.Fatalf("global event classification = %+v", second)
	}
	select {
	case event := <-sub:
		t.Fatalf("non-allowlisted event was broadcast: %+v", event)
	case <-time.After(25 * time.Millisecond):
	}
}
