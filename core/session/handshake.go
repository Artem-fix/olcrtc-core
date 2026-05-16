package session

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/openlibrecommunity/olcrtc-core/core/crypto"
)

// handshakeTimeout is the default max time for the crypto handshake.
const defaultHandshakeTimeout = 15 * time.Second

// handshakeResult holds negotiated session keys after a successful handshake.
type handshakeResult struct {
	sendKey crypto.Key
	recvKey crypto.Key
}

// ClientHandshake performs the initiator side of the olcrtc handshake:
//
//  1. Client generates ephemeral X25519 keypair.
//  2. Client sends: [pubKey(32)]
//  3. Server sends: [pubKey(32)] + AEAD{ "olcrtc-hello" }
//  4. Client derives shared secret → session keys.
//  5. Client sends: AEAD{ "olcrtc-ready" }
//
// masterKey is the pre-shared key used as HKDF master.
func ClientHandshake(ctx context.Context, conn net.Conn, masterKey crypto.Key, timeout time.Duration) (handshakeResult, error) {
	if timeout <= 0 {
		timeout = defaultHandshakeTimeout
	}
	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return handshakeResult{}, fmt.Errorf("set handshake deadline: %w", err)
	}
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	// Step 1: generate ephemeral keypair.
	kp, err := crypto.GenerateHandshakeKeypair()
	if err != nil {
		return handshakeResult{}, err
	}

	// Step 2: send our ephemeral public key.
	if _, err := conn.Write(kp.PublicKeyBytes()); err != nil {
		return handshakeResult{}, fmt.Errorf("write client pubkey: %w", err)
	}

	// Step 3: receive server's ephemeral public key + encrypted hello.
	serverPubBytes := make([]byte, 32)
	if _, err := io.ReadFull(conn, serverPubBytes); err != nil {
		return handshakeResult{}, fmt.Errorf("read server pubkey: %w", err)
	}
	serverPub, err := crypto.PublicKeyFromBytes(serverPubBytes)
	if err != nil {
		return handshakeResult{}, err
	}

	// Read encrypted hello length + payload.
	helloEnc, err := readLenPrefixed(conn)
	if err != nil {
		return handshakeResult{}, fmt.Errorf("read server hello: %w", err)
	}

	// Step 4: derive shared secret and session keys.
	sharedSecret, err := kp.DH(serverPub)
	if err != nil {
		return handshakeResult{}, err
	}
	sendKey, recvKey, err := deriveSessionKeys(masterKey, sharedSecret, kp.PublicKeyBytes(), serverPubBytes)
	if err != nil {
		return handshakeResult{}, err
	}

	// Verify server hello.
	recvAEAD, err := crypto.NewAEAD(recvKey)
	if err != nil {
		return handshakeResult{}, err
	}
	helloPlain, err := recvAEAD.Open(nil, helloEnc, nil)
	if err != nil {
		return handshakeResult{}, fmt.Errorf("decrypt server hello: %w", err)
	}
	if string(helloPlain) != "olcrtc-hello" {
		return handshakeResult{}, fmt.Errorf("handshake: unexpected server hello")
	}

	// Step 5: send ready.
	sendAEAD, err := crypto.NewAEAD(sendKey)
	if err != nil {
		return handshakeResult{}, err
	}
	readyEnc, err := sendAEAD.Seal(nil, []byte("olcrtc-ready"), nil)
	if err != nil {
		return handshakeResult{}, err
	}
	if err := writeLenPrefixed(conn, readyEnc); err != nil {
		return handshakeResult{}, fmt.Errorf("write client ready: %w", err)
	}

	return handshakeResult{sendKey: sendKey, recvKey: recvKey}, nil
}

// ServerHandshake performs the responder side of the handshake.
func ServerHandshake(ctx context.Context, conn net.Conn, masterKey crypto.Key, timeout time.Duration) (handshakeResult, error) {
	if timeout <= 0 {
		timeout = defaultHandshakeTimeout
	}
	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return handshakeResult{}, fmt.Errorf("set handshake deadline: %w", err)
	}
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	// Read client ephemeral public key.
	clientPubBytes := make([]byte, 32)
	if _, err := io.ReadFull(conn, clientPubBytes); err != nil {
		return handshakeResult{}, fmt.Errorf("read client pubkey: %w", err)
	}
	clientPub, err := crypto.PublicKeyFromBytes(clientPubBytes)
	if err != nil {
		return handshakeResult{}, err
	}

	// Generate server ephemeral keypair.
	kp, err := crypto.GenerateHandshakeKeypair()
	if err != nil {
		return handshakeResult{}, err
	}

	// Derive shared secret and session keys.
	sharedSecret, err := kp.DH(clientPub)
	if err != nil {
		return handshakeResult{}, err
	}
	// Server's send == client's recv, and vice versa.
	recvKey, sendKey, err := deriveSessionKeys(masterKey, sharedSecret, clientPubBytes, kp.PublicKeyBytes())
	if err != nil {
		return handshakeResult{}, err
	}

	// Send server public key.
	if _, err := conn.Write(kp.PublicKeyBytes()); err != nil {
		return handshakeResult{}, fmt.Errorf("write server pubkey: %w", err)
	}

	// Send encrypted hello.
	sendAEAD, err := crypto.NewAEAD(sendKey)
	if err != nil {
		return handshakeResult{}, err
	}
	helloEnc, err := sendAEAD.Seal(nil, []byte("olcrtc-hello"), nil)
	if err != nil {
		return handshakeResult{}, err
	}
	if err := writeLenPrefixed(conn, helloEnc); err != nil {
		return handshakeResult{}, fmt.Errorf("write server hello: %w", err)
	}

	// Read client ready.
	recvAEAD, err := crypto.NewAEAD(recvKey)
	if err != nil {
		return handshakeResult{}, err
	}
	readyEnc, err := readLenPrefixed(conn)
	if err != nil {
		return handshakeResult{}, fmt.Errorf("read client ready: %w", err)
	}
	readyPlain, err := recvAEAD.Open(nil, readyEnc, nil)
	if err != nil {
		return handshakeResult{}, fmt.Errorf("decrypt client ready: %w", err)
	}
	if string(readyPlain) != "olcrtc-ready" {
		return handshakeResult{}, fmt.Errorf("handshake: unexpected client ready")
	}

	return handshakeResult{sendKey: sendKey, recvKey: recvKey}, nil
}

// deriveSessionKeys derives two independent session keys from the shared
// DH secret and both parties' public keys (to prevent key-reuse attacks).
// Returns (initiatorSend, responderSend).
func deriveSessionKeys(master crypto.Key, shared, initPub, respPub []byte) (crypto.Key, crypto.Key, error) {
	salt := append(initPub, respPub...) //nolint:gocritic
	salt = append(salt, shared...)

	k1, err := crypto.DeriveSubkey(master, salt, "session/send")
	if err != nil {
		return crypto.Key{}, crypto.Key{}, err
	}
	k2, err := crypto.DeriveSubkey(master, salt, "session/recv")
	if err != nil {
		return crypto.Key{}, crypto.Key{}, err
	}
	return k1, k2, nil
}

// writeLenPrefixed writes a 4-byte big-endian length prefix followed by data.
func writeLenPrefixed(w io.Writer, data []byte) error {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(len(data)))
	if _, err := w.Write(buf[:]); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// readLenPrefixed reads a 4-byte big-endian length prefix then the payload.
func readLenPrefixed(r io.Reader) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lenBuf[:])
	if length > 16*1024*1024 { // 16 MiB sanity cap
		return nil, fmt.Errorf("message too large: %d bytes", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}