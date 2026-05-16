// Package routing defines the routing primitives for olcrtc-core.
// A Router examines stream metadata and returns a routing decision
// (destination address + handler tag). Unlike xray-core, this router
// operates purely on olcrtc-specific metadata; there are no GeoIP databases
// or domain-based rules required for the core routing layer.
package routing

import (
	"context"
	"fmt"
	"net"
	"sync"
)

// Destination is where a routed stream should be forwarded.
type Destination struct {
	Network string // "tcp" or "udp"
	Address string // host:port
}

// ParseDestination parses a "host:port" string with an explicit network.
func ParseDestination(network, addr string) (Destination, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return Destination{}, fmt.Errorf("parse destination %q: %w", addr, err)
	}
	if host == "" {
		return Destination{}, fmt.Errorf("destination host is empty")
	}
	if port == "" {
		return Destination{}, fmt.Errorf("destination port is empty")
	}
	return Destination{Network: network, Address: addr}, nil
}

// String returns the human-readable destination.
func (d Destination) String() string {
	return d.Network + "://" + d.Address
}

// Rule is a single routing rule.
type Rule struct {
	// Tag is the handler tag this rule forwards to.
	Tag string

	// Match returns true if this rule applies to the given metadata.
	Match func(meta map[string]string) bool

	// Destination is the outbound target for streams matched by this rule.
	Destination Destination
}

// Router resolves a routing decision from stream metadata.
type Router interface {
	Route(ctx context.Context, meta map[string]string) (Destination, string, error)
}

// StaticRouter evaluates a list of Rules in order and returns the first match.
// If no rule matches, it returns the fallback destination/tag.
type StaticRouter struct {
	mu       sync.RWMutex
	rules    []Rule
	fallback Rule
}

// NewStaticRouter creates a StaticRouter with the given fallback rule.
func NewStaticRouter(fallback Rule) *StaticRouter {
	return &StaticRouter{fallback: fallback}
}

// AddRule appends a rule to the evaluation list.
func (r *StaticRouter) AddRule(rule Rule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules = append(r.rules, rule)
}

// Route implements Router.
func (r *StaticRouter) Route(_ context.Context, meta map[string]string) (Destination, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, rule := range r.rules {
		if rule.Match != nil && rule.Match(meta) {
			return rule.Destination, rule.Tag, nil
		}
	}
	return r.fallback.Destination, r.fallback.Tag, nil
}