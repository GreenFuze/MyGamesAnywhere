package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Layout struct {
	DataDir        string
	ConfigPath     string
	PrivateKeyPath string
	LogPath        string
}

func Resolve(dataDir string) (Layout, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		dataDir = strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if dataDir != "" {
			dataDir = filepath.Join(dataDir, "MyGamesAnywhere", "Client")
		} else {
			fallback, err := os.UserConfigDir()
			if err != nil {
				return Layout{}, errors.New("resolve per-user data directory")
			}
			dataDir = filepath.Join(fallback, "MyGamesAnywhere", "Client")
		}
	}
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return Layout{}, err
	}
	return Layout{
		DataDir:        absolute,
		ConfigPath:     filepath.Join(absolute, "config.json"),
		PrivateKeyPath: filepath.Join(absolute, "endpoint_key.dpapi"),
		LogPath:        filepath.Join(absolute, "mga-client.log"),
	}, nil
}

func (l Layout) Ensure() error {
	if strings.TrimSpace(l.DataDir) == "" {
		return errors.New("client data directory is required")
	}
	return os.MkdirAll(l.DataDir, 0o700)
}
