//go:build windows

package singleinstance

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// Signal is an auto-reset, per-user process-control event shared by all MGA
// Client processes for the same data directory.
type Signal struct {
	handle windows.Handle
}

func OpenSignal(name string) (*Signal, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("single-instance signal name is required")
	}
	pointer, err := windows.UTF16PtrFromString("Local\\" + name)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateEvent(nil, 0, 0, pointer)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	return &Signal{handle: handle}, nil
}

func (s *Signal) Notify() error {
	if s == nil || s.handle == 0 {
		return errors.New("single-instance signal is closed")
	}
	return windows.SetEvent(s.handle)
}

func (s *Signal) Wait(ctx context.Context) error {
	if s == nil || s.handle == 0 {
		return errors.New("single-instance signal is closed")
	}
	for {
		result, err := windows.WaitForSingleObject(s.handle, 250)
		if err != nil {
			return err
		}
		switch result {
		case windows.WAIT_OBJECT_0:
			return nil
		case uint32(windows.WAIT_TIMEOUT):
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		default:
			return errors.New("wait for single-instance signal failed")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *Signal) Close() error {
	if s == nil || s.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(s.handle)
	s.handle = 0
	return err
}
