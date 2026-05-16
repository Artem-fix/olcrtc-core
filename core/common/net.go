package common

import (
	"net"
	"time"
)

// Connection wraps net.Conn with olcrtc metadata.
type Connection interface {
	net.Conn

	// ID returns a unique identifier for this connection.
	ID() string

	// Metadata returns arbitrary key-value pairs attached to this connection.
	Metadata() map[string]string
}

// Address represents a network address with transport hints.
type Address struct {
	Network  string
	Host     string
	Port     uint16
	Metadata map[string]string
}

// ConnStats holds runtime statistics for a connection.
type ConnStats struct {
	BytesRead    uint64
	BytesWritten uint64
	PacketsSent  uint64
	PacketsRecv  uint64
	Latency      time.Duration
	OpenedAt     time.Time
}