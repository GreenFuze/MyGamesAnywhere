//go:build windows

package singleinstance

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSignalCrossHandleNotification(t *testing.T) {
	name := fmt.Sprintf("MGAClientTest-%d", time.Now().UnixNano())
	waiter, err := OpenSignal(name)
	if err != nil {
		t.Fatal(err)
	}
	defer waiter.Close()
	notifier, err := OpenSignal(name)
	if err != nil {
		t.Fatal(err)
	}
	defer notifier.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- waiter.Wait(ctx) }()
	if err := notifier.Notify(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}
