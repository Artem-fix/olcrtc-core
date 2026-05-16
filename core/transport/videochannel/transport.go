package videochannel

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"go.uber.org/zap"

	"github.com/openlibrecommunity/olcrtc-core/core/log"
	"github.com/openlibrecommunity/olcrtc-core/core/provider"
	"github.com/openlibrecommunity/olcrtc-core/core/transport"
)

const (
	// videoWidth and videoHeight define the synthetic frame dimensions.
	// 320×240 gives a 76 800-byte luma plane → ≈64 KiB payload capacity.
	videoWidth  = 320
	videoHeight = 240
	lumaSize    = videoWidth * videoHeight

	frameQueueSize = 256
	rtpPayloadType = 102 // dynamic PT for H.264
)

// conn is a net.Conn backed by a WebRTC video track (sender/receiver pair).
type conn struct {
	id   string
	kind transport.Kind

	// sender is non-nil on the writer side (sends RTP packets).
	sender *webrtc.RTPSender
	// track is the local track we write synthetic video frames into.
	track *webrtc.TrackLocalStaticRTP

	// receiver is non-nil on the reader side (receives RTP packets).
	receiver *webrtc.RTPReceiver

	readQueue chan []byte // decoded payload chunks
	pending   []byte     // leftover bytes from the previous Read

	closeOnce sync.Once
	closed    chan struct{}

	seq uint32 // outgoing frame sequence counter (atomic-like via lock)
	mu  sync.Mutex

	bytesRead    atomic.Uint64
	bytesWritten atomic.Uint64
	packetsSent  atomic.Uint64
	packetsRecv  atomic.Uint64

	readDeadline  atomic.Value // time.Time
	writeDeadline atomic.Value // time.Time

	logger *zap.Logger
}

func newSenderConn(track *webrtc.TrackLocalStaticRTP, sender *webrtc.RTPSender) *conn {
	return &conn{
		id:        uuid.NewString(),
		kind:      transport.KindVideoChannel,
		track:     track,
		sender:    sender,
		readQueue: make(chan []byte, frameQueueSize),
		closed:    make(chan struct{}),
		logger:    log.Named("videochannel.sender"),
	}
}

func newReceiverConn(receiver *webrtc.RTPReceiver) *conn {
	c := &conn{
		id:        uuid.NewString(),
		kind:      transport.KindVideoChannel,
		receiver:  receiver,
		readQueue: make(chan []byte, frameQueueSize),
		closed:    make(chan struct{}),
		logger:    log.Named("videochannel.receiver"),
	}
	go c.readLoop()
	return c
}

// readLoop continuously reads RTP packets from the receiver and decodes frames.
func (c *conn) readLoop() {
	luma := make([]byte, lumaSize)
	var accumulator []byte
	var expectedSeq uint32

	for {
		select {
		case <-c.closed:
			return
		default:
		}

		pkt := &rtp.Packet{}
		if _, _, err := c.receiver.Read(luma); err != nil {
			if c.isClosed() {
				return
			}
			c.logger.Warn("rtp read error", zap.Error(err))
			continue
		}
		_ = pkt

		seq, payload, err := DecodeFrame(luma)
		if err != nil {
			c.logger.Warn("decode frame", zap.Error(err))
			continue
		}

		// Simple re-ordering: drop frames that arrive too late.
		if seq < expectedSeq && expectedSeq-seq < 1000 {
			continue
		}
		if seq > expectedSeq+1 {
			// Gap detected — flush accumulator as-is and reset.
			if len(accumulator) > 0 {
				buf := make([]byte, len(accumulator))
				copy(buf, accumulator)
				select {
				case c.readQueue <- buf:
				case <-c.closed:
					return
				}
				accumulator = accumulator[:0]
			}
		}
		expectedSeq = seq + 1
		accumulator = append(accumulator, payload...)
		c.packetsRecv.Add(1)

		// Flush on non-zero payload to avoid latency accumulation.
		if len(accumulator) > 0 {
			buf := make([]byte, len(accumulator))
			copy(buf, accumulator)
			select {
			case c.readQueue <- buf:
			case <-c.closed:
				return
			}
			accumulator = accumulator[:0]
		}
	}
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

// Write encodes b into a synthetic video frame and sends it as RTP.
func (c *conn) Write(b []byte) (int, error) {
	if c.isClosed() {
		return 0, net.ErrClosed
	}
	if c.track == nil {
		return 0, fmt.Errorf("videochannel: not a sender conn")
	}

	// Split into MaxPayloadSize chunks if needed.
	total := 0
	for len(b) > 0 {
		chunk := b
		if len(chunk) > MaxPayloadSize {
			chunk = b[:MaxPayloadSize]
		}

		luma := make([]byte, lumaSize)
		c.mu.Lock()
		seq := c.seq
		c.seq++
		c.mu.Unlock()

		if err := EncodeFrame(luma, seq, chunk); err != nil {
			return total, fmt.Errorf("encode frame: %w", err)
		}

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    rtpPayloadType,
				SequenceNumber: uint16(seq),
				Timestamp:      seq * 90000 / 30, // 30 fps clock
				SSRC:           0xDEADBEEF,
			},
			Payload: luma,
		}

		raw, err := pkt.Marshal()
		if err != nil {
			return total, fmt.Errorf("marshal rtp: %w", err)
		}
		if _, err := c.track.Write(raw); err != nil {
			return total, fmt.Errorf("write rtp: %w", err)
		}

		c.bytesWritten.Add(uint64(len(chunk)))
		c.packetsSent.Add(1)
		total += len(chunk)
		b = b[len(chunk):]
	}
	return total, nil
}

