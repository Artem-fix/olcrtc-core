package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/openlibrecommunity/olcrtc-core/core/crypto"
	"github.com/openlibrecommunity/olcrtc-core/core/lifecycle"
	"github.com/openlibrecommunity/olcrtc-core/core/log"
	"github.com/openlibrecommunity/olcrtc-core/core/mux"
	"github.com/openlibrecommunity/olcrtc-core/core/provider"
	"github.com/openlibrecommunity/olcrtc-core/core/transport"
	"github.com/openlibrecommunity/olcrtc-core/core/transport/datachannel"
)

// Client implements the client-side session lifecycle.
type Client struct {
	lifecycle.Base
	cfg    Config
	logger *zap.Logger

	mu      sync.Mutex
	muxSess *mux.Session
}

// NewClient creates a Client.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:    cfg,
		logger: log.Named("session.client"),
	}
}

// Start connects to the provider and opens a mux session, then begins
// listening for local connections on ListenAddr.
func (c *Client) Start(parent context.Context) error {
	ctx, err := c.Begin(parent)
	if err != nil {
		return err
	}
	go func() {
		defer c.End()
		if runErr := c.run(ctx); runErr != nil {
			c.logger.Error("client run error", zap.Error(runErr))
		}
	}()
	return nil
}

// Stop gracefully shuts down the client.
func (c *Client) Stop(ctx context.Context) error {
	return c.Shutdown(ctx)
}

func (c *Client) run(ctx context.Context) error {
	masterKey, err := crypto.KeyFromHex(c.cfg.KeyHex)
	if err != nil {
		return fmt.Errorf("parse key: %w", err)
	}

	// Connect to provider.
	prov, err := provider.Create(c.cfg.Provider, provider.Config{})
	if err != nil {
		return fmt.Errorf("create provider %q: %w", c.cfg.Provider, err)
	}
	defer func() { _ = prov.Close() }()

	localPeer, err := prov.Join(ctx, provider.RoomConfig{
		RoomID:      c.cfg.RoomID,
		DisplayName: c.cfg.DisplayName,
		SOCKSProxy:  c.cfg.SOCKSProxy,
	})
	if err != nil {
		return fmt.Errorf("join room %q: %w", c.cfg.RoomID, err)
	}
	defer func() { _ = localPeer.Close() }()

	c.logger.Info("joined room", zap.String("room_id", c.cfg.RoomID))

	// Dial transport.
	dialTimeout := c.cfg.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 30 * time.Second
	}
	dialCtx, dialCancel := context.WithTimeout(ctx, dialTimeout)
	defer dialCancel()

	tFactory := &datachannel.Factory{}
	t, err := tFactory.NewDialer().Dial(dialCtx, localPeer, transport.Config{Kind: c.cfg.Transport})
	if err != nil {
		return fmt.Errorf("dial transport: %w", err)
	}
	defer func() { _ = t.Close() }()

	// Handshake.
	hsTimeout := c.cfg.HandshakeTimeout
	if hsTimeout == 0 {
		hsTimeout = defaultHandshakeTimeout
	}
	hsResult, err := ClientHandshake(ctx, t, masterKey, hsTimeout)
	if err != nil {
		return fmt.Errorf("handshake: %w", err)
	}

	// Wrap with AEAD.
	encConn, err := newEncryptedConn(t, hsResult.sendKey, hsResult.recvKey)
	if err != nil {
		return fmt.Errorf("encrypted conn: %w", err)
	}
	defer func() { _ = encConn.Close() }()

	// Open smux client session.
	sess, err := mux.NewClientSession(t)
	if err != nil {
		return fmt.Errorf("mux client session: %w", err)
	}
	c.mu.Lock()
	c.muxSess = sess
	c.mu.Unlock()
	defer func() { _ = sess.Close() }()

	c.logger.Info("mux session established")

	// Start local listener.
	ln, err := net.Listen("tcp", c.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", c.cfg.ListenAddr, err)
	}
	defer func() { _ = ln.Close() }()

	c.logger.Info("listening", zap.String("addr", c.cfg.ListenAddr))

	// Accept local connections and multiplex them over the session.
	var wg sync.WaitGroup
	for {
		local, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			c.logger.Warn("accept local conn", zap.Error(err))
			break
		}
		wg.Add(1)
		go func(lc net.Conn) {
			defer wg.Done()
			c.proxyConn(ctx, lc, sess)
		}(local)
	}
	wg.Wait()
	return nil
}

func (c *Client) proxyConn(ctx context.Context, local net.Conn, sess *mux.Session) {
	defer func() { _ = local.Close() }()

	remote, err := sess.OpenStream()
	if err != nil {
		c.logger.Warn("open mux stream", zap.Error(err))
		return
	}
	defer func() { _ = remote.Close() }()

	sent, recv, err := mux.Pipe(ctx, local, remote)
	c.logger.Debug("connection closed",
		zap.Int64("bytes_sent", sent),
		zap.Int64("bytes_recv", recv),
		zap.Error(err),
	)
}

// ensure encryptedConn satisfies transport.Transport constraints indirectly.
var _ io.ReadWriter = (*encryptedConn)(nil)