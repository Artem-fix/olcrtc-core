// Package transport defines the interface for olcrtc data channels.
// Three transport flavours exist:
//   - DataChannel   – WebRTC data channel (lowest latency, available everywhere)
//   - VideoChannel  – steganographic H.264/VP8 video frames (high bandwidth)
//   - SEIChannel    – H.264 SEI NAL units (medium bandwidth, lower visibility)
//
// All transports expose a net.Conn-compatible interface so upper layers
// (mux, session) can be transport-agnostic.
package transport

import (
	"context"
	"io"
	"net"

	"github.com/openlibrecommunity/olcrtc-core/core/provider"
)

// Kind identifies the underlying carrier mechanism.
type Kind string

const (
	KindDataChannel  Kind = "datachannel"
	KindVideoChannel Kind = "videochannel"
	KindSEIChannel   Kind = "seichannel"
	KindVP8Channel   Kind = "vp8channel"
)

// Config holds parameters shared by all transport implementations.
type Config struct {
	// Kind selects the transport mechanism.
	Kind Kind

	// MTU overrides the default maximum transfer unit (bytes).
	// 0 means use the transport default.
	MTU int

	// Extra holds transport-specific parameters.
	Extra map[string]any
}

// Transport represents a bidirectional byte-stream between two olcrtc peers.
// It wraps a net.Conn so existing Go networking code can use it directly.
type Transport interface {
	net.Conn
	io.ReaderFrom
	io.WriterTo

	// Kind returns the transport type.
	Kind() Kind

	// Stats returns current throughput statistics.
	Stats() Stats
}

// Stats holds runtime counters for a transport instance.
type Stats struct {
	BytesRead    uint64
	BytesWritten uint64
	PacketsSent  uint64
	PacketsRecv  uint64
}

// Dialer opens an outbound Transport to a remote peer.
type Dialer interface {
	// Dial establishes a transport connection using the given peer connection.
	Dial(ctx context.Context, peer provider.Peer, cfg Config) (Transport, error)
}

// Listener accepts inbound Transport connections from remote peers.
type Listener interface {
	// Accept waits for and returns the next inbound transport.
	Accept(ctx context.Context) (Transport, error)

	// Close stops the listener.
	Close() error
}

// Factory creates a Dialer and/or Listener for a given transport Kind.
type Factory interface {
	NewDialer() Dialer
	NewListener(peer provider.Peer, cfg Config) (Listener, error)
	Kind() Kind
}