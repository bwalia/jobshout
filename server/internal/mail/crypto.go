package mail

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// KeyFromSecret derives a 32-byte AES key from GMAIL_TOKEN_KEY.
// A 64-char hex string is used as raw bytes; anything else is SHA-256 hashed
// so an operator can set a passphrase without generating a key file.
func KeyFromSecret(secret string) ([]byte, error) {
	if secret == "" {
		return nil, errors.New("mail: GMAIL_TOKEN_KEY is empty")
	}
	if len(secret) == 64 {
		if b, err := hex.DecodeString(secret); err == nil && len(b) == 32 {
			return b, nil
		}
	}
	sum := sha256.Sum256([]byte(secret))
	return sum[:], nil
}

// Encrypt seals plaintext with AES-256-GCM. The nonce is prepended.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("mail: encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("mail: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("mail: gcm: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("mail: nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens a blob produced by Encrypt.
func Decrypt(key, blob []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("mail: encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("mail: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("mail: gcm: %w", err)
	}
	ns := aead.NonceSize()
	if len(blob) < ns {
		return nil, errors.New("mail: ciphertext too short")
	}
	plain, err := aead.Open(nil, blob[:ns], blob[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("mail: decrypt: %w", err)
	}
	return plain, nil
}
