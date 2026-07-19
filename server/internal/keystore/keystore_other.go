//go:build !windows

package keystore

import (
	"os"
	"path/filepath"
	"sync"
)

// FileKeyStore stores the sync key in a file protected by OS permissions (0600).
// On non-Windows platforms this is the best we can do without external keyrings.
type FileKeyStore struct {
	dir string
	mu  sync.Mutex
}

func New() *FileKeyStore {
	return &FileKeyStore{dir: DefaultPath()}
}

func NewWithDir(dir string) *FileKeyStore {
	return &FileKeyStore{dir: dir}
}

func (k *FileKeyStore) keyPath(profileID string) (string, error) {
	return profileKeyPath(k.dir, profileID, "sync_key")
}

func (k *FileKeyStore) Store(profileID, passphrase string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	keyPath, err := k.keyPath(profileID)
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(keyPath)); err != nil {
		return err
	}
	return os.WriteFile(keyPath, []byte(passphrase), 0o600)
}

func (k *FileKeyStore) Load(profileID string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	keyPath, err := k.keyPath(profileID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNoKey
		}
		return "", err
	}
	return string(data), nil
}

func (k *FileKeyStore) Clear(profileID string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	keyPath, err := k.keyPath(profileID)
	if err != nil {
		return err
	}
	err = os.Remove(keyPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (k *FileKeyStore) HasKey(profileID string) bool {
	keyPath, err := k.keyPath(profileID)
	if err != nil {
		return false
	}
	_, err = os.Stat(keyPath)
	return err == nil
}
