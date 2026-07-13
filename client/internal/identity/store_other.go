//go:build !windows

package identity

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
)

type FileStore struct {
	path string
}

func NewStore(path string) Store {
	return &FileStore{path: path}
}

func (s *FileStore) Save(privateKey ed25519.PrivateKey) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid Ed25519 private key length %d", len(privateKey))
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, privateKey, 0o600)
}

func (s *FileStore) Load() (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoPrivateKey
		}
		return nil, err
	}
	if len(data) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid stored Ed25519 private key length %d", len(data))
	}
	return ed25519.PrivateKey(data), nil
}

func (s *FileStore) Clear() error {
	err := os.Remove(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
