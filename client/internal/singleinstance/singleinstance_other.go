//go:build !windows

package singleinstance

import (
	"errors"
	"strings"
	"sync"
)

type Lock struct {
	name string
}

var (
	locksMu sync.Mutex
	locks   = make(map[string]bool)
)

func Acquire(name string) (*Lock, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("single-instance name is required")
	}
	locksMu.Lock()
	defer locksMu.Unlock()
	if locks[name] {
		return nil, ErrAlreadyRunning
	}
	locks[name] = true
	return &Lock{name: name}, nil
}

func (l *Lock) Close() error {
	if l == nil || l.name == "" {
		return nil
	}
	locksMu.Lock()
	delete(locks, l.name)
	locksMu.Unlock()
	l.name = ""
	return nil
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
