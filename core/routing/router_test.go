package routing_test

import (
	"context"
	"testing"

	"github.com/openlibrecommunity/olcrtc-core/core/routing"
)

func TestStaticRouter_FallbackUsedWhenNoMatch(t *testing.T) {
	t.Parallel()

	fallback := routing.Rule{
		Tag:         "direct",
		Destination: routing.Destination{Network: "tcp", Address: "127.0.0.1:80"},
	}
	router := routing.NewStaticRouter(fallback)

	dest, tag, err := router.Route(context.Background(), map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if tag != "direct" {
		t.Fatalf("tag: got %q want %q", tag, "direct")
	}
	if dest.Address != "127.0.0.1:80" {
		t.Fatalf("dest: got %q", dest.Address)
	}
}

func TestStaticRouter_FirstMatchWins(t *testing.T) {
	t.Parallel()

	fallback := routing.Rule{Tag: "fallback", Destination: routing.Destination{Network: "tcp", Address: "0.0.0.0:0"}}
	router := routing.NewStaticRouter(fallback)

	router.AddRule(routing.Rule{
		Tag:         "a",
		Destination: routing.Destination{Network: "tcp", Address: "10.0.0.1:8080"},
		Match:       func(m map[string]string) bool { return m["type"] == "a" },
	})
	router.AddRule(routing.Rule{
		Tag:         "b",
		Destination: routing.Destination{Network: "tcp", Address: "10.0.0.2:9090"},
		Match:       func(m map[string]string) bool { return m["type"] == "b" },
	})

	_, tagA, _ := router.Route(context.Background(), map[string]string{"type": "a"})
	if tagA != "a" {
		t.Fatalf("expected tag 'a', got %q", tagA)
	}

	_, tagB, _ := router.Route(context.Background(), map[string]string{"type": "b"})
	if tagB != "b" {
		t.Fatalf("expected tag 'b', got %q", tagB)
	}

	_, tagFB, _ := router.Route(context.Background(), map[string]string{"type": "unknown"})
	if tagFB != "fallback" {
		t.Fatalf("expected tag 'fallback', got %q", tagFB)
	}
}