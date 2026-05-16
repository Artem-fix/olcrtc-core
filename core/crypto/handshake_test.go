package crypto_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/openlibrecommunity/olcrtc-core/core/session"
	"github.com/openlibrecommunity/olcrtc-core/core/crypto"
)

// TestHandshake_ClientServer verifies the full two-party handshake over a
// synchronous in-memory pipe.
func TestHandshake_ClientServer(t *testing.T) {
	t.Parallel()

	masterKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	clientConn, serverConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()
	defer func() { _ = serverConn.Close() }()

	type hsResult struct {
		res session.HandshakeResultExported
		err error
	}

	clientCh := make(chan hsResult, 1)
	serverCh := make(chan hsResult, 1)

	ctx := context.Background()
	timeout := 5 * time.Second

	go func() {
		r, e := session.ClientHandshakeExported(ctx, clientConn, masterKey, timeout)
		clientCh <- hsResult{r, e}
	}()
	go func() {
		r, e := session.ServerHandshakeExported(ctx, serverConn, masterKey, timeout)
		serverCh <- hsResult{r, e}
	}()

	cr := <-clientCh
	sr := <-serverCh

	if cr.err != nil {
		t.Fatalf("client handshake: %v", cr.err)
	}
	if sr.err != nil {
		t.Fatalf("server handshake: %v", sr.err)
	}

	// Client's send key must equal server's recv key, and vice versa.
	if cr.res.SendKey != sr.res.RecvKey {
		t.Fatal("client send key != server recv key")
	}
	if cr.res.RecvKey != sr.res.SendKey {
		t.Fatal("client recv key != server send key")
	}
}