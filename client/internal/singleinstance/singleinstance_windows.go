//go:build windows

package singleinstance

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

type Lock struct {
	handle windows.Handle
}

func Acquire(name string) (*Lock, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("single-instance name is required")
	}
	namePointer, err := windows.UTF16PtrFromString("Local\\" + name)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateMutex(nil, false, namePointer)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, fmt.Errorf("create client mutex: %w", err)
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		_ = windows.CloseHandle(handle)
		return nil, ErrAlreadyRunning
	}
	return &Lock{handle: handle}, nil
}

func IsRunning(name string) (bool, error) {
	lock, err := Acquire(name)
	if err == nil {
		return false, lock.Close()
	}
	if errors.Is(err, ErrAlreadyRunning) {
		return true, nil
	}
	return false, err
}

func (l *Lock) Close() error {
	if l == nil || l.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(l.handle)
	l.handle = 0
	return err
}
