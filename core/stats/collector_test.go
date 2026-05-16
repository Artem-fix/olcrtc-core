package stats_test

import (
	"testing"
	"time"

	"github.com/openlibrecommunity/olcrtc-core/core/stats"
)

func TestCollector_Counters(t *testing.T) {
	t.Parallel()

	c := stats.NewCollector()

	c.AddBytesRead(100)
	c.AddBytesRead(200)
	c.AddBytesWritten(50)
	c.SessionOpened()
	c.SessionOpened()
	c.SessionClosed()
	c.RecordError()

	snap := c.Snapshot()

	if snap.BytesRead != 300 {
		t.Fatalf("BytesRead: got %d want 300", snap.BytesRead)
	}
	if snap.BytesWritten != 50 {
		t.Fatalf("BytesWritten: got %d want 50", snap.BytesWritten)
	}
	if snap.ActiveSessions != 1 {
		t.Fatalf("ActiveSessions: got %d want 1", snap.ActiveSessions)
	}
	if snap.TotalSessions != 2 {
		t.Fatalf("TotalSessions: got %d want 2", snap.TotalSessions)
	}
	if snap.Errors != 1 {
		t.Fatalf("Errors: got %d want 1", snap.Errors)
	}
	if snap.Uptime < 0 {
		t.Fatal("Uptime must be non-negative")
	}
	_ = time.Second // ensure time package is used
}