//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// googleDriveMounts finds Drive for Desktop on macOS, and finds nothing on
// Linux, which is the correct answer rather than a failure: Google ships no
// Drive client for Linux. The console offers mount instructions when this
// comes back empty, so a Linux user is told what to do instead of being shown
// a picker with nothing in it.
func googleDriveMounts() []driveMount {
	mounts := make([]driveMount, 0, 2)

	// Current macOS clients mount under the user's CloudStorage folder, one
	// directory per signed-in account, with the address in the name:
	//   ~/Library/CloudStorage/GoogleDrive-someone@example.com
	if home, err := os.UserHomeDir(); err == nil {
		cloudStorage := filepath.Join(home, "Library", "CloudStorage")
		entries, err := os.ReadDir(cloudStorage)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				account, ok := strings.CutPrefix(entry.Name(), "GoogleDrive-")
				if !ok {
					continue
				}
				path := preferMyDrive(filepath.ToSlash(filepath.Join(cloudStorage, entry.Name())))
				mounts = append(mounts, driveMount{
					Path:    path,
					Account: strings.TrimSpace(account),
					Label:   describeMount(strings.TrimSpace(account), path),
				})
			}
		}
	}

	// Older clients used a single volume, which cannot tell us the account.
	legacy := "/Volumes/GoogleDrive"
	if info, err := os.Stat(legacy); err == nil && info.IsDir() {
		path := preferMyDrive(legacy)
		if !alreadyFound(mounts, path) {
			mounts = append(mounts, driveMount{Path: path, Label: describeMount("", path)})
		}
	}

	return mounts
}

func alreadyFound(mounts []driveMount, path string) bool {
	for _, mount := range mounts {
		if mount.Path == path {
			return true
		}
	}
	return false
}
