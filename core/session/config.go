// Package session orchestrates a full olcrtc session: it connects to a
// provider, establishes a transport, sets up mux, and runs the dispatcher
// loop. It is the main entry point for both client and server modes.
package session

import (
	"fmt"
	"time"

	"github.com/openlibrecommunity/olcrtc-core/core/transport"
)

// Mode determines whether this node acts as a client or server.
type Mode string

const (
	ModeClient Mode = "client"
	ModeServer Mode = "server"
)

// Config holds all parameters required to start a session.
type Config struct {
	// Mode selects client or server role.
	Mode Mode

	// Provider names the carrier backend (e.g. "telemost", "jazz", "wbstream").
	Provider string

	// RoomID is the room to join. Empty means create a new room (server only).
	RoomID string

	// DisplayName is shown in the provider's room roster.
	DisplayName string

	// KeyHex is the 64-char hex-encoded shared symmetric key.
	// Both peers must have the same key.
	KeyHex string

	// Transport selects the channel mechanism (datachannel, videochannel, etc.).
	Transport transport.Kind

	// TransportExtra holds transport-specific tuning options.
	TransportExtra map[string]any

	// ListenAddr is the local SOCKS5 / TCP proxy bind address (client mode).
	// Example: "127.0.0.1:1080"
	ListenAddr string

	// ForwardAddr is the TCP destination streams are forwarded to (server mode).
	// Example: "127.0.0.1:80"
	ForwardAddr string

	// SOCKSProxy optionally routes provider API calls through a SOCKS5 proxy.
	SOCKSProxy string

	// DNSServer overrides the system resolver. Empty means system default.
	DNSServer string

	// ReconnectDelay is the time between reconnection attempts.
	// Zero disables auto-reconnect.
	ReconnectDelay time.Duration

	// HandshakeTimeout is the maximum time allowed for the crypto handshake.
	HandshakeTimeout time.Duration

	// DialTimeout is the maximum time allowed to open a transport.
	DialTimeout time.Duration
}

// Validate checks that the configuration is complete and consistent.
func Validate(cfg Config) error {
	if cfg.Provider == "" {
		return fmt.Errorf("%w: provider is required", ErrConfig)
	}
	if cfg.KeyHex == "" {
		return fmt.Errorf("%w: key is required", ErrConfig)
	}
	if len(cfg.KeyHex) != 64 {
		return fmt.Errorf("%w: key must be 64 hex chars", ErrConfig)
	}
	switch cfg.Mode {
	case ModeClient:
		if cfg.ListenAddr == "" {
			return fmt.Errorf("%w: listen_addr is required in client mode", ErrConfig)
		}
		if cfg.RoomID == "" {
			return fmt.Errorf("%w: room_id is required in client mode", ErrConfig)
		}
	case ModeServer:
		if cfg.ForwardAddr == "" {
			return fmt.Errorf("%w: forward_addr is required in server mode", ErrConfig)
		}
	default:
		return fmt.Errorf("%w: mode must be %q or %q", ErrConfig, ModeClient, ModeServer)
	}
	return nil
}

// ErrConfig is the sentinel for configuration errors.
var ErrConfig = fmt.Errorf("invalid config")