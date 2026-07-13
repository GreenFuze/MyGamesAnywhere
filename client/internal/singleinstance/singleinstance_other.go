//go:build !windows

package singleinstance

type Lock struct{}

func Acquire(_ string) (*Lock, error) {
	return &Lock{}, nil
}

func (l *Lock) Close() error {
	return nil
}
