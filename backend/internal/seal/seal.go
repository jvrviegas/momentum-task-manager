// Package seal encrypts a secret the server must use later, such as a Google
// refresh token (ADR 0003, D5), so a copy of the database alone does not
// expose it. The server's own tokens need none of this: it keeps only their
// hashes.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// KeySize is the length of a key in bytes: AES-256.
const KeySize = 32

// ErrOpen reports a sealed value that does not open: it was changed, it is
// too short, or the key is not the one it was sealed with. It says no more,
// so an attacker learns nothing from which of them it was.
var ErrOpen = errors.New("the sealed value does not open with this key")

// Seal encrypts plaintext with AES-256-GCM under key. The answer is a fresh
// random nonce followed by the ciphertext and its tag, so sealing the same
// plaintext twice gives different values.
func Seal(key, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize(), gcm.NonceSize()+len(plaintext)+gcm.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("make a nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts a value Seal made under the same key, or answers ErrOpen.
func Open(key, sealed []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize()+gcm.Overhead() {
		return nil, ErrOpen
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrOpen
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("the key must be %d bytes, not %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
