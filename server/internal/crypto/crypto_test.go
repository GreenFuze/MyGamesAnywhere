package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := "user-secret-key"
	plaintext := []byte("secret credentials json")
	enc, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatal(err)
	}
	if enc == "" {
		t.Fatal("expected non-empty ciphertext")
	}
	dec, err := Decrypt(enc, key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, plaintext) {
		t.Errorf("decrypted %q, want %q", dec, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	plaintext := []byte("secret")
	enc, err := Encrypt(plaintext, "key1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Decrypt(enc, "key2")
	if err == nil {
		t.Error("expected decryption error with wrong key")
	}
}

func TestEncryptEmptyKey(t *testing.T) {
	_, err := Encrypt([]byte("x"), "")
	if err == nil {
		t.Error("expected error for empty key")
	}
}
