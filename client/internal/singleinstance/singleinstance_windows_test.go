//go:build windows

package singleinstance

import (
	"errors"
	"testing"
)

func TestAcquireClassifiesExistingWindowsMutex(t *testing.T) {
	name := "MGAClient-Test-" + t.Name()
	first, err := Acquire(name)
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer first.Close()

	if _, err = Acquire(name); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Acquire() error = %v, want ErrAlreadyRunning", err)
	}
	running, err := IsRunning(name)
	if err != nil {
		t.Fatalf("IsRunning() error = %v", err)
	}
	if !running {
		t.Fatal("IsRunning() = false, want true")
	}
}
