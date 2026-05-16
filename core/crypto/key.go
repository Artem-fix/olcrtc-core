// Package crypto provides all cryptographic primitives for olcrtc-core.
// It does NOT use any Xray/V2Ray specific constructs.
// Security model: ChaCha20-Poly1305 AEAD with HKDF-SHA256 key derivation.
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	// KeySize is the required length for raw symmetric keys.
	KeySize = 32

	// NonceSize is the ChaCha20-Poly1305 nonce length.
	NonceSize = chacha20poly1305.NonceSizeX // 24 bytes (XChaCha20)

	// TagSize is the authentication tag length appended to ciphertext.
	TagSize = chacha20poly1305.Overhead // 16 bytes

	hkdfInfo = "olcrtc-core/v1"
)

// Key is a 32-byte symmetric key.
type Key [KeySize]byte

// GenerateKey creates a cryptographically random key.
func GenerateKey() (Key, error) {
	var k Key
	if _, err := io.ReadFull(rand.Reader, k[:]); err != nil {
		return k, fmt.Errorf("generate key: %w", err)
	}
	return k, nil
}

// KeyFromHex decodes a 64-char hex string into a Key.
func KeyFromHex(s string) (Key, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return Key{}, fmt.Errorf("decode key hex: %w", err)
	}
	if len(b) != KeySize {
		return Key{}, fmt.Errorf("key must be %d bytes, got %d", KeySize, len(b))
	}
	var k Key
	copy(k[:], b)
	return k, nil
}

// Hex returns the hex-encoded string representation of the key.
func (k Key) Hex() string {
	return hex.EncodeToString(k[:])
}

// DeriveSubkey runs HKDF-SHA256 to derive a sub-key for a specific purpose.
// purpose is a short ASCII label, e.g. "handshake" or "data".
func DeriveSubkey(master Key, salt []byte, purpose string) (Key, error) {
	info := []byte(hkdfInfo + "/" + purpose)
	reader := hkdf.New(sha256.New, master[:], salt, info)

	var out Key
	if _, err := io.ReadFull(reader, out[:]); err != nil {
		return out, fmt.Errorf("hkdf derive: %w", err)
	}
	return out, nil
}