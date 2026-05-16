package crypto_test

import (
	"bytes"
	"testing"

	"github.com/openlibrecommunity/olcrtc-core/core/crypto"
)

func TestAEAD_RoundTrip(t *testing.T) {
	t.Parallel()

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	aead, err := crypto.NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD: %v", err)
	}

	plaintext := []byte("hello from olcrtc-core test suite")
	ad := []byte("additional-data")

	sealed, err := aead.Seal(nil, plaintext, ad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	opened, err := aead.Open(nil, sealed, ad)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if !bytes.Equal(plaintext, opened) {
		t.Fatalf("plaintext mismatch: got %q want %q", opened, plaintext)
	}
}

func TestAEAD_TamperedCiphertext(t *testing.T) {
	t.Parallel()

	key, _ := crypto.GenerateKey()
	aead, _ := crypto.NewAEAD(key)

	sealed, _ := aead.Seal(nil, []byte("secret"), nil)
	sealed[len(sealed)-1] ^= 0xFF // flip a bit

	if _, err := aead.Open(nil, sealed, nil); err == nil {
		t.Fatal("expected authentication failure on tampered ciphertext")
	}
}

func TestAEAD_Overhead(t *testing.T) {
	t.Parallel()

	key, _ := crypto.GenerateKey()
	aead, _ := crypto.NewAEAD(key)

	want := crypto.NonceSize + crypto.TagSize
	if aead.Overhead() != want {
		t.Fatalf("overhead: got %d want %d", aead.Overhead(), want)
	}
}

func TestDeriveSubkey_Deterministic(t *testing.T) {
	t.Parallel()

	key, _ := crypto.GenerateKey()
	salt := []byte("test-salt")

	k1, err := crypto.DeriveSubkey(key, salt, "purpose-a")
	if err != nil {
		t.Fatalf("DeriveSubkey: %v", err)
	}
	k2, err := crypto.DeriveSubkey(key, salt, "purpose-a")
	if err != nil {
		t.Fatalf("DeriveSubkey: %v", err)
	}
	if k1 != k2 {
		t.Fatal("derived keys must be deterministic")
	}

	k3, _ := crypto.DeriveSubkey(key, salt, "purpose-b")
	if k1 == k3 {
		t.Fatal("different purposes must produce different keys")
	}
}