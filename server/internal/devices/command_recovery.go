package devices

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type interruptedCommandStore interface {
	FailInterruptedCommands(context.Context, time.Time) (int64, error)
}

type CommandRecovery struct {
	store  interruptedCommandStore
	logger core.Logger
	now    func() time.Time
}

func NewCommandRecovery(store interruptedCommandStore, logger core.Logger) (*CommandRecovery, error) {
	if store == nil {
		return nil, errors.New("device command store is required")
	}
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	return &CommandRecovery{store: store, logger: logger, now: time.Now}, nil
}

func (r *CommandRecovery) Run(ctx context.Context) error {
	if r == nil || r.store == nil || r.logger == nil || r.now == nil {
		return errors.New("device command recovery is unavailable")
	}
	count, err := r.store.FailInterruptedCommands(ctx, r.now())
	if err != nil {
		return fmt.Errorf("recover interrupted device commands: %w", err)
	}
	if count > 0 {
		r.logger.Warn("recovered interrupted device commands", "count", count)
	}
	return nil
}
