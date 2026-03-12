package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

const gcmNonceSize = 12

// Encrypt encrypts plaintext with the given key string using AES-256-GCM.
// The key is derived via SHA-256 to produce a 32-byte key. Returns base64(nonce||ciphertext).
func Encrypt(plaintext []byte, key string) (ciphertextBase64 string, err error) {
	if key == "" {
		return "", errors.New("encryption key is required")
	}
	keyBytes := deriveKey(key)
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcmNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a value produced by Encrypt (base64(nonce||ciphertext)) using the given key string.
func Decrypt(ciphertextBase64 string, key string) (plaintext []byte, err error) {
	if key == "" {
		return nil, errors.New("encryption key is required")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcmNonceSize {
		return nil, errors.New("ciphertext too short")
	}
	keyBytes := deriveKey(key)
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, ciphertext := ciphertext[:gcmNonceSize], ciphertext[gcmNonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func deriveKey(key string) []byte {
	h := sha256.Sum256([]byte(key))
	return h[:]
}
