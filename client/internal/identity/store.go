package identity

import (
	"crypto/ed25519"
	"errors"
)

var ErrNoPrivateKey = errors.New("endpoint private key is not available")

type Store interface {
	Save(privateKey ed25519.PrivateKey) error
	Load() (ed25519.PrivateKey, error)
	Clear() error
}
