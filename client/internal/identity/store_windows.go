//go:build windows

package identity

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

var (
	crypt32                = syscall.NewLazyDLL("crypt32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree          = kernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

type DPAPIStore struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) Store {
	return &DPAPIStore{path: path}
}

func (s *DPAPIStore) Save(privateKey ed25519.PrivateKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(privateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid Ed25519 private key length %d", len(privateKey))
	}
	protected, err := dpProtect(privateKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, protected, 0o600)
}

func (s *DPAPIStore) Load() (ed25519.PrivateKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	protected, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoPrivateKey
		}
		return nil, err
	}
	plain, err := dpUnprotect(protected)
	if err != nil {
		return nil, err
	}
	if len(plain) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid stored Ed25519 private key length %d", len(plain))
	}
	return ed25519.PrivateKey(plain), nil
}

func (s *DPAPIStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func newDataBlob(data []byte) *dataBlob {
	if len(data) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
}

func dpProtect(plaintext []byte) ([]byte, error) {
	inBlob := newDataBlob(plaintext)
	var outBlob dataBlob
	result, _, callErr := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(inBlob)), 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if result == 0 {
		return nil, fmt.Errorf("CryptProtectData: %w", callErr)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outBlob.pbData)))
	output := make([]byte, outBlob.cbData)
	copy(output, unsafe.Slice(outBlob.pbData, outBlob.cbData))
	return output, nil
}

func dpUnprotect(ciphertext []byte) ([]byte, error) {
	inBlob := newDataBlob(ciphertext)
	var outBlob dataBlob
	result, _, callErr := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(inBlob)), 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if result == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", callErr)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outBlob.pbData)))
	output := make([]byte, outBlob.cbData)
	copy(output, unsafe.Slice(outBlob.pbData, outBlob.cbData))
	return output, nil
}
