//go:build windows

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
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			return filepath.Join(base, "MyGamesAnywhere"), nil
		}
		return "", fmt.Errorf("LOCALAPPDATA is required for %s runtime mode", mode)
	case ModeMachine:
		if base := os.Getenv("PROGRAMDATA"); base != "" {
			return filepath.Join(base, "MyGamesAnywhere"), nil
		}
		return "", fmt.Errorf("PROGRAMDATA is required for %s runtime mode", mode)
	default:
		return "", fmt.Errorf("unsupported runtime mode %q", mode)
	}
}
