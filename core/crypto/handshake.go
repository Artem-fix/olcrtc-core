package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
)

// HandshakeKeypair holds an ephemeral ECDH key pair (X25519).
type HandshakeKeypair struct {
	private *ecdh.PrivateKey
	Public  *ecdh.PublicKey
}

// GenerateHandshakeKeypair generates a fresh X25519 ephemeral keypair.
func GenerateHandshakeKeypair() (*HandshakeKeypair, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate x25519 keypair: %w", err)
	}
	return &HandshakeKeypair{
		private: priv,
		Public:  priv.PublicKey(),
	}, nil
}

// DH performs X25519 Diffie-Hellman with a peer's public key and returns
// the shared secret. The secret is deterministic; callers should derive
// subkeys via DeriveSubkey before use.
func (kp *HandshakeKeypair) DH(peerPublic *ecdh.PublicKey) ([]byte, error) {
	shared, err := kp.private.ECDH(peerPublic)
	if err != nil {
		return nil, fmt.Errorf("x25519 dh: %w", err)
	}
	return shared, nil
}

// PublicKeyBytes returns the 32-byte little-endian encoding of the public key.
func (kp *HandshakeKeypair) PublicKeyBytes() []byte {
	return kp.Public.Bytes()
}

// PublicKeyFromBytes decodes a 32-byte X25519 public key.
func PublicKeyFromBytes(b []byte) (*ecdh.PublicKey, error) {
	pub, err := ecdh.X25519().NewPublicKey(b)
	if err != nil {
		return nil, fmt.Errorf("decode x25519 public key: %w", err)
	}
	return pub, nil
}