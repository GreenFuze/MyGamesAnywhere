package events

import (
	"encoding/json"
	"time"
)

// PublishJSON publishes one SSE event with a JSON object payload and a UTC RFC3339Nano "ts" field.
func PublishJSON(bus *EventBus, typ string, payload map[string]any) {
	if bus == nil || payload == nil {
		return
	}
	m := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		m[k] = v
	}
	m["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	bus.Publish(Event{Type: typ, Data: data})
}