// Read decodes the next payload from the receive queue.
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
	case buf, ok := <-c.readQueue:
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
		return 0, context.DeadlineExceeded
	}
}

func (c *conn) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

// Close implements net.Conn.
func (c *conn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	if c.sender != nil {
		return c.sender.Stop()
	}
	return nil
}

// LocalAddr implements net.Conn.
func (c *conn) LocalAddr() net.Addr { return videoAddr("videochannel:local") }

// RemoteAddr implements net.Conn.
func (c *conn) RemoteAddr() net.Addr { return videoAddr("videochannel:remote") }

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

// ReadFrom implements io.ReaderFrom.
func (c *conn) ReadFrom(r io.Reader) (int64, error) {
	buf := make([]byte, MaxPayloadSize)
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

// WriteTo implements io.WriterTo.
func (c *conn) WriteTo(w io.Writer) (int64, error) {
	buf := make([]byte, MaxPayloadSize)
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

type videoAddr string

func (a videoAddr) Network() string { return "olcrtc-video" }
func (a videoAddr) String() string  { return string(a) }

// ---- Factory ----

// Factory is the VideoChannel transport factory.
type Factory struct{}

// Kind returns KindVideoChannel.
func (f *Factory) Kind() transport.Kind { return transport.KindVideoChannel }

// NewDialer returns a VideoChannel Dialer.
func (f *Factory) NewDialer() transport.Dialer { return &Dialer{} }

// NewListener returns a VideoChannel Listener.
func (f *Factory) NewListener(peer provider.Peer, cfg transport.Config) (transport.Listener, error) {
	return NewListener(peer, cfg)
}

// ---- Dialer ----

// Dialer opens outbound VideoChannel transports.
type Dialer struct{}

// Dial creates a local H.264 video track and adds it to the peer connection.
func (d *Dialer) Dial(ctx context.Context, peer provider.Peer, _ transport.Config) (transport.Transport, error) {
	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video", "olcrtc-video",
	)
	if err != nil {
		return nil, fmt.Errorf("new local track: %w", err)
	}

	sender, err := peer.PeerConnection().AddTrack(track)
	if err != nil {
		return nil, fmt.Errorf("add track: %w", err)
	}

	// Drain RTCP packets to avoid goroutine leak.
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := sender.Read(buf); rtcpErr != nil {
				return
			}
		}
	}()

	return newSenderConn(track, sender), nil
}

// ---- Listener ----

// Listener accepts inbound VideoChannel transports from a remote peer.
type Listener struct {
	queue  chan transport.Transport
	closed chan struct{}
	once   sync.Once
}

// NewListener creates a Listener that watches for incoming video tracks.
func NewListener(peer provider.Peer, _ transport.Config) (*Listener, error) {
	l := &Listener{
		queue:  make(chan transport.Transport, 16),
		closed: make(chan struct{}),
	}

	peer.PeerConnection().OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeVideo {
			return
		}
		c := newReceiverConn(receiver)
		select {
		case l.queue <- c:
		case <-l.closed:
		}
	})

	return l, nil
}

// Accept waits for the next inbound video transport.
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