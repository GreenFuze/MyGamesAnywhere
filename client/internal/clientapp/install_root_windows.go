//go:build windows

package clientapp

import (
	"errors"
	"path/filepath"
	"strings"
)

func validateInstallRootStorage(path string) error {
	volume := filepath.VolumeName(filepath.Clean(path))
	if len(volume) != 2 || volume[1] != ':' {
		return errors.New("install folder must be on a local Windows drive")
	}
	if !isLocalFixedVolume(strings.ToUpper(volume) + `\`) {
		return errors.New("install folder must be on a real local fixed drive; network, cloud, removable, and virtual drives are not supported")
	}
	return nil
}
