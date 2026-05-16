// Package main provides the CLI entry point for olcrtc-core.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/openlibrecommunity/olcrtc-core/core/app"
	"github.com/openlibrecommunity/olcrtc-core/core/session"
	"github.com/openlibrecommunity/olcrtc-core/core/transport"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "olcrtc-core: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("olcrtc-core", flag.ExitOnError)

	mode        := fs.String("mode", "server", "session mode: client|server")
	provider    := fs.String("provider", "", "carrier provider: telemost|jazz|wbstream")
	roomID      := fs.String("room", "", "room ID (required for client; empty=create for server)")
	keyHex      := fs.String("key", "", "64-char hex shared key")
	transportKind := fs.String("transport", "datachannel", "transport: datachannel|videochannel|seichannel|vp8channel")
	listenAddr  := fs.String("listen", "127.0.0.1:1080", "local address to listen on (client mode)")
	forwardAddr := fs.String("forward", "", "address to forward streams to (server mode)")
	displayName := fs.String("name", "olcrtc-core", "peer display name in the room")
	socksProxy  := fs.String("socks", "", "SOCKS5 proxy for provider API calls (host:port)")
	dnsServer   := fs.String("dns", "", "DNS resolver override (host:port)")
	logLevel    := fs.String("log", "info", "log level: debug|info|warn|error")
	dev         := fs.Bool("dev", false, "enable development (human-readable) logging")

	handshakeTimeout := fs.Duration("handshake-timeout", 15*time.Second, "crypto handshake timeout")
	dialTimeout      := fs.Duration("dial-timeout", 30*time.Second, "transport dial timeout")
	reconnectDelay   := fs.Duration("reconnect", 0, "reconnect delay (0 = disabled)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	cfg := app.Config{
		LogLevel:    *logLevel,
		Development: *dev,
		Session: session.Config{
			Mode:             session.Mode(*mode),
			Provider:         *provider,
			RoomID:           *roomID,
			KeyHex:           *keyHex,
			Transport:        transport.Kind(*transportKind),
			ListenAddr:       *listenAddr,
			ForwardAddr:      *forwardAddr,
			DisplayName:      *displayName,
			SOCKSProxy:       *socksProxy,
			DNSServer:        *dnsServer,
			HandshakeTimeout: *handshakeTimeout,
			DialTimeout:      *dialTimeout,
			ReconnectDelay:   *reconnectDelay,
		},
	}

	return app.Run(cfg)
}