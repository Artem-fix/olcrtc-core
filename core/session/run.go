package session

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/openlibrecommunity/olcrtc-core/core/log"
)

// Run starts a session (client or server) from Config and blocks until
// ctx is cancelled or a fatal error occurs.
// It wraps Server/Client with optional reconnect logic.
func Run(ctx context.Context, cfg Config) error {
	if err := Validate(cfg); err != nil {
		return err
	}

	logger := log.Named("session.run").With(
		zap.String("mode", string(cfg.Mode)),
		zap.String("provider", cfg.Provider),
	)

	attempt := 0
	for {
		attempt++
		logger.Info("starting session", zap.Int("attempt", attempt))

		err := runOnce(ctx, cfg)
		if err == nil || ctx.Err() != nil {
			return err
		}

		logger.Warn("session ended with error", zap.Error(err))

		if cfg.ReconnectDelay <= 0 {
			return err
		}

		logger.Info("reconnecting", zap.Duration("delay", cfg.ReconnectDelay))
		select {
		case <-time.After(cfg.ReconnectDelay):
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during reconnect wait: %w", ctx.Err())
		}
	}
}

func runOnce(ctx context.Context, cfg Config) error {
	var component interface {
		Start(context.Context) error
		Stop(context.Context) error
	}

	switch cfg.Mode {
	case ModeServer:
		component = NewServer(cfg)
	case ModeClient:
		component = NewClient(cfg)
	default:
		return fmt.Errorf("unknown mode: %q", cfg.Mode)
	}

	if err := component.Start(ctx); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return component.Stop(stopCtx)
}