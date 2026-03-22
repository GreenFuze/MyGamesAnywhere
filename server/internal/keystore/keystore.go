package keystore

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

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
