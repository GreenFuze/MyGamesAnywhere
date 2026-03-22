// Package events provides an in-memory pub/sub bus for server-sent events and notifications.
package events

import (
	"encoding/json"
	"reflect"
	"sync"
)

const subscriberBuffer = 64

// Event is a single message on the wire (SSE event type + JSON payload).
type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
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
