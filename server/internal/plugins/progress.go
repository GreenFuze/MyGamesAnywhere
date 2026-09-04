package plugins

import "context"

// Progress is what a plugin can report while it is still working on a call.
//
// Total is optional: a filesystem walk knows how many entries it has seen but
// not how many remain, and a provider fetch may only be able to name the step
// it is on. A report with no Total still tells an operator the difference
// between working and stuck, which is the whole point.
type Progress struct {
	Current int64  `json:"current,omitempty"`
	Total   int64  `json:"total,omitempty"`
	Unit    string `json:"unit,omitempty"`
	// Item names what is being worked on, or the step being performed.
	Item string `json:"item,omitempty"`
}

// ProgressFunc receives a plugin's progress reports. It runs on the plugin's
// read loop, so it must not block: publish and return.
type ProgressFunc func(Progress)

type progressContextKey struct{}

// WithProgress attaches a progress handler to one call.
//
// This travels in the context rather than in the Call signature deliberately.
// Progress is ambient, optional metadata for a single call, and threading it
// through PluginCaller, PluginHost and every test fake would change a contract
// that nearly all callers do not care about. A caller that wants progress opts
// in by wrapping the context it is already passing.
func WithProgress(ctx context.Context, handler ProgressFunc) context.Context {
	if handler == nil {
		return ctx
	}
	return context.WithValue(ctx, progressContextKey{}, handler)
}

// ProgressFromContext returns the handler attached to this call, if any.
func ProgressFromContext(ctx context.Context) ProgressFunc {
	handler, _ := ctx.Value(progressContextKey{}).(ProgressFunc)
	return handler
}
