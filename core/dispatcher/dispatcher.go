// Package dispatcher implements the central routing engine for olcrtc-core.
//
// Architecture mirrors xray-core's dispatcher concept but is stripped of all
// proxy protocols and transports. The Dispatcher receives inbound Streams,
// classifies them by an olcrtc-specific routing decision, and hands them to
// the appropriate outbound Handler.
package dispatcher

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/openlibrecommunity/olcrtc-core/core/common"
	"github.com/openlibrecommunity/olcrtc-core/core/log"
	"github.com/openlibrecommunity/olcrtc-core/core/routing"
)

// Stream is an accepted inbound logical stream with its resolved destination.
type Stream struct {
	Conn net.Conn

	// Destination is where this stream should be forwarded.
	Destination routing.Destination

	// Metadata carries per-stream tags for logging and routing decisions.
	Metadata map[string]string
}

// Handler processes a fully routed stream.
type Handler interface {
	Handle(ctx context.Context, stream Stream) error
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc func(ctx context.Context, stream Stream) error

// Handle implements Handler.
func (f HandlerFunc) Handle(ctx context.Context, stream Stream) error {
	return f(ctx, stream)
}

// Dispatcher routes inbound streams to outbound handlers.
type Dispatcher struct {
	router   routing.Router
	handlers map[string]Handler
	mu       sync.RWMutex
	inflight atomic.Int64
	logger   *zap.Logger
	closed   chan struct{}
	once     sync.Once
}

// New creates a Dispatcher with the given router.
func New(router routing.Router) *Dispatcher {
	return &Dispatcher{
		router:   router,
		handlers: make(map[string]Handler),
		logger:   log.Named("dispatcher"),
		closed:   make(chan struct{}),
	}
}

// RegisterHandler binds a tag to a Handler.
func (d *Dispatcher) RegisterHandler(tag string, h Handler) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.handlers[tag]; exists {
		return fmt.Errorf("dispatcher: handler %q already registered", tag)
	}
	d.handlers[tag] = h
	d.logger.Info("handler registered", zap.String("tag", tag))
	return nil
}

// Dispatch routes stream to the appropriate handler based on routing rules.
// It is safe to call from multiple goroutines concurrently.
func (d *Dispatcher) Dispatch(ctx context.Context, conn net.Conn, meta map[string]string) error {
	select {
	case <-d.closed:
		_ = conn.Close()
		return common.ErrClosed
	default:
	}

	dest, tag, err := d.router.Route(ctx, meta)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("route: %w", err)
	}

	d.mu.RLock()
	h, ok := d.handlers[tag]
	d.mu.RUnlock()

	if !ok {
		_ = conn.Close()
		return fmt.Errorf("dispatcher: no handler for tag %q", tag)
	}

	stream := Stream{
		Conn:        conn,
		Destination: dest,
		Metadata:    meta,
	}

	d.inflight.Add(1)
	go func() {
		defer d.inflight.Add(-1)
		if herr := h.Handle(ctx, stream); herr != nil {
			d.logger.Warn("handler error",
				zap.String("tag", tag),
				zap.Error(herr),
			)
		}
	}()

	return nil
}

// Close drains in-flight streams and shuts down the dispatcher.
func (d *Dispatcher) Close() error {
	d.once.Do(func() { close(d.closed) })
	return nil
}

// Inflight returns the number of currently active streams.
func (d *Dispatcher) Inflight() int64 {
	return d.inflight.Load()
}