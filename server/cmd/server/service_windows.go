//go:build windows

package main

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows/svc"
)

type serviceRunner struct {
	run func(context.Context) error
}

func runWindowsService(name string, run func(context.Context) error) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("detect service session: %w", err)
	}
	if !isService {
		return run(context.Background())
	}
	return svc.Run(name, &serviceRunner{run: run})
}

func (r *serviceRunner) Execute(args []string, changes <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	status <- svc.Status{State: svc.StartPending}
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.run(ctx)
	}()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case change := <-changes:
			switch change.Cmd {
			case svc.Interrogate:
				status <- change.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				if err := <-errCh; err != nil {
					return true, 1
				}
				return false, 0
			}
		case err := <-errCh:
			if err != nil {
				return true, 1
			}
			return false, 0
		}
	}
}
