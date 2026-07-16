package clientapp

import (
	"errors"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

// ErrElevationRelaunched is returned only by the unelevated protocol handler
// after Windows accepted a runas relaunch. The parent must exit quietly so the
// elevated child can redeem the single-use launch challenge.
var ErrElevationRelaunched = errors.New("MGA Client relaunched elevated")

// ErrElevationUnavailable means MGA cannot honor an explicit elevated launch.
// It must not silently fall back to a standard client.
var ErrElevationUnavailable = errors.New("MGA Client elevation is unavailable on this platform")

func currentExecutionMode() devicev1.ClientExecutionMode {
	return platformExecutionMode()
}
