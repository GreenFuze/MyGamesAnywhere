//go:build windows

package clientapp

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"golang.org/x/sys/windows"
)

var (
	elevationShell32           = windows.NewLazySystemDLL("shell32.dll")
	procElevationShellExecuteW = elevationShell32.NewProc("ShellExecuteW")
)

func platformExecutionMode() devicev1.ClientExecutionMode {
	if windows.GetCurrentProcessToken().IsElevated() {
		return devicev1.ClientExecutionModeElevated
	}
	return devicev1.ClientExecutionModeStandard
}

func relaunchElevated(uri string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find MGA Client executable for elevation: %w", err)
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	parameters, err := windows.UTF16PtrFromString("protocol " + syscall.EscapeArg(uri))
	if err != nil {
		return err
	}
	result, _, callErr := procElevationShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(parameters)),
		0,
		1,
	)
	if result <= 32 {
		if callErr != nil && callErr != windows.ERROR_SUCCESS {
			return fmt.Errorf("request MGA Client elevation: %w", callErr)
		}
		return fmt.Errorf("request MGA Client elevation: ShellExecuteW returned %d", result)
	}
	return ErrElevationRelaunched
}
