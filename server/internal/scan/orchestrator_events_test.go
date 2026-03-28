package scan

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
)

type eventTestLogger struct{}

func (eventTestLogger) Info(string, ...any)         {}
func (eventTestLogger) Error(string, error, ...any) {}
func (eventTestLogger) Debug(string, ...any)        {}
func (eventTestLogger) Warn(string, ...any)         {}

func TestPublishEventWithContextInjectsJobID(t *testing.T) {
	bus := events.New()
	defer bus.Close()

	sub := bus.Subscribe()
	defer bus.Unsubscribe(sub)

	o := &Orchestrator{eventBus: bus, logger: eventTestLogger{}}
	o.publishEventWithContext(WithScanJobID(context.Background(), "job-123"), "scan_started", map[string]any{
		"integration_count": 2,
	})

	select {
	case ev := <-sub:
		if ev.Type != "scan_started" {
			t.Fatalf("event type = %q, want scan_started", ev.Type)
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if payload["job_id"] != "job-123" {
			t.Fatalf("job_id = %v, want job-123", payload["job_id"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}
