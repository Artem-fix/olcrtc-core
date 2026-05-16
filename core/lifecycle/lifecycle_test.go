package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/openlibrecommunity/olcrtc-core/core/lifecycle"
)

// testComponent is a minimal Component using lifecycle.Base.
type testComponent struct {
	lifecycle.Base
	started chan struct{}
}

func (c *testComponent) Start(parent context.Context) error {
	ctx, err := c.Begin(parent)
	if err != nil {
		return err
	}
	close(c.started)
	go func() {
		defer c.End()
		<-ctx.Done()
	}()
	return nil
}

func (c *testComponent) Stop(ctx context.Context) error {
	return c.Shutdown(ctx)
}

func TestLifecycle_StartStop(t *testing.T) {
	t.Parallel()

	comp := &testComponent{started: make(chan struct{})}
	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-comp.started:
	case <-time.After(time.Second):
		t.Fatal("component did not start within 1s")
	}

	if comp.State() != lifecycle.StateRunning {
		t.Fatalf("expected StateRunning, got %v", comp.State())
	}

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := comp.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if comp.State() != lifecycle.StateStopped {
		t.Fatalf("expected StateStopped, got %v", comp.State())
	}
}

func TestLifecycle_DoubleStart(t *testing.T) {
	t.Parallel()

	comp := &testComponent{started: make(chan struct{})}
	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	<-comp.started

	if err := comp.Start(ctx); err == nil {
		t.Fatal("expected error on double Start, got nil")
	}

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = comp.Stop(stopCtx)
}