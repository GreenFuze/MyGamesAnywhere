//go:build !windows

package runtime

import (
	"fmt"
	"os"
	"path/filepath"
)

func defaultDataDir(mode Mode, appDir string) (string, error) {
	switch mode {
	case ModePortable:
		return appDir, nil
	case ModeUser:
		if base := os.Getenv("XDG_DATA_HOME"); base != "" {
			return filepath.Join(base, "mga"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user data dir: %w", err)
		}
		return filepath.Join(home, ".local", "share", "mga"), nil
	case ModeMachine:
		return filepath.Join(string(filepath.Separator), "var", "lib", "mga"), nil
	default:
		return "", fmt.Errorf("unsupported runtime mode %q", mode)
	}
}
