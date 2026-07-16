//go:build !windows

package clientapp

import devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"

func platformExecutionMode() devicev1.ClientExecutionMode {
	return devicev1.ClientExecutionModeStandard
}

func relaunchElevated(string) error {
	return ErrElevationUnavailable
}
