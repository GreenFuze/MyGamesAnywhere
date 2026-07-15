//go:build !windows

package desktop

import (
	"context"
	"errors"
)

// Run executes the agent without a notification-area host on non-Windows
// development platforms.
func (h *Host) Run(ctx context.Context, runner AgentRunner) error {
	if h == nil {
		return errors.New("desktop host is required")
	}
	if runner == nil {
		return errors.New("desktop agent runner is required")
	}
	return runner(ctx)
}
