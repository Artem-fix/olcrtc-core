// Package provider defines the abstraction for WebRTC carrier backends
// (e.g. Telemost, Jazz, Wbstream). A Provider is responsible for creating
// and destroying WebRTC Peer Connections inside a given room.
package provider

import (
	"context"
	"io"

	"github.com/pion/webrtc/v4"
)

// RoomConfig holds parameters for joining or creating a room.
type RoomConfig struct {
	// RoomID is the provider-specific room identifier.
	// If empty, the provider creates a new room.
	RoomID string

	// DisplayName is a human-readable peer name for the room roster.
	DisplayName string

	// ExtraHeaders are provider-specific HTTP headers (e.g. auth tokens).
	ExtraHeaders map[string]string

	// SOCKSProxy optionally routes provider API calls through a SOCKS5 proxy.
	SOCKSProxy string
}

// Peer represents a remote WebRTC peer reachable through the provider.
type Peer interface {
	io.Closer

	// ID returns the provider-assigned peer identifier.
	ID() string

	// PeerConnection returns the underlying WebRTC connection.
	PeerConnection() *webrtc.PeerConnection

	// Done returns a channel closed when the peer disconnects.
	Done() <-chan struct{}
}

// Provider is the interface every carrier backend must implement.
type Provider interface {
	io.Closer

	// Name returns the provider's unique identifier (e.g. "telemost").
	Name() string

	// Join joins (or creates) a room and returns the local peer.
	// The returned Peer is the local side; incoming peers are delivered
	// via the PeerHandler callback set during creation.
	Join(ctx context.Context, cfg RoomConfig) (Peer, error)

	// RoomID returns the joined room ID (valid only after a successful Join).
	RoomID() string
}

// PeerHandler is called each time a new remote peer connects.
type PeerHandler func(ctx context.Context, peer Peer)

// Config is passed to a Provider factory.
type Config struct {
	// Handler is invoked for each incoming peer.
	Handler PeerHandler

	// APIURL overrides the default provider endpoint.
	APIURL string

	// Extra holds provider-specific configuration keys.
	Extra map[string]any
}