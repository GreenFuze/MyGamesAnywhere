//go:build windows

package clientapp

import (
	"fmt"
	"strings"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"golang.org/x/sys/windows"
)

func collectLocalStorage() ([]devicev1.StorageInventory, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("list storage volumes: %w", err)
	}
	storage := make([]devicev1.StorageInventory, 0, 4)
	for index := uint32(0); index < 26; index++ {
		if mask&(1<<index) == 0 {
			continue
		}
		root := fmt.Sprintf("%c:\\", 'A'+index)
		if !isLocalFixedVolume(root) {
			continue
		}
		rootUTF16, err := windows.UTF16PtrFromString(root)
		if err != nil {
			return nil, fmt.Errorf("encode storage root %s: %w", root, err)
		}
		var available, total, free uint64
		if err := windows.GetDiskFreeSpaceEx(rootUTF16, &available, &total, &free); err != nil || total == 0 {
			continue
		}
		storage = append(storage, devicev1.StorageInventory{
			ID:         strings.ToLower(strings.TrimSuffix(root, `\`)),
			Root:       root,
			TotalBytes: total,
			FreeBytes:  available,
		})
	}
	return storage, nil
}

func isLocalFixedVolume(root string) bool {
	root = strings.TrimSpace(root)
	rootPointer, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return false
	}
	volumeName := make([]uint16, windows.MAX_PATH+1)
	volumeIdentityAvailable := windows.GetVolumeNameForVolumeMountPoint(rootPointer, &volumeName[0], uint32(len(volumeName))) == nil && volumeName[0] != 0
	deviceName := strings.TrimRight(root, `\`)
	devicePointer, err := windows.UTF16PtrFromString(deviceName)
	if err != nil {
		return false
	}
	target := make([]uint16, 4096)
	_, err = windows.QueryDosDevice(devicePointer, &target[0], uint32(len(target)))
	if err != nil {
		return false
	}
	return storageVolumeEligible(windows.GetDriveType(rootPointer), volumeIdentityAvailable, windows.UTF16ToString(target))
}

func storageVolumeEligible(driveType uint32, volumeIdentityAvailable bool, deviceTarget string) bool {
	return driveType == windows.DRIVE_FIXED && volumeIdentityAvailable && strings.HasPrefix(strings.ToLower(deviceTarget), `\device\harddiskvolume`)
}
