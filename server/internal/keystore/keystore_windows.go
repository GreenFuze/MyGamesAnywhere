//go:build windows

package keystore

import (
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

func newDataBlob(d []byte) *dataBlob {
	if len(d) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{cbData: uint32(len(d)), pbData: &d[0]}
}

// DPAPIKeyStore uses Windows DPAPI to protect the sync encryption key.
// Only the same Windows user on the same machine can decrypt it.
type DPAPIKeyStore struct {
	dir  string
	mu   sync.Mutex
}

func New() *DPAPIKeyStore {
	return &DPAPIKeyStore{dir: DefaultPath()}
}

func NewWithDir(dir string) *DPAPIKeyStore {
	return &DPAPIKeyStore{dir: dir}
}

func (k *DPAPIKeyStore) keyPath() string {
	return filepath.Join(k.dir, "sync_key.enc")
}

func (k *DPAPIKeyStore) Store(passphrase string) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	encrypted, err := dpProtect([]byte(passphrase))
	if err != nil {
		return fmt.Errorf("DPAPI encrypt: %w", err)
	}
	if err := ensureDir(k.dir); err != nil {
		return err
	}
	return os.WriteFile(k.keyPath(), encrypted, 0o600)
}

func (k *DPAPIKeyStore) Load() (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	data, err := os.ReadFile(k.keyPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNoKey
		}
		return "", err
	}
	plain, err := dpUnprotect(data)
	if err != nil {
		return "", fmt.Errorf("DPAPI decrypt: %w", err)
	}
	return string(plain), nil
}

func (k *DPAPIKeyStore) Clear() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	err := os.Remove(k.keyPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (k *DPAPIKeyStore) HasKey() bool {
	_, err := os.Stat(filepath.Join(k.dir, "sync_key.enc"))
	return err == nil
}

func dpProtect(plaintext []byte) ([]byte, error) {
	inBlob := newDataBlob(plaintext)
	var outBlob dataBlob

	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(inBlob)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outBlob.pbData)))

	out := make([]byte, outBlob.cbData)
	copy(out, unsafe.Slice(outBlob.pbData, outBlob.cbData))
	return out, nil
}

func dpUnprotect(ciphertext []byte) ([]byte, error) {
	inBlob := newDataBlob(ciphertext)
	var outBlob dataBlob

	r, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(inBlob)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outBlob.pbData)))

	out := make([]byte, outBlob.cbData)
	copy(out, unsafe.Slice(outBlob.pbData, outBlob.cbData))
	return out, nil
}
