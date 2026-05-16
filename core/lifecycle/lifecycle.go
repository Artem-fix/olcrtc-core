// Package lifecycle provides context-aware start/stop primitives used by
// every long-running component in olcrtc-core.
package lifecycle

import (
	"context"
	"fmt"
	"sync"

	"github.com/openlibrecommunity/olcrtc-core/core/common"
)

// State represents the current phase of a component.
type State uint8

const (
	StateIdle    State = iota // not yet started
	StateRunning              // running normally
	StateStopped              // cleanly stopped
)

// Component is a long-running object with a managed lifecycle.
type Component interface {
	// Start begins the component's work. Blocks until the component is ready
	// to serve, then returns nil. Returns an error if startup fails.
	Start(ctx context.Context) error

	// Stop signals the component to shut down and waits for it to finish.
	// The provided context controls the maximum wait time.
	Stop(ctx context.Context) error

	// State returns the current lifecycle state.
	State() State
}

// Base provides the boilerplate for implementing Component safely.
// Embed it in your struct and delegate Start/Stop bookkeeping to it.
type Base struct {
	mu      sync.Mutex
	state   State
	cancel  context.CancelFunc
	stopped chan struct{}
}

// Begin transitions the component to StateRunning and returns a derived
// context that will be cancelled on Stop. Must be called at the top of Start.
func (b *Base) Begin(parent context.Context) (context.Context, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateRunning:
		return nil, fmt.Errorf("%w: already running", common.ErrAlreadyStarted)
	case StateStopped:
		return nil, fmt.Errorf("%w: cannot restart a stopped component", common.ErrClosed)
	}

	ctx, cancel := context.WithCancel(parent)
	b.cancel = cancel
	b.stopped = make(chan struct{})
	b.state = StateRunning
	return ctx, nil
}

// End marks the component as stopped and unblocks any callers of Stop.
// Must be deferred at the top of the goroutine launched by Start.
func (b *Base) End() {
	b.mu.Lock()
	b.state = StateStopped
	ch := b.stopped
	b.mu.Unlock()

	if ch != nil {
		close(ch)
	}
}

// Shutdown cancels the running context and waits for End to be called,
// or until ctx expires.
func (b *Base) Shutdown(ctx context.Context) error {
	b.mu.Lock()
	state := b.state
	cancel := b.cancel
	stopped := b.stopped
	b.mu.Unlock()

	if state != StateRunning {
		return nil
	}
	if cancel != nil {
		cancel()
	}

	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop timed out: %w", common.ErrTimeout)
	}
}

// State returns the current state.
func (b *Base) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}