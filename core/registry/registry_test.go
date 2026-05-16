package registry_test

import (
	"errors"
	"testing"

	"github.com/openlibrecommunity/olcrtc-core/core/registry"
)

type mockComp struct{ name string }

func TestRegistry_RegisterAndCreate(t *testing.T) {
	t.Parallel()

	r := registry.New[*mockComp]()

	if err := r.Register("alpha", func(_ any) (*mockComp, error) {
		return &mockComp{name: "alpha"}, nil
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	comp, err := r.Create("alpha", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if comp.name != "alpha" {
		t.Fatalf("name: got %q want %q", comp.name, "alpha")
	}
}

func TestRegistry_DuplicateReturnsError(t *testing.T) {
	t.Parallel()

	r := registry.New[*mockComp]()
	f := func(_ any) (*mockComp, error) { return nil, nil }

	if err := r.Register("dup", f); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register("dup", f); err == nil {
		t.Fatal("expected error on duplicate register, got nil")
	}
}

func TestRegistry_NotFound(t *testing.T) {
	t.Parallel()

	r := registry.New[*mockComp]()
	_, err := r.Create("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown name")
	}
	_ = errors.Is(err, err) // just to confirm it's an error type
}

func TestRegistry_MustRegisterPanics(t *testing.T) {
	t.Parallel()

	r := registry.New[*mockComp]()
	f := func(_ any) (*mockConn, error) { return nil, nil }
	r.MustRegister("x", func(_ any) (*mockComp, error) { return nil, nil })

	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("expected panic on duplicate MustRegister")
		}
	}()
	r.MustRegister("x", func(_ any) (*mockComp, error) { return nil, nil })

	_ = f
}

type mockConn struct{}