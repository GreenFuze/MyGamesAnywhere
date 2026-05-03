//go:build !windows

package main

import (
	"context"
	"errors"
)

func runWindowsService(name string, run func(context.Context) error) error {
	_ = name
	_ = run
	return errors.New("--service is currently supported only on Windows")
}
