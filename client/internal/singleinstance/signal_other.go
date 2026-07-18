//go:build !windows

package singleinstance

import "context"

type Signal struct{ channel chan struct{} }

func OpenSignal(_ string) (*Signal, error) { return &Signal{channel: make(chan struct{}, 1)}, nil }
func (s *Signal) Notify() error {
	select {
	case s.channel <- struct{}{}:
	default:
	}
	return nil
}
func (s *Signal) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.channel:
		return nil
	}
}
func (s *Signal) Close() error { return nil }
