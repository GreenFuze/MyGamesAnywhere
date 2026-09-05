package main

import (
	"os"
	"path/filepath"
	"strings"
)

// A Google Drive for Desktop mount is an ordinary folder as far as this plugin
// is concerned: the operating system has already done the syncing. What the
// user needs from us is to find it and say whose account it is, because more
// than one signed-in account is normal — three are mounted on the machine this
// was written against.
//
// Detection never fails the caller. Returning nothing is a legitimate answer,
// and on Linux it is the expected one: Google ships no Drive client there, so
// the console offers mount instructions instead.

type driveMount struct {
	// Path is the folder to offer as the connection's base, in forward-slash
	// form to match everything else the browse API returns.
	Path string
	// Account is the Google address this mount belongs to, when the platform
	// tells us. Empty is fine; the path is still usable.
	Account string
	// Label is what the user sees. It names the account when we know it,
	// because "G:/My Drive" alone does not say which of three accounts it is.
	Label string
}

const googleDriveSuffix = "Google Drive"

// googleDriveAccount reads a Windows volume label of the form
// "someone@example.com - Google Drive".
//
// Windows caps a volume label at 32 characters, so a long address arrives
// truncated: "green.fuzer@gmail.com - Googl..." is what the machine this was
// written against actually reports. Matching the whole suffix would therefore
// miss exactly the accounts most likely to be present, so the check is
// deliberately tolerant of a cut-off tail while still requiring the separator
// and something that looks like an address in front of it.
func googleDriveAccount(volumeLabel string) (string, bool) {
	label := strings.TrimSpace(volumeLabel)
	if label == "" {
		return "", false
	}

	separator := strings.LastIndex(label, " - ")
	if separator < 0 {
		// A drive whose whole label is "Google Drive" is still a Drive mount,
		// just one that did not name an account.
		if strings.EqualFold(label, googleDriveSuffix) {
			return "", true
		}
		return "", false
	}

	account := strings.TrimSpace(label[:separator])
	tail := strings.TrimSpace(label[separator+3:])
	if !looksLikeGoogleDriveTail(tail) {
		return "", false
	}
	if !strings.Contains(account, "@") {
		// The separator matched something else; without an address in front of
		// it this is not a Drive mount we can attribute.
		return "", false
	}
	return account, true
}

// looksLikeGoogleDriveTail accepts the full suffix and any leading run of it,
// which is what truncation leaves behind.
func looksLikeGoogleDriveTail(tail string) bool {
	trimmed := strings.TrimRight(tail, ".")
	if trimmed == "" {
		return false
	}
	if len(trimmed) > len(googleDriveSuffix) {
		return false
	}
	// Two characters is enough because the prefix has to match: real truncation
	// produced "Go..." on this machine, while "Gg" is not a prefix of "Google
	// Drive" and is rejected on that basis rather than on length.
	return len(trimmed) >= 2 && strings.EqualFold(trimmed, googleDriveSuffix[:len(trimmed)])
}

// preferMyDrive points the connection at the personal area when it is there.
// Drive for Desktop puts everything under "My Drive", so offering the bare
// drive letter would cost the user a click and invite them to pick a root that
// also contains "Shared drives".
func preferMyDrive(root string) string {
	candidate := filepath.Join(filepath.FromSlash(root), "My Drive")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return filepath.ToSlash(candidate)
	}
	return root
}

// describeMount names the mount for a person choosing between several.
func describeMount(account, path string) string {
	if account == "" {
		return "Google Drive (" + path + ")"
	}
	return account
}
