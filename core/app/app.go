// Package app is the top-level wiring layer of olcrtc-core.
// It instantiates and connects all subsystems (logging, provider registry,
// transport registry, dispatcher, session) in the correct order, and
// provides graceful startup and shutdown.
package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/openlibrecommunity/olcrtc-core/core/log"
	"github.com/openlibrecommunity/olcrtc-core/core/session"
)

const shutdownTimeout = 15 * time.Second

// Config holds top-level application configuration.
type Config struct {
	Session session.Config

	// LogLevel controls verbosity: "debug", "info", "warn", "error".
	LogLevel string

	// Development enables human-readable (coloured) log output.
	Development bool
}

// Run starts the application and blocks until a signal is received or a fatal
// error occurs. It handles SIGINT and SIGTERM for graceful shutdown.
func Run(cfg Config) error {
	if err := initLogging(cfg.LogLevel, cfg.Development); err != nil {
		return err
	}
	defer log.Sync()

	if err := session.Validate(cfg.Session); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Catch OS signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start the appropriate session mode.
	var component interface {
		Start(context.Context) error
		Stop(context.Context) error
	}

	switch cfg.Session.Mode {
	case session.ModeServer:
		component = session.NewServer(cfg.Session)
	case session.ModeClient:
		component = session.NewClient(cfg.Session)
	default:
		return fmt.Errorf("unknown mode: %q", cfg.Session.Mode)
	}

	if err := component.Start(ctx); err != nil {
		return fmt.Errorf("start %s: %w", cfg.Session.Mode, err)
	}
	log.Logger.Info("olcrtc-core started", zap.String("mode", string(cfg.Session.Mode)))

	// Wait for signal or context cancellation.
	select {
	case sig := <-sigCh:
		log.Logger.Info("signal received, shutting down", zap.String("signal", sig.String()))
	case <-ctx.Done():
		log.Logger.Info("context cancelled, shutting down")
	}

	// Graceful shutdown.
	cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer stopCancel()

	if err := component.Stop(stopCtx); err != nil {
		log.Logger.Error("shutdown error", zap.Error(err))
		return err
	}
	log.Logger.Info("olcrtc-core stopped cleanly")
	return nil
}

func initLogging(level string, development bool) error {
	var lvl log.Level
	switch level {
	case "debug":
		lvl = log.DebugLevel
	case "warn":
		lvl = log.WarnLevel
	case "error":
		lvl = log.ErrorLevel
	default:
		lvl = log.InfoLevel
	}
	return log.Init(lvl, development)
}