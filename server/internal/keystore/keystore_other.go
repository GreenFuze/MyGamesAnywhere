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

func (k *FileKeyStore) keyPath() string {
	return filepath.Join(k.dir, "sync_key")
}

func (k *FileKeyStore) Store(passphrase string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err := ensureDir(k.dir); err != nil {
		return err
	}
	return os.WriteFile(k.keyPath(), []byte(passphrase), 0o600)
}

func (k *FileKeyStore) Load() (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	data, err := os.ReadFile(k.keyPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNoKey
		}
		return "", err
	}
	return string(data), nil
}

func (k *FileKeyStore) Clear() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	err := os.Remove(k.keyPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (k *FileKeyStore) HasKey() bool {
	_, err := os.Stat(filepath.Join(k.dir, "sync_key"))
	return err == nil
}
