package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
)

// SSEController serves GET /api/events (Server-Sent Events).
// Scan-related event names: ../events/scan_events.md
// Integrations, sync, plugin lifecycle, coarse errors: ../events/notification_events.md
type SSEController struct {
	bus    *events.EventBus
	logger core.Logger
}

// NewSSEController constructs an SSEController.
func NewSSEController(bus *events.EventBus, logger core.Logger) *SSEController {
	return &SSEController{bus: bus, logger: logger}
}

// Events streams events until the client disconnects or the event bus is closed.
func (c *SSEController) Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Flush headers so the client establishes the stream immediately.
	flusher.Flush()

	profileID := core.ProfileIDFromContext(r.Context())
	ch := c.bus.Subscribe()
	defer c.bus.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !sseEventVisibleToProfile(ev, profileID) {
				continue
			}
			_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, string(ev.Data))
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func sseEventVisibleToProfile(ev events.Event, profileID string) bool {
	eventProfileID := strings.TrimSpace(ev.ProfileID)
	if eventProfileID == "" {
		eventProfileID = sseEventProfileID(ev)
	}
	if eventProfileID == "" {
		return events.IsGlobalEventType(ev.Type)
	}
	return profileID != "" && eventProfileID == profileID
}

func sseEventProfileID(ev events.Event) string {
	if len(ev.Data) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(ev.Data, &payload); err != nil {
		return ""
	}
	value, ok := payload["profile_id"]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
