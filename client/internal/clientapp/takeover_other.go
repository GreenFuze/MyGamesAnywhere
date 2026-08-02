//go:build !windows

package clientapp

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func launchTakeover(elevated bool, mode devicev1.ClientExecutionMode, startURI string) error {
	if elevated {
		return errors.New("elevated MGA Client takeover is only supported on Windows")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find MGA Client executable for takeover: %w", err)
	}
	args := []string{"agent", "--takeover", "--mode", string(mode)}
	if startURI != "" {
		args = append(args, "--start-uri", startURI)
	}
	command := exec.Command(executable, args...)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start MGA Client takeover: %w", err)
	}
	return command.Process.Release()
}
