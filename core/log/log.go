// Package log wraps uber-go/zap for structured, levelled logging across olcrtc-core.
package log

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Level mirrors zapcore.Level for external callers who shouldn't import zap directly.
type Level = zapcore.Level

// Re-export the common levels.
const (
	DebugLevel = zapcore.DebugLevel
	InfoLevel  = zapcore.InfoLevel
	WarnLevel  = zapcore.WarnLevel
	ErrorLevel = zapcore.ErrorLevel
)

// Logger is the package-wide logger. Replaced by Init.
//
//nolint:gochecknoglobals
var Logger *zap.Logger = zap.NewNop()

// Init initialises the global logger. Must be called before any component is started.
func Init(level Level, development bool) error {
	var cfg zap.Config
	if development {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}
	cfg.Level = zap.NewAtomicLevelAt(level)

	l, err := cfg.Build(zap.AddCallerSkip(0))
	if err != nil {
		return fmt.Errorf("build logger: %w", err)
	}
	Logger = l
	return nil
}

// With creates a child logger with additional fields.
func With(fields ...zap.Field) *zap.Logger {
	return Logger.With(fields...)
}

// Named returns a named child logger.
func Named(name string) *zap.Logger {
	return Logger.Named(name)
}

// Sync flushes any buffered log entries.
func Sync() {
	_ = Logger.Sync()
}