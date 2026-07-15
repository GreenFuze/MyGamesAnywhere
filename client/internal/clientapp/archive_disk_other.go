//go:build !windows

package clientapp

import "errors"

func availableDiskBytes(string) (uint64, error) {
	return 0, errors.New("archive installation is currently supported only on Windows")
}
