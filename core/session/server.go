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

// Server implements the server-side session lifecycle.
type Server struct {
	lifecycle.Base
	cfg    Config
	logger *zap.Logger
}

// NewServer creates a Server.
func NewServer(cfg Config) *Server {
	return &Server{
		cfg:    cfg,
		logger: log.Named("session.server"),
	}
}

// Start begins accepting client connections and proxying traffic.
func (s *Server) Start(parent context.Context) error {
	ctx, err := s.Begin(parent)
	if err != nil {
		return err
	}
	go func() {
		defer s.End()
		if runErr := s.run(ctx); runErr != nil {
			s.logger.Error("server run error", zap.Error(runErr))
		}
	}()
	return nil
}

// Stop gracefully stops the server.
func (s *Server) Stop(ctx context.Context) error {
	return s.Shutdown(ctx)
}

func (s *Server) run(ctx context.Context) error {
	masterKey, err := crypto.KeyFromHex(s.cfg.KeyHex)
	if err != nil {
		return fmt.Errorf("parse key: %w", err)
	}

	prov, err := provider.Create(s.cfg.Provider, provider.Config{
		Handler: func(pctx context.Context, peer provider.Peer) {
			go s.handlePeer(pctx, peer, masterKey)
		},
	})
	if err != nil {
		return fmt.Errorf("create provider %q: %w", s.cfg.Provider, err)
	}
	defer func() { _ = prov.Close() }()

	roomCfg := provider.RoomConfig{
		RoomID:      s.cfg.RoomID,
		DisplayName: s.cfg.DisplayName,
		SOCKSProxy:  s.cfg.SOCKSProxy,
	}

	localPeer, err := prov.Join(ctx, roomCfg)
	if err != nil {
		return fmt.Errorf("join room: %w", err)
	}
	defer func() { _ = localPeer.Close() }()

	s.logger.Info("server ready", zap.String("room_id", prov.RoomID()))

	<-ctx.Done()
	return nil
}

func (s *Server) handlePeer(ctx context.Context, peer provider.Peer, masterKey crypto.Key) {
	logger := s.logger.With(zap.String("peer_id", peer.ID()))
	logger.Info("new peer connected")

	// Set up transport listener.
	tFactory := &datachannel.Factory{}
	listener, err := tFactory.NewListener(peer, transport.Config{Kind: s.cfg.Transport})
	if err != nil {
		logger.Error("create listener", zap.Error(err))
		return
	}
	defer func() { _ = listener.Close() }()

	// Accept the transport connection.
	acceptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	t, err := listener.Accept(acceptCtx)
	if err != nil {
		logger.Error("accept transport", zap.Error(err))
		return
	}

	// Handshake.
	hsTimeout := s.cfg.HandshakeTimeout
	if hsTimeout == 0 {
		hsTimeout = defaultHandshakeTimeout
	}
	hsResult, err := ServerHandshake(ctx, t, masterKey, hsTimeout)
	if err != nil {
		logger.Error("handshake failed", zap.Error(err))
		_ = t.Close()
		return
	}

	// Wrap transport with AEAD.
	encConn, err := newEncryptedConn(t, hsResult.sendKey, hsResult.recvKey)
	if err != nil {
		logger.Error("create encrypted conn", zap.Error(err))
		_ = t.Close()
		return
	}

	// Start smux server session.
	muxSess, err := mux.NewServerSession(t)
	if err != nil {
		logger.Error("mux server session", zap.Error(err))
		_ = encConn.Close()
		return
	}
	defer func() { _ = muxSess.Close() }()

	logger.Info("peer session established")

	var wg sync.WaitGroup
	for {
		stream, err := muxSess.AcceptStream(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			logger.Warn("accept stream", zap.Error(err))
			break
		}
		wg.Add(1)
		go func(st net.Conn) {
			defer wg.Done()
			s.forwardStream(ctx, st, logger)
		}(stream)
	}
	wg.Wait()
	logger.Info("peer session closed")
}

func (s *Server) forwardStream(ctx context.Context, stream net.Conn, logger *zap.Logger) {
	defer func() { _ = stream.Close() }()

	remote, err := net.DialTimeout("tcp", s.cfg.ForwardAddr, 10*time.Second)
	if err != nil {
		logger.Warn("dial forward addr", zap.String("addr", s.cfg.ForwardAddr), zap.Error(err))
		return
	}
	defer func() { _ = remote.Close() }()

	sent, recv, err := mux.Pipe(ctx, stream, remote)
	logger.Debug("stream closed",
		zap.Int64("bytes_sent", sent),
		zap.Int64("bytes_recv", recv),
		zap.Error(err),
	)
}

// ensure encryptedConn satisfies io.ReadWriter (used for mux).
var _ io.ReadWriter = (*encryptedConn)(nil)