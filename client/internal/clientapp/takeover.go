package clientapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/singleinstance"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

const takeoverWaitTimeout = 30 * time.Second

// RunAgentTakeover waits for the old tray owner to exit and becomes the sole
// agent. An elevated start URI is redeemed only after the elevated successor
// owns the agent mutex.
func (s *Service) RunAgentTakeover(ctx context.Context, executionMode devicev1.ClientExecutionMode, startURI string) error {
	if executionMode == "" {
		executionMode = currentExecutionMode()
	}
	if err := executionMode.Validate(); err != nil {
		return err
	}
	if currentExecutionMode() != executionMode {
		return fmt.Errorf("takeover process is running in %s mode, expected %s", currentExecutionMode(), executionMode)
	}
	lock, err := s.waitForAgentLock(ctx)
	if err != nil {
		return err
	}
	if startURI != "" {
		action, parseErr := parseProtocolAction(startURI)
		if parseErr != nil {
			_ = lock.Close()
			return parseErr
		}
		if action.host != "start" {
			_ = lock.Close()
			return errors.New("takeover launch URI must be an MGA start request")
		}
		action.start.ExecutionMode = executionMode
		if acknowledgeErr := s.acknowledgeLaunch(ctx, action.start); acknowledgeErr != nil {
			_ = lock.Close()
			return acknowledgeErr
		}
	}
	return s.runAgentWithLock(ctx, executionMode, lock)
}

func (s *Service) waitForAgentLock(ctx context.Context) (*singleinstance.Lock, error) {
	deadline := time.NewTimer(takeoverWaitTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		lock, err := singleinstance.Acquire(s.instanceLockName())
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, singleinstance.ErrAlreadyRunning) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errors.New("the previous MGA Client did not exit for the requested takeover")
		case <-ticker.C:
		}
	}
}
