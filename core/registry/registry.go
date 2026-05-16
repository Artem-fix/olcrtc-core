// Package registry provides a generic, thread-safe component registry.
//
// Inspired by xray-core's feature registration model but decoupled from any
// specific protocol. Components register under a string key; the registry
// resolves them at runtime.
package registry

import (
	"fmt"
	"sync"
)

// Factory is a function that creates a new instance of a component.
type Factory[T any] func(cfg any) (T, error)

// Registry stores named factories and creates instances on demand.
type Registry[T any] struct {
	mu        sync.RWMutex
	factories map[string]Factory[T]
}

// New returns an empty registry.
func New[T any]() *Registry[T] {
	return &Registry[T]{
		factories: make(map[string]Factory[T]),
	}
}

// Register adds a factory under the given name.
// Returns an error if the name is already registered.
func (r *Registry[T]) Register(name string, f Factory[T]) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("registry: %q is already registered", name)
	}
	r.factories[name] = f
	return nil
}

// MustRegister is like Register but panics on duplicate name.
func (r *Registry[T]) MustRegister(name string, f Factory[T]) {
	if err := r.Register(name, f); err != nil {
		panic(err)
	}
}

// Create constructs a component by name using the registered factory.
func (r *Registry[T]) Create(name string, cfg any) (T, error) {
	r.mu.RLock()
	f, ok := r.factories[name]
	r.mu.RUnlock()

	var zero T
	if !ok {
		return zero, fmt.Errorf("registry: %q not found", name)
	}
	return f(cfg)
}

// Names returns all registered names, sorted alphabetically.
func (r *Registry[T]) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for n := range r.factories {
		names = append(names, n)
	}
	return names
}