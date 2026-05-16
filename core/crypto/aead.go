package crypto

import (
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

// AEAD wraps an XChaCha20-Poly1305 cipher for encrypt/decrypt operations.
type AEAD struct {
	aead interface {
		Seal(dst, nonce, plaintext, additionalData []byte) []byte
		Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
		NonceSize() int
		Overhead() int
	}
}

// NewAEAD creates a new AEAD cipher from a 32-byte key.
func NewAEAD(key Key) (*AEAD, error) {
	c, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		return nil, fmt.Errorf("new xchacha20poly1305: %w", err)
	}
	return &AEAD{aead: c}, nil
}

// Seal encrypts and authenticates plaintext, appending the result to dst.
// A random nonce is prepended to the output automatically.
// Format: [nonce(24) | ciphertext | tag(16)]
func (a *AEAD) Seal(dst, plaintext, additionalData []byte) ([]byte, error) {
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	dst = append(dst, nonce...)
	dst = a.aead.Seal(dst, nonce, plaintext, additionalData)
	return dst, nil
}

// Open decrypts and authenticates data produced by Seal.
// The leading nonce is consumed automatically.
func (a *AEAD) Open(dst, data, additionalData []byte) ([]byte, error) {
	if len(data) < NonceSize+TagSize {
		return nil, fmt.Errorf("ciphertext too short (%d bytes)", len(data))
	}
	nonce := data[:NonceSize]
	ct := data[NonceSize:]

	out, err := a.aead.Open(dst, nonce, ct, additionalData)
	if err != nil {
		return nil, fmt.Errorf("aead open: %w", err)
	}
	return out, nil
}

// Overhead returns the byte overhead added by Seal (nonce + tag).
func (a *AEAD) Overhead() int {
	return NonceSize + TagSize
}