// Package events provides an in-memory pub/sub bus for server-sent events and notifications.
package events

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

const subscriberBuffer = 64

// Event is a single message on the wire (SSE event type + JSON payload).
type Event struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	ProfileID string          `json:"-"`
	Global    bool            `json:"-"`
}

// EventBus fans out published events to subscribers. It is safe for concurrent use.
type EventBus struct {
	mu     sync.Mutex
	subs   map[chan Event]struct{}
	closed bool
}

// New returns a new EventBus.
func New() *EventBus {
	return &EventBus{
		subs: make(map[chan Event]struct{}),
	}
}

// Subscribe registers a new subscriber and returns a receive-only channel.
// If the bus is already closed, returns a closed channel.
func (b *EventBus) Subscribe() <-chan Event {
	ch := make(chan Event, subscriberBuffer)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		close(ch)
		return ch
	}
	b.subs[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a subscriber and closes its channel.
// rc must be the channel returned from Subscribe.
func (b *EventBus) Unsubscribe(rc <-chan Event) {
	ptr := reflect.ValueOf(rc).Pointer()
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		if reflect.ValueOf(ch).Pointer() == ptr {
			delete(b.subs, ch)
			close(ch)
			return
		}
	}
}

// Publish sends an event to all subscribers. Full buffers are skipped (non-blocking).
// No-op if the bus is closed.
func (b *EventBus) Publish(ev Event) {
	ev.Type = strings.TrimSpace(ev.Type)
	if ev.Type == "" {
		return
	}
	if ev.ProfileID == "" {
		ev.ProfileID = profileIDFromData(ev.Data)
	}
	if ev.ProfileID == "" {
		// Global visibility is determined by the central allowlist. A caller
		// cannot turn an arbitrary ownerless event into a broadcast by setting
		// Global, which keeps the publication boundary fail-closed.
		ev.Global = IsGlobalEventType(ev.Type)
		if !ev.Global {
			// Unknown or profile-owned events without an owner fail closed rather
			// than becoming a broadcast by omission.
			return
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
			// drop if subscriber is slow
		}
	}
}

func IsGlobalEventType(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	switch eventType {
	case "update_available",
		"update_download_started",
		"update_download_progress",
		"update_download_complete",
		"update_download_error",
		"update_apply_started",
		"update_apply_error",
		"plugin_process_exited":
		return true
	default:
		return false
	}
}

func profileIDFromData(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	value, ok := payload["profile_id"]
	if !ok {
		return ""
	}
	profileID, _ := value.(string)
	return strings.TrimSpace(profileID)
}

// Close shuts down the bus: no further publishes, all subscriber channels are closed.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for ch := range b.subs {
		close(ch)
		delete(b.subs, ch)
	}
}
