//go:build windows

package clientapp

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestStorageVolumeEligible(t *testing.T) {
	tests := []struct {
		name                    string
		driveType               uint32
		volumeIdentityAvailable bool
		deviceTarget            string
		want                    bool
	}{
		{name: "local fixed volume", driveType: windows.DRIVE_FIXED, volumeIdentityAvailable: true, deviceTarget: `\Device\HarddiskVolume3`, want: true},
		{name: "virtual fixed volume", driveType: windows.DRIVE_FIXED, volumeIdentityAvailable: true, deviceTarget: `\Device\Volume{cloud}`, want: false},
		{name: "virtual fixed mount without volume identity", driveType: windows.DRIVE_FIXED, volumeIdentityAvailable: false, deviceTarget: `\Device\HarddiskVolume3`, want: false},
		{name: "network volume", driveType: windows.DRIVE_REMOTE, volumeIdentityAvailable: true, deviceTarget: `\Device\Mup`, want: false},
		{name: "removable volume", driveType: windows.DRIVE_REMOVABLE, volumeIdentityAvailable: true, deviceTarget: `\Device\HarddiskVolume9`, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := storageVolumeEligible(test.driveType, test.volumeIdentityAvailable, test.deviceTarget); got != test.want {
				t.Fatalf("storageVolumeEligible(%d, %t, %q) = %t, want %t", test.driveType, test.volumeIdentityAvailable, test.deviceTarget, got, test.want)
			}
		})
	}
}

func TestCollectLocalStorageReturnsOnlyVolumeBackedFixedDrives(t *testing.T) {
	storage, err := collectLocalStorage()
	if err != nil {
		t.Fatal(err)
	}
	returned := make(map[string]bool, len(storage))
	for _, item := range storage {
		returned[item.Root] = true
		if !isLocalFixedVolume(item.Root) {
			t.Fatalf("collectLocalStorage returned non-local volume %q", item.Root)
		}
	}
	if isLocalFixedVolume(`C:\`) && !returned[`C:\`] {
		t.Fatal("collectLocalStorage omitted the real local C: volume")
	}
	for _, virtual := range []string{`G:\`, `H:\`, `I:\`} {
		if returned[virtual] {
			t.Fatalf("collectLocalStorage returned virtual volume %s", virtual)
		}
	}
}

func TestValidateInstallRootStorageRejectsGoogleDriveDesktopMounts(t *testing.T) {
	for _, root := range []string{`G:\Games`, `H:\Games`, `I:\Games`} {
		pointer, err := windows.UTF16PtrFromString(root[:3])
		if err != nil {
			t.Fatal(err)
		}
		if windows.GetDriveType(pointer) == windows.DRIVE_NO_ROOT_DIR {
			continue
		}
		if isLocalFixedVolume(root[:3]) {
			t.Fatalf("%s was misclassified as a real local volume", root[:3])
		}
		if err := validateInstallRootStorage(root); err == nil {
			t.Fatalf("validateInstallRootStorage(%q) accepted a virtual or non-local mount", root)
		}
	}
}
