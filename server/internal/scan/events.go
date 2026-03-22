package scan

// ScanEventFunc publishes a named scan/SSE event with a JSON-serializable payload.
// Implementations should treat nil as no-op. Used to avoid importing internal/events from metadata.
type ScanEventFunc func(eventType string, payload any)
