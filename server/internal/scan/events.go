package scan

import "context"

// ScanEventFunc publishes a named scan/SSE event with a JSON-serializable payload.
// Implementations should treat nil as no-op. Used to avoid importing internal/events from metadata.
type ScanEventFunc func(ctx context.Context, eventType string, payload any)

type scanContextKey string

const (
	scanJobIDContextKey      scanContextKey = "scan.job_id"
	scanTriggerContextKey    scanContextKey = "scan.trigger"
	scanSourceOnlyContextKey scanContextKey = "scan.source_only"
)

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

func WithScanTrigger(ctx context.Context, trigger string) context.Context {
	if trigger == "" {
		return ctx
	}
	return context.WithValue(ctx, scanTriggerContextKey, trigger)
}

func ScanTriggerFromContext(ctx context.Context) string {
	if ctx == nil {
		return "manual"
	}
	trigger, _ := ctx.Value(scanTriggerContextKey).(string)
	if trigger == "" {
		return "manual"
	}
	return trigger
}

func WithSourceOnlyScan(ctx context.Context) context.Context {
	return context.WithValue(ctx, scanSourceOnlyContextKey, true)
}

func SourceOnlyScanFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	sourceOnly, _ := ctx.Value(scanSourceOnlyContextKey).(bool)
	return sourceOnly
}
