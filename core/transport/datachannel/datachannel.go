// Package datachannel implements the DataChannel transport for olcrtc-core.
// It tunnels arbitrary byte streams over WebRTC data channels.
package datachannel

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
	"go.uber.org/zap"

	"github.com/openlibrecommunity/olcrtc-core/core/log"
	"github.com/openlibrecommunity/olcrtc-core/core/provider"
	"github.com/openlibrecommunity/olcrtc-core/core/transport"
)

const (
	defaultLabel   = "olcrtc-data"
	defaultMTU     = 65535
	dialTimeout    = 30 * time.Second
	acceptTimeout  = 60 * time.Second
)

// conn is a net.Conn backed by a WebRTC DataChannel.
type conn struct {
	dc       *webrtc.DataChannel
	id       string
	kind     transport.Kind
	readBuf  chan []byte
	pending  []byte // leftover bytes from the last Read

	closeOnce sync.Once
	closed    chan struct{}

	bytesRead    atomic.Uint64
	bytesWritten atomic.Uint64
	packetsSent  atomic.Uint64
	packetsRecv  atomic.Uint64

	readDeadline  atomic.Value // time.Time
	writeDeadline atomic.Value // time.Time
}

func newConn(dc *webrtc.DataChannel) *conn {
	c := &conn{
		dc:      dc,
		id:      uuid.NewString(),
		kind:    transport.KindDataChannel,
		readBuf: make(chan []byte, 256),
		closed:  make(chan struct{}),
	}

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		buf := make([]byte, len(msg.Data))
		copy(buf, msg.Data)
		select {
		case c.readBuf <- buf:
			c.packetsRecv.Add(1)
		case <-c.closed:
		}
	})
	dc.OnClose(func() { c.closeOnce.Do(func() { close(c.closed) }) })
	dc.OnError(func(_ error) { c.closeOnce.Do(func() { close(c.closed) }) })

	return c
}

// Kind implements transport.Transport.
func (c *conn) Kind() transport.Kind { return c.kind }

// Stats implements transport.Transport.
func (c *conn) Stats() transport.Stats {
	return transport.Stats{
		BytesRead:    c.bytesRead.Load(),
		BytesWritten: c.bytesWritten.Load(),
		PacketsSent:  c.packetsSent.Load(),
		PacketsRecv:  c.packetsRecv.Load(),
	}
}

// Read implements net.Conn.
func (c *conn) Read(b []byte) (int, error) {
	if len(c.pending) > 0 {
		n := copy(b, c.pending)
		c.pending = c.pending[n:]
		c.bytesRead.Add(uint64(n))
		return n, nil
	}

	deadline, _ := c.readDeadline.Load().(time.Time)
	var timer <-chan time.Time
	if !deadline.IsZero() {
		timer = time.After(time.Until(deadline))
	}

	select {
	case buf, ok := <-c.readBuf:
		if !ok {
			return 0, io.EOF
		}
		n := copy(b, buf)
		if n < len(buf) {
			c.pending = buf[n:]
		}
		c.bytesRead.Add(uint64(n))
		return n, nil
	case <-c.closed:
		return 0, net.ErrClosed
	case <-timer:
		return 0, fmt.Errorf("read: %w", context.DeadlineExceeded)
	}
}

// Write implements net.Conn.
func (c *conn) Write(b []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	default:
	}

	if err := c.dc.Send(b); err != nil {
		return 0, fmt.Errorf("datachannel send: %w", err)
	}
	c.bytesWritten.Add(uint64(len(b)))
	c.packetsSent.Add(1)
	return len(b), nil
}

// Close implements net.Conn.
func (c *conn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.dc.Close()
}

// LocalAddr implements net.Conn.
func (c *conn) LocalAddr() net.Addr { return addr("datachannel:local") }

// RemoteAddr implements net.Conn.
func (c *conn) RemoteAddr() net.Addr { return addr("datachannel:remote") }

// SetDeadline implements net.Conn.
func (c *conn) SetDeadline(t time.Time) error {
	c.readDeadline.Store(t)
	c.writeDeadline.Store(t)
	return nil
}

// SetReadDeadline implements net.Conn.
func (c *conn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Store(t)
	return nil
}

// SetWriteDeadline implements net.Conn.
func (c *conn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline.Store(t)
	return nil
}

// ReadFrom implements io.ReaderFrom (zero-copy from a Reader into the DC).
func (c *conn) ReadFrom(r io.Reader) (int64, error) {
	buf := make([]byte, defaultMTU)
	var total int64
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := c.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
		}
		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
	}
}

// WriteTo implements io.WriterTo (zero-copy from the DC into a Writer).
func (c *conn) WriteTo(w io.Writer) (int64, error) {
	buf := make([]byte, defaultMTU)
	var total int64
	for {
		n, err := c.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
		}
		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
	}
}

// addr is a trivial net.Addr implementation.
type addr string

func (a addr) Network() string { return "olcrtc" }
func (a addr) String() string  { return string(a) }

// ---- Dialer ----

// Dialer opens outbound DataChannel transports.
type Dialer struct{}

// Dial creates a new labelled DataChannel on the peer's PeerConnection.
func (d *Dialer) Dial(ctx context.Context, peer provider.Peer, cfg transport.Config) (transport.Transport, error) {
	label := defaultLabel
	if l, ok := cfg.Extra["label"].(string); ok && l != "" {
		label = l
	}

	ordered := true
	dc, err := peer.PeerConnection().CreateDataChannel(label, &webrtc.DataChannelInit{
		Ordered: &ordered,
	})
	if err != nil {
		return nil, fmt.Errorf("create data channel %q: %w", label, err)
	}

	c := newConn(dc)

	// Wait for the DataChannel to open.
	openCh := make(chan struct{}, 1)
	dc.OnOpen(func() {
		select {
		case openCh <- struct{}{}:
		default:
		}
	})

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	select {
	case <-openCh:
		log.Named("datachannel.dialer").Info("data channel opened", zap.String("label", label))
		return c, nil
	case <-dialCtx.Done():
		_ = dc.Close()
		return nil, fmt.Errorf("dial timeout waiting for data channel %q: %w", label, dialCtx.Err())
	case <-peer.Done():
		_ = dc.Close()
		return nil, fmt.Errorf("peer disconnected before data channel %q opened", label)
	}
}

// ---- Listener ----

// Listener accepts inbound DataChannel transports from a remote peer.
type Listener struct {
	queue  chan transport.Transport
	closed chan struct{}
	once   sync.Once
}

// NewListener creates a Listener that receives all DataChannels opened by peer.
func NewListener(peer provider.Peer, _ transport.Config) (*Listener, error) {
	l := &Listener{
		queue:  make(chan transport.Transport, 64),
		closed: make(chan struct{}),
	}

	peer.PeerConnection().OnDataChannel(func(dc *webrtc.DataChannel) {
		c := newConn(dc)
		dc.OnOpen(func() {
			select {
			case l.queue <- c:
			case <-l.closed:
			}
		})
	})

	return l, nil
}

// Accept waits for the next inbound transport.
func (l *Listener) Accept(ctx context.Context) (transport.Transport, error) {
	select {
	case t, ok := <-l.queue:
		if !ok {
			return nil, net.ErrClosed
		}
		return t, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

// Close stops the listener.
func (l *Listener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

// ---- Factory ----

// Factory is the DataChannel transport factory, satisfies transport.Factory.
type Factory struct{}

// Kind returns KindDataChannel.
func (f *Factory) Kind() transport.Kind { return transport.KindDataChannel }

// NewDialer returns a DataChannel Dialer.
func (f *Factory) NewDialer() transport.Dialer { return &Dialer{} }

// NewListener wraps NewListener.
func (f *Factory) NewListener(peer provider.Peer, cfg transport.Config) (transport.Listener, error) {
	return NewListener(peer, cfg)
}