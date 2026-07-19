package keystore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func profileKeyPath(dir, profileID, filename string) (string, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return "", fmt.Errorf("profile id is required")
	}
	for _, char := range profileID {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return "", fmt.Errorf("profile id contains an unsafe character")
	}
	return filepath.Join(dir, "profiles", profileID, filename), nil
}

// ErrNoKey is returned by Load when no stored key exists.
var ErrNoKey = errors.New("no stored encryption key")

// DefaultPath returns the OS-appropriate directory for storing the key file.
func DefaultPath() string {
	if runtime.GOOS == "windows" {
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(appdata, "mga")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "mga")
}

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o700)
}
