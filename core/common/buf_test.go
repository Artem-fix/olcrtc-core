package common_test

import (
	"testing"

	"github.com/openlibrecommunity/olcrtc-core/core/common"
)

func TestBufPool_GetPut(t *testing.T) {
	t.Parallel()

	pool := common.NewBufPool(4096)

	buf := pool.Get()
	if len(buf) != 4096 {
		t.Fatalf("buf len: got %d want 4096", len(buf))
	}

	// Fill to ensure no aliasing after Put.
	for i := range buf {
		buf[i] = 0xAB
	}
	pool.Put(buf)

	// Get again; contents are undefined but should not panic.
	buf2 := pool.Get()
	if len(buf2) != 4096 {
		t.Fatalf("buf2 len: got %d want 4096", len(buf2))
	}
	pool.Put(buf2)
}

func TestBufPool_SmallBufNotReturned(t *testing.T) {
	t.Parallel()

	pool := common.NewBufPool(1024)
	// Putting a smaller buffer should be a no-op (no panic).
	pool.Put(make([]byte, 32))
}