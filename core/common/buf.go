package common

import "sync"

const defaultBufSize = 32 * 1024 // 32 KiB

// BufPool is a typed sync.Pool for byte slices of a fixed capacity.
type BufPool struct {
	pool sync.Pool
	size int
}

// NewBufPool creates a pool that returns slices of length 0, cap size.
func NewBufPool(size int) *BufPool {
	if size <= 0 {
		size = defaultBufSize
	}
	return &BufPool{
		size: size,
		pool: sync.Pool{
			New: func() any {
				b := make([]byte, size)
				return &b
			},
		},
	}
}

// Get retrieves a buffer from the pool. Callers must call Put when done.
func (p *BufPool) Get() []byte {
	bp := p.pool.Get().(*[]byte) //nolint:forcetypeassert
	return (*bp)[:p.size]
}

// Put returns a buffer to the pool. The buffer must not be used after this call.
func (p *BufPool) Put(b []byte) {
	if cap(b) < p.size {
		return
	}
	b = b[:cap(b)]
	p.pool.Put(&b)
}

// GlobalPool is a package-level buffer pool with the default size.
//
//nolint:gochecknoglobals
var GlobalPool = NewBufPool(defaultBufSize)