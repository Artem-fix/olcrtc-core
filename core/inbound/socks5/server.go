// Package socks5 provides a minimal SOCKS5 server that acts as the local
// inbound handler on the client side. Each accepted SOCKS5 connection is
// turned into a mux stream and forwarded to the server peer.
package socks5

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/openlibrecommunity/olcrtc-core/core/log"
	"github.com/openlibrecommunity/olcrtc-core/core/mux"
)

const (
	socks5Version = 0x05
	cmdConnect    = 0x01
	atypIPv4      = 0x01
	atypDomain    = 0x03
	atypIPv6      = 0x04

	authNone     = 0x00
	authNoAccept = 0xFF

	replySuccess = 0x00
	replyFail    = 0x01

	handshakeTimeout = 10 * time.Second
)

// Handler wraps a mux.Session and proxies accepted SOCKS5 connections
// through it as multiplexed streams.
type Handler struct {
	sess   *mux.Session
	logger *zap.Logger
	wg     sync.WaitGroup
}

// NewHandler creates a Handler backed by sess.
func NewHandler(sess *mux.Session) *Handler {
	return &Handler{
		sess:   sess,
		logger: log.Named("socks5"),
	}
}

// Serve accepts connections from ln and proxies them through the mux session.
// Blocks until ln is closed or ctx is cancelled.
func (h *Handler) Serve(ctx context.Context, ln net.Listener) error {
	defer h.wg.Wait()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("socks5 accept: %w", err)
		}

		h.wg.Add(1)
		go func(c net.Conn) {
			defer h.wg.Done()
			h.handleConn(ctx, c)
		}(conn)
	}
}

func (h *Handler) handleConn(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		h.logger.Warn("set deadline", zap.Error(err))
		return
	}

	dest, err := h.negotiate(conn)
	if err != nil {
		h.logger.Warn("socks5 negotiate", zap.Error(err))
		return
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		h.logger.Warn("clear deadline", zap.Error(err))
		return
	}

	// Open mux stream.
	stream, err := h.sess.OpenStream()
	if err != nil {
		h.logger.Warn("open mux stream", zap.String("dest", dest), zap.Error(err))
		_, _ = conn.Write([]byte{socks5Version, replyFail, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
		return
	}
	defer func() { _ = stream.Close() }()

	// Send SOCKS5 success reply.
	reply := []byte{socks5Version, replySuccess, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0}
	if _, err := conn.Write(reply); err != nil {
		h.logger.Warn("write socks reply", zap.Error(err))
		return
	}

	// Write destination to stream so server knows where to forward.
	if err := writeDestination(stream, dest); err != nil {
		h.logger.Warn("write destination", zap.Error(err))
		return
	}

	h.logger.Debug("proxying", zap.String("dest", dest))
	sent, recv, _ := mux.Pipe(ctx, conn, stream)
	h.logger.Debug("done",
		zap.String("dest", dest),
		zap.Int64("sent", sent),
		zap.Int64("recv", recv),
	)
}

// negotiate performs the SOCKS5 handshake and returns the target address.
func (h *Handler) negotiate(conn net.Conn) (string, error) {
	// Read greeting.
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", fmt.Errorf("read header: %w", err)
	}
	if header[0] != socks5Version {
		return "", fmt.Errorf("unsupported socks version: %d", header[0])
	}

	nMethods := int(header[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", fmt.Errorf("read methods: %w", err)
	}

	// We only support no-auth.
	hasNoAuth := false
	for _, m := range methods {
		if m == authNone {
			hasNoAuth = true
			break
		}
	}
	if !hasNoAuth {
		_, _ = conn.Write([]byte{socks5Version, authNoAccept})
		return "", fmt.Errorf("no acceptable auth method")
	}
	if _, err := conn.Write([]byte{socks5Version, authNone}); err != nil {
		return "", fmt.Errorf("write auth choice: %w", err)
	}

	// Read request.
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(conn, reqHeader); err != nil {
		return "", fmt.Errorf("read request: %w", err)
	}
	if reqHeader[0] != socks5Version {
		return "", fmt.Errorf("bad socks version in request")
	}
	if reqHeader[1] != cmdConnect {
		return "", fmt.Errorf("unsupported command: %d", reqHeader[1])
	}

	// Parse address.
	var host string
	switch reqHeader[3] {
	case atypIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	case atypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", err
		}
		domain := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", err
		}
		host = string(domain)
	case atypIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	default:
		return "", fmt.Errorf("unsupported address type: %d", reqHeader[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBuf)

	return fmt.Sprintf("%s:%d", host, port), nil
}

// writeDestination encodes the target address as [1-byte len][addr string] into w.
func writeDestination(w io.Writer, dest string) error {
	b := []byte(dest)
	if len(b) > 255 {
		return fmt.Errorf("destination address too long: %d", len(b))
	}
	header := []byte{byte(len(b))}
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

// ReadDestination reads a destination written by writeDestination.
func ReadDestination(r io.Reader) (string, error) {
	lenBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return "", fmt.Errorf("read dest len: %w", err)
	}
	dest := make([]byte, int(lenBuf[0]))
	if _, err := io.ReadFull(r, dest); err != nil {
		return "", fmt.Errorf("read dest: %w", err)
	}
	return string(dest), nil
}