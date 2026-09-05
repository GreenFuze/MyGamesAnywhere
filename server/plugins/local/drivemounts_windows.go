//go:build windows

package main

import (
	"golang.org/x/sys/windows"
)

// googleDriveMounts finds Drive for Desktop by reading each volume's label,
// which is where Windows records the signed-in account: a drive mounted for
// someone@example.com is labelled "someone@example.com - Google Drive".
//
// Enumerating letters and asking each one is deliberate. The alternative is
// reading Drive FS's own configuration under %LOCALAPPDATA%, which knows the
// accounts but not reliably which letter each was given, and which changes
// shape between client versions. The volume label is what the user already
// sees in Explorer, so it is also the answer they will recognise.
func googleDriveMounts() []driveMount {
	mounts := make([]driveMount, 0, 4)
	for letter := 'A'; letter <= 'Z'; letter++ {
		root := string(letter) + `:\`
		rootPtr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		var label [windows.MAX_PATH + 1]uint16
		// A drive letter with no volume behind it fails here, which is the
		// cheapest way to skip empty slots.
		if err := windows.GetVolumeInformation(rootPtr, &label[0], uint32(len(label)), nil, nil, nil, nil, 0); err != nil {
			continue
		}
		volumeLabel := windows.UTF16ToString(label[:])
		account, ok := googleDriveAccount(volumeLabel)
		if !ok {
			continue
		}
		path := preferMyDrive(string(letter) + ":/")
		mounts = append(mounts, driveMount{
			Path:    path,
			Account: account,
			Label:   describeMount(account, path),
		})
	}
	return mounts
}
