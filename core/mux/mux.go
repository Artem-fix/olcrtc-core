// Package mux provides stream multiplexing over a single Transport connection
// using xtaci/smux. This allows multiple logical streams (SOCKS, HTTP proxy,
// control channel) to share one WebRTC DataChannel or VideoChannel.
package mux

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/xtaci/smux"
	"go.uber.org/zap"

	"github.com/openlibrecommunity/olcrtc-core/core/log"
	"github.com/openlibrecommunity/olcrtc-core/core/transport"
)

// defaultSmuxConfig returns a production-safe smux config.
func defaultSmuxConfig() *smux.Config {
	cfg := smux.DefaultConfig()
	cfg.Version = 2
	cfg.KeepAliveInterval = 15 * time.Second
	cfg.KeepAliveTimeout = 45 * time.Second
	cfg.MaxFrameSize = 32 * 1024
	cfg.MaxReceiveBuffer = 4 * 1024 * 1024
	cfg.MaxStreamBuffer = 512 * 1024
	return cfg
}

// Session wraps an smux session (client or server).
type Session struct {
	inner     *smux.Session
	logger    *zap.Logger
	closeOnce sync.Once
}

// NewClientSession wraps a transport as an smux client (initiator).
func NewClientSession(t transport.Transport) (*Session, error) {
	inner, err := smux.Client(t, defaultSmuxConfig())
	if err != nil {
		return nil, fmt.Errorf("smux client: %w", err)
	}
	return &Session{
		inner:  inner,
		logger: log.Named("mux.client"),
	}, nil
}

// NewServerSession wraps a transport as an smux server (acceptor).
func NewServerSession(t transport.Transport) (*Session, error) {
	inner, err := smux.Server(t, defaultSmuxConfig())
	if err != nil {
		return nil, fmt.Errorf("smux server: %w", err)
	}
	return &Session{
		inner:  inner,
		logger: log.Named("mux.server"),
	}, nil
}

// OpenStream opens a new logical stream inside the session.
func (s *Session) OpenStream() (net.Conn, error) {
	st, err := s.inner.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	return st, nil
}

// AcceptStream waits for the next inbound logical stream.
func (s *Session) AcceptStream(ctx context.Context) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}

	ch := make(chan result, 1)
	go func() {
		st, err := s.inner.AcceptStream()
		ch <- result{st, err}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("accept stream: %w", r.err)
		}
		return r.conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close shuts down the mux session.
func (s *Session) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.logger.Info("closing mux session")
		err = s.inner.Close()
	})
	return err
}

// IsClosed reports whether the session has been closed.
func (s *Session) IsClosed() bool {
	return s.inner.IsClosed()
}

// Pipe copies data between two net.Conns concurrently and closes both when done.
// Returns the total bytes transferred in each direction.
func Pipe(ctx context.Context, a, b net.Conn) (int64, int64, error) {
	type result struct {
		n   int64
		err error
	}

	var (
		wg   sync.WaitGroup
		resA = make(chan result, 1)
		resB = make(chan result, 1)
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		n, err := io.Copy(a, b)
		resA <- result{n, err}
		_ = a.Close()
	}()
	go func() {
		defer wg.Done()
		n, err := io.Copy(b, a)
		resB <- result{n, err}
		_ = b.Close()
	}()

	// Close both ends if context is cancelled.
	go func() {
		select {
		case <-ctx.Done():
			_ = a.Close()
			_ = b.Close()
		case <-waitGroup(&wg):
		}
	}()

	wg.Wait()

	rA := <-resA
	rB := <-resB

	if rA.err != nil && rA.err != io.EOF {
		return rA.n, rB.n, rA.err
	}
	if rB.err != nil && rB.err != io.EOF {
		return rA.n, rB.n, rB.err
	}
	return rA.n, rB.n, nil
}

func waitGroup(wg *sync.WaitGroup) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	return done
}