package provider

import (
	"fmt"
	"sync"
)

// Factory creates a Provider from Config.
type Factory func(cfg Config) (Provider, error)

//nolint:gochecknoglobals
var globalRegistry = &providerRegistry{
	factories: make(map[string]Factory),
}

type providerRegistry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// Register adds a provider factory under name.
// Panics on duplicate to catch mis-configuration at init time.
func Register(name string, f Factory) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	if _, exists := globalRegistry.factories[name]; exists {
		panic(fmt.Sprintf("provider: %q already registered", name))
	}
	globalRegistry.factories[name] = f
}

// Create instantiates a registered provider by name.
func Create(name string, cfg Config) (Provider, error) {
	globalRegistry.mu.RLock()
	f, ok := globalRegistry.factories[name]
	globalRegistry.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider %q not found", name)
	}
	return f(cfg)
}

// Registered returns the names of all registered providers.
func Registered() []string {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	names := make([]string, 0, len(globalRegistry.factories))
	for n := range globalRegistry.factories {
		names = append(names, n)
	}
	return names
}