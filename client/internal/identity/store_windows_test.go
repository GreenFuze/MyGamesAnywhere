//go:build windows

package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"path/filepath"
	"testing"
)

func TestDPAPIStoreRoundTripForCurrentUser(t *testing.T) {
	t.Parallel()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	store := NewStore(filepath.Join(t.TempDir(), "identity.dpapi"))
	if err := store.Save(privateKey); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !bytes.Equal(loaded, privateKey) {
		t.Fatal("loaded private key does not match")
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNoPrivateKey) {
		t.Fatalf("Load() after Clear error = %v, want ErrNoPrivateKey", err)
	}
}
