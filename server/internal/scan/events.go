package scan

import "context"

// ScanEventFunc publishes a named scan/SSE event with a JSON-serializable payload.
// Implementations should treat nil as no-op. Used to avoid importing internal/events from metadata.
type ScanEventFunc func(ctx context.Context, eventType string, payload any)

type scanContextKey string

const scanJobIDContextKey scanContextKey = "scan.job_id"

func WithScanJobID(ctx context.Context, jobID string) context.Context {
	if jobID == "" {
		return ctx
	}
	return context.WithValue(ctx, scanJobIDContextKey, jobID)
}

func ScanJobIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	jobID, ok := ctx.Value(scanJobIDContextKey).(string)
	return jobID, ok && jobID != ""
}
