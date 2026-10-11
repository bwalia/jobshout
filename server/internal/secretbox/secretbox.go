// Package secretbox seals tenant secrets (LLM provider API keys, webhook
// signing secrets) for storage with AES-256-GCM.
//
// Sealed values are text — "enc:v1:" + base64(nonce||ciphertext) — so they fit
// the existing TEXT columns without a schema change, and a value without the
// prefix is treated as legacy plaintext. That keeps rows written before
// SECRETS_ENCRYPTION_KEY was set readable, and lets an operator turn
// encryption on without a data migration: each secret is sealed the next time
// it is written.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Prefix marks a sealed value.
const Prefix = "enc:v1:"

// ErrNoKey is returned when a sealed value is read but no key is configured.
var ErrNoKey = errors.New("secretbox: value is encrypted but SECRETS_ENCRYPTION_KEY is not set")

// Box seals and opens secrets. A nil *Box is valid and stores plaintext, so
// callers never branch on whether encryption is configured.
type Box struct {
	aead cipher.AEAD
}

// New derives a Box from secret. A 64-char hex secret is the raw 32-byte key;
// anything else is SHA-256 hashed. An empty secret returns (nil, nil): the
// plaintext box.
func New(secret string) (*Box, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, nil
	}
	var key []byte
	if len(secret) == 64 {
		if b, err := hex.DecodeString(secret); err == nil {
			key = b
		}
	}
	if key == nil {
		sum := sha256.Sum256([]byte(secret))
		key = sum[:]
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Enabled reports whether values are actually encrypted.
func (b *Box) Enabled() bool { return b != nil }

// Seal encrypts plain. The empty string stays empty (nothing to protect, and
// "no key set" must remain distinguishable). A nil Box returns plain as is.
func (b *Box) Seal(plain string) (string, error) {
	if b == nil || plain == "" || IsSealed(plain) {
		return plain, nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secretbox: nonce: %w", err)
	}
	blob := b.aead.Seal(nonce, nonce, []byte(plain), nil)
	return Prefix + base64.StdEncoding.EncodeToString(blob), nil
}

// Open decrypts a stored value. Legacy plaintext (no prefix) is returned as is.
func (b *Box) Open(stored string) (string, error) {
	if !IsSealed(stored) {
		return stored, nil
	}
	if b == nil {
		return "", ErrNoKey
	}
	blob, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, Prefix))
	if err != nil {
		return "", fmt.Errorf("secretbox: decode: %w", err)
	}
	ns := b.aead.NonceSize()
	if len(blob) < ns {
		return "", errors.New("secretbox: ciphertext too short")
	}
	plain, err := b.aead.Open(nil, blob[:ns], blob[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("secretbox: decrypt: %w", err)
	}
	return string(plain), nil
}

// IsSealed reports whether s carries the sealed-value prefix.
func IsSealed(s string) bool { return strings.HasPrefix(s, Prefix) }
