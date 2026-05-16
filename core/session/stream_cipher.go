package session

import (
	"fmt"
	"io"
	"net"

	"github.com/openlibrecommunity/olcrtc-core/core/crypto"
	"github.com/openlibrecommunity/olcrtc-core/core/common"
)

// encryptedConn wraps a net.Conn and applies AEAD encryption to every
// logical message. Wire format per message:
//
//	[4-byte big-endian payload length] [nonce(24) | ciphertext | tag(16)]
type encryptedConn struct {
	net.Conn
	sendAEAD *crypto.AEAD
	recvAEAD *crypto.AEAD
	readBuf  []byte
	writeBuf []byte
}

// newEncryptedConn wraps conn with the given AEAD ciphers.
func newEncryptedConn(conn net.Conn, sendKey, recvKey crypto.Key) (*encryptedConn, error) {
	sa, err := crypto.NewAEAD(sendKey)
	if err != nil {
		return nil, fmt.Errorf("new send aead: %w", err)
	}
	ra, err := crypto.NewAEAD(recvKey)
	if err != nil {
		return nil, fmt.Errorf("new recv aead: %w", err)
	}
	return &encryptedConn{
		Conn:     conn,
		sendAEAD: sa,
		recvAEAD: ra,
		readBuf:  common.GlobalPool.Get(),
		writeBuf: common.GlobalPool.Get(),
	}, nil
}

// Write encrypts plaintext and writes it as a length-prefixed frame.
func (c *encryptedConn) Write(plaintext []byte) (int, error) {
	enc, err := c.sendAEAD.Seal(c.writeBuf[:0], plaintext, nil)
	if err != nil {
		return 0, fmt.Errorf("seal: %w", err)
	}
	if err := writeLenPrefixed(c.Conn, enc); err != nil {
		return 0, err
	}
	return len(plaintext), nil
}

// Read decrypts one frame from the connection.
func (c *encryptedConn) Read(dst []byte) (int, error) {
	frame, err := readLenPrefixed(c.Conn)
	if err != nil {
		if err == io.EOF {
			return 0, io.EOF
		}
		return 0, fmt.Errorf("read frame: %w", err)
	}
	plain, err := c.recvAEAD.Open(c.readBuf[:0], frame, nil)
	if err != nil {
		return 0, fmt.Errorf("open: %w", err)
	}
	n := copy(dst, plain)
	return n, nil
}

// Close releases pooled buffers before delegating to the underlying conn.
func (c *encryptedConn) Close() error {
	common.GlobalPool.Put(c.readBuf)
	common.GlobalPool.Put(c.writeBuf)
	return c.Conn.Close()
}