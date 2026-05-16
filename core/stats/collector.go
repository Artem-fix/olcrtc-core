// Package stats provides thread-safe runtime metrics collection.
package stats

import (
	"sync/atomic"
	"time"
)

// Snapshot is a point-in-time copy of collected metrics.
type Snapshot struct {
	BytesRead      uint64
	BytesWritten   uint64
	ActiveSessions int64
	TotalSessions  uint64
	Errors         uint64
	Uptime         time.Duration
}

// Collector accumulates runtime metrics.
type Collector struct {
	bytesRead      atomic.Uint64
	bytesWritten   atomic.Uint64
	activeSessions atomic.Int64
	totalSessions  atomic.Uint64
	errors         atomic.Uint64
	startTime      time.Time
}

// NewCollector creates a Collector with the start time set to now.
func NewCollector() *Collector {
	return &Collector{startTime: time.Now()}
}

// AddBytesRead records n bytes read.
func (c *Collector) AddBytesRead(n uint64) { c.bytesRead.Add(n) }

// AddBytesWritten records n bytes written.
func (c *Collector) AddBytesWritten(n uint64) { c.bytesWritten.Add(n) }

// SessionOpened records a new session.
func (c *Collector) SessionOpened() {
	c.activeSessions.Add(1)
	c.totalSessions.Add(1)
}

// SessionClosed records a closed session.
func (c *Collector) SessionClosed() { c.activeSessions.Add(-1) }

// RecordError records an error event.
func (c *Collector) RecordError() { c.errors.Add(1) }

// Snapshot returns a point-in-time copy of all counters.
func (c *Collector) Snapshot() Snapshot {
	return Snapshot{
		BytesRead:      c.bytesRead.Load(),
		BytesWritten:   c.bytesWritten.Load(),
		ActiveSessions: c.activeSessions.Load(),
		TotalSessions:  c.totalSessions.Load(),
		Errors:         c.errors.Load(),
		Uptime:         time.Since(c.startTime),
	}
}