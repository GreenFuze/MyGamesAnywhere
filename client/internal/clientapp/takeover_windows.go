//go:build windows

package clientapp

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"golang.org/x/sys/windows"
)

func launchTakeover(elevated bool, mode devicev1.ClientExecutionMode, startURI string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find MGA Client executable for takeover: %w", err)
	}
	args := []string{"agent", "--takeover", "--mode", string(mode)}
	if startURI != "" {
		args = append(args, "--start-uri", startURI)
	}
	if !elevated {
		command := exec.Command(executable, args...)
		if err := command.Start(); err != nil {
			return fmt.Errorf("start MGA Client takeover: %w", err)
		}
		return command.Process.Release()
	}

	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	parameters := ""
	for index, arg := range args {
		if index > 0 {
			parameters += " "
		}
		parameters += syscall.EscapeArg(arg)
	}
	parameterPointer, err := windows.UTF16PtrFromString(parameters)
	if err != nil {
		return err
	}
	result, _, callErr := procElevationShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(parameterPointer)),
		0,
		1,
	)
	if result <= 32 {
		if callErr != nil && callErr != windows.ERROR_SUCCESS {
			return fmt.Errorf("request MGA Client elevation: %w", callErr)
		}
		return fmt.Errorf("request MGA Client elevation: ShellExecuteW returned %d", result)
	}
	return nil
}
