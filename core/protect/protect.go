// Package protect provides rate-limiting and connection-guard primitives
// to prevent abuse and resource exhaustion.
package protect

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/openlibrecommunity/olcrtc-core/core/common"
	"github.com/openlibrecommunity/olcrtc-core/core/log"
)

// Config configures the connection guard.
type Config struct {
	// MaxConns is the maximum number of simultaneously active connections.
	// Zero means unlimited.
	MaxConns int

	// ReadTimeout is applied to every accepted connection.
	// Zero means no timeout.
	ReadTimeout time.Duration

	// WriteTimeout is applied to every accepted connection.
	// Zero means no timeout.
	WriteTimeout time.Duration
}

// Guard wraps a net.Listener and enforces connection limits and timeouts.
type Guard struct {
	ln      net.Listener
	cfg     Config
	active  atomic.Int64
	logger  *zap.Logger
}

// NewGuard wraps ln with the given protections.
func NewGuard(ln net.Listener, cfg Config) *Guard {
	return &Guard{
		ln:     ln,
		cfg:    cfg,
		logger: log.Named("protect.guard"),
	}
}

// Accept returns the next connection, enforcing the max-connections limit.
func (g *Guard) Accept() (net.Conn, error) {
	for {
		conn, err := g.ln.Accept()
		if err != nil {
			return nil, err
		}

		if g.cfg.MaxConns > 0 && int(g.active.Load()) >= g.cfg.MaxConns {
			g.logger.Warn("max connections reached, rejecting",
				zap.Int("max", g.cfg.MaxConns),
			)
			_ = conn.Close()
			continue
		}

		g.active.Add(1)
		return &guardedConn{Conn: conn, guard: g, cfg: g.cfg}, nil
	}
}

// Close closes the underlying listener.
func (g *Guard) Close() error {
	return g.ln.Close()
}

// Addr returns the listener's address.
func (g *Guard) Addr() net.Addr {
	return g.ln.Addr()
}

// Active returns the number of currently active connections.
func (g *Guard) Active() int {
	return int(g.active.Load())
}

type guardedConn struct {
	net.Conn
	guard  *Guard
	cfg    Config
	closed atomic.Bool
}

func (c *guardedConn) Read(b []byte) (int, error) {
	if c.cfg.ReadTimeout > 0 {
		if err := c.Conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout)); err != nil {
			return 0, err
		}
	}
	return c.Conn.Read(b)
}

func (c *guardedConn) Write(b []byte) (int, error) {
	if c.cfg.WriteTimeout > 0 {
		if err := c.Conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout)); err != nil {
			return 0, err
		}
	}
	return c.Conn.Write(b)
}

func (c *guardedConn) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	c.guard.active.Add(-1)
	return c.Conn.Close()
}

// DialWithTimeout dials a TCP address with a mandatory timeout and context support.
func DialWithTimeout(ctx context.Context, network, addr string, timeout time.Duration) (net.Conn, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("%w: dial %s %s: %v", common.ErrTimeout, network, addr, err)
	}
	return conn, nil
}