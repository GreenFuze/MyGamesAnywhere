//go:build windows

package clientapp

import (
	"fmt"
	"syscall"
	"unsafe"
)

var archiveKernel32 = syscall.NewLazyDLL("kernel32.dll")
var getArchiveDiskFreeSpaceExW = archiveKernel32.NewProc("GetDiskFreeSpaceExW")

func availableDiskBytes(path string) (uint64, error) {
	pathPointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free uint64
	result, _, callErr := getArchiveDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(pathPointer)), uintptr(unsafe.Pointer(&free)), 0, 0)
	if result == 0 {
		return 0, fmt.Errorf("GetDiskFreeSpaceExW: %w", callErr)
	}
	return free, nil
}
