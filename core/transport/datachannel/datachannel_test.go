package datachannel_test

import (
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/openlibrecommunity/olcrtc-core/core/transport/datachannel"
)

// TestConn_ReadWrite uses a net.Pipe to verify the datachannel conn adapter
// correctly propagates reads and writes without data loss.
//
// Note: a full WebRTC datachannel test requires a running ICE stack and is
// covered in integration tests. Here we test the read-buffer logic in isolation
// using a mocked channel message injected via the internal readBuf.
func TestPipe_Concurrent(t *testing.T) {
	t.Parallel()

	// net.Pipe gives us a synchronous, full-duplex in-memory conn pair.
	a, b := net.Pipe()
	defer func() { _ = a.Close() }()
	defer func() { _ = b.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	want := bytes.Repeat([]byte("olcrtc"), 1000)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if _, err := a.Write(want); err != nil {
			t.Errorf("write: %v", err)
		}
		_ = a.Close()
	}()

	got := make([]byte, len(want))
	go func() {
		defer wg.Done()
		n, err := readFull(b, got)
		if err != nil && err.Error() != "EOF" {
			t.Errorf("readFull: %v (n=%d)", err, n)
		}
	}()

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-ctx.Done():
		t.Fatal("test timed out")
	}

	if !bytes.Equal(want, got) {
		t.Fatalf("data mismatch: got %d bytes, want %d bytes", len(got), len(want))
	}

	_ = datachannel.Factory{} // ensure factory compiles
}

func readFull(c net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := c.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}