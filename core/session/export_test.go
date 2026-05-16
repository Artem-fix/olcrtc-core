// export_test.go exposes internal handshake functions for white-box testing.
package session

import (
	"context"
	"net"
	"time"

	"github.com/openlibrecommunity/olcrtc-core/core/crypto"
)

// HandshakeResultExported is the exported view of handshakeResult for tests.
type HandshakeResultExported struct {
	SendKey crypto.Key
	RecvKey crypto.Key
}

// ClientHandshakeExported wraps ClientHandshake for tests.
func ClientHandshakeExported(ctx context.Context, conn net.Conn, key crypto.Key, timeout time.Duration) (HandshakeResultExported, error) {
	r, err := ClientHandshake(ctx, conn, key, timeout)
	return HandshakeResultExported{SendKey: r.sendKey, RecvKey: r.recvKey}, err
}

// ServerHandshakeExported wraps ServerHandshake for tests.
func ServerHandshakeExported(ctx context.Context, conn net.Conn, key crypto.Key, timeout time.Duration) (HandshakeResultExported, error) {
	r, err := ServerHandshake(ctx, conn, key, timeout)
	return HandshakeResultExported{SendKey: r.sendKey, RecvKey: r.recvKey}, err
}