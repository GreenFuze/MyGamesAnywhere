package http

import (
	"fmt"
	"net/http"

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
			_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, string(ev.Data))
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
