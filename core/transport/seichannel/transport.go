// Package seichannel implements a data transport using H.264 SEI
// (Supplemental Enhancement Information) NAL units embedded inside
// video frames. This is even less visible than full-frame encoding
// but has lower throughput (~8 KiB per frame at 30 fps ≈ 240 KiB/s).
package seichannel

import (
	"context"
	"encoding/binary"
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
	// seiUUID identifies olcrtc SEI payloads (user_data_unregistered, type 5).
	// 16-byte UUID prefix: 4f4c-4356-5345-4900-0000-000000000001
	seiPayloadType = 5

	maxSEIPayload   = 8 * 1024 // 8 KiB per SEI NAL
	seiQueueSize    = 256
	seiRTPPayloadPT = 103 // dynamic PT for H.264+SEI
)

// seiUUIDBytes is our registered SEI user_data UUID prefix.
var seiUUIDBytes = [16]byte{
	0x4F, 0x4C, 0x43, 0x56, 0x53, 0x45, 0x49, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
}

// buildSEIPacket wraps payload in a SEI NAL unit for user_data_unregistered.
// Format: [NAL header (1)] [SEI type (1)] [size varint] [UUID (16)] [data]
func buildSEIPacket(seq uint32, payload []byte) ([]byte, error) {
	if len(payload) > maxSEIPayload {
		return nil, fmt.Errorf("sei payload too large: %d", len(payload))
	}

	// Inner data = 4-byte seq + UUID prefix + payload.
	inner := make([]byte, 4+16+len(payload))
	binary.BigEndian.PutUint32(inner[:4], seq)
	copy(inner[4:20], seiUUIDBytes[:])
	copy(inner[20:], payload)

	// H.264 SEI RBSP: NAL unit type 6 (SEI), payload_type=5, size, data.
	var out []byte
	out = append(out, 0x06)             // NAL header: SEI
	out = append(out, seiPayloadType)   // payload type = user_data_unregistered
	out = appendSEISize(out, len(inner))
	out = append(out, inner...)
	out = append(out, 0x80) // RBSP stop bit

	return out, nil
}

// parseSEIPacket extracts the sequence and payload from a SEI NAL unit.
func parseSEIPacket(data []byte) (seq uint32, payload []byte, err error) {
	// Skip NAL header and SEI type byte.
	if len(data) < 3 {
		return 0, nil, fmt.Errorf("sei packet too short")
	}
	// NAL header = data[0], seiType = data[1]
	if data[0] != 0x06 {
		return 0, nil, fmt.Errorf("not a SEI NAL unit: 0x%02X", data[0])
	}
	if data[1] != seiPayloadType {
		return 0, nil, fmt.Errorf("unexpected sei type: %d", data[1])
	}

	size, n := readSEISize(data[2:])
	if n <= 0 || 2+n+size > len(data) {
		return 0, nil, fmt.Errorf("sei size field corrupt")
	}
	inner := data[2+n : 2+n+size]

	if len(inner) < 4+16 {
		return 0, nil, fmt.Errorf("sei inner payload too short")
	}

	// Validate UUID prefix.
	var gotUUID [16]byte
	copy(gotUUID[:], inner[4:20])
	if gotUUID != seiUUIDBytes {
		return 0, nil, fmt.Errorf("sei uuid mismatch")
	}

	seq = binary.BigEndian.Uint32(inner[:4])
	payload = make([]byte, len(inner)-20)
	copy(payload, inner[20:])
	return seq, payload, nil
}

// appendSEISize encodes size as MPEG-style variable-length field.
func appendSEISize(dst []byte, size int) []byte {
	for size >= 255 {
		dst = append(dst, 0xFF)
		size -= 255
	}
	return append(dst, byte(size))
}

func readSEISize(data []byte) (size, bytesRead int) {
	for _, b := range data {
		bytesRead++
		size += int(b)
		if b != 0xFF {
			break
		}
	}
	return size, bytesRead
}

// conn implements transport.Transport over SEI-encoded RTP.
type conn struct {
	id   string
	kind transport.Kind

	track    *webrtc.TrackLocalStaticRTP
	sender   *webrtc.RTPSender
	receiver *webrtc.RTPReceiver

	readQueue chan []byte
	pending   []byte

	closeOnce sync.Once
	closed    chan struct{}

	seqOut uint32
	mu     sync.Mutex

	bytesRead    atomic.Uint64
	bytesWritten atomic.Uint64
	packetsSent  atomic.Uint64
	packetsRecv  atomic.Uint64

	readDeadline  atomic.Value
	writeDeadline atomic.Value

	logger *zap.Logger
}

func newSEISenderConn(track *webrtc.TrackLocalStaticRTP, sender *webrtc.RTPSender) *conn {
	return &conn{
		id:        uuid.NewString(),
		kind:      transport.KindSEIChannel,
		track:     track,
		sender:    sender,
		readQueue: make(chan []byte, seiQueueSize),
		closed:    make(chan struct{}),
		logger:    log.Named("seichannel.sender"),
	}
}

func newSEIReceiverConn(receiver *webrtc.RTPReceiver) *conn {
	c := &conn{
		id:        uuid.NewString(),
		kind:      transport.KindSEIChannel,
		receiver:  receiver,
		readQueue: make(chan []byte, seiQueueSize),
		closed:    make(chan struct{}),
		logger:    log.Named("seichannel.receiver"),
	}
	go c.readLoop()
	return c
}

func (c *conn) readLoop() {
	buf := make([]byte, 65535)
	for {
		select {
		case <-c.closed:
			return
		default:
		}
		n, _, err := c.receiver.Read(buf)
		if err != nil {
			if c.isClosed() {
				return
			}
			c.logger.Warn("rtp sei read", zap.Error(err))
			continue
		}

		pkt := &rtp.Packet{}
		if parseErr := pkt.Unmarshal(buf[:n]); parseErr != nil {
			c.logger.Warn("rtp unmarshal", zap.Error(parseErr))
			continue
		}

		_, payload, parseErr := parseSEIPacket(pkt.Payload)
		if parseErr != nil {
			c.logger.Debug("parse sei", zap.Error(parseErr))
			continue
		}

		if len(payload) == 0 {
			continue
		}
		out := make([]byte, len(payload))
		copy(out, payload)
		c.packetsRecv.Add(1)

		select {
		case c.readQueue <- out:
		case <-c.closed:
			return
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

// Write sends b as one or more SEI-encoded RTP packets.
func (c *conn) Write(b []byte) (int, error) {
	if c.isClosed() {
		return 0, net.ErrClosed
	}
	if c.track == nil {
		return 0, fmt.Errorf("seichannel: not a sender conn")
	}

	total := 0
	for len(b) > 0 {
		chunk := b
		if len(chunk) > maxSEIPayload {
			chunk = b[:maxSEIPayload]
		}

		c.mu.Lock()
		seq := c.seqOut
		c.seqOut++
		c.mu.Unlock()

		seiNAL, err := buildSEIPacket(seq, chunk)
		if err != nil {
			return total, err
		}

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    seiRTPPayloadPT,
				SequenceNumber: uint16(seq),
				Timestamp:      seq * 3000,
				SSRC:           0xC0FFEE01,
			},
			Payload: seiNAL,
		}
		raw, err := pkt.Marshal()
		if err != nil {
			return total, fmt.Errorf("marshal sei rtp: %w", err)
		}
		if _, err := c.track.Write(raw); err != nil {
			return total, fmt.Errorf("write sei rtp: %w", err)
		}

		c.bytesWritten.Add(uint64(len(chunk)))
		c.packetsSent.Add(1)
		total += len(chunk)
		b = b[len(chunk):]
	}
	return total, nil
}

// Read returns the next decoded payload.
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

func (c *conn) LocalAddr() net.Addr               { return seiAddr("sei:local") }
func (c *conn) RemoteAddr() net.Addr              { return seiAddr("sei:remote") }
func (c *conn) SetDeadline(t time.Time) error      { c.readDeadline.Store(t); c.writeDeadline.Store(t); return nil }
func (c *conn) SetReadDeadline(t time.Time) error  { c.readDeadline.Store(t); return nil }
func (c *conn) SetWriteDeadline(t time.Time) error { c.writeDeadline.Store(t); return nil }

func (c *conn) ReadFrom(r io.Reader) (int64, error) {
	buf := make([]byte, maxSEIPayload)
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

func (c *conn) WriteTo(w io.Writer) (int64, error) {
	buf := make([]byte, maxSEIPayload)
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

type seiAddr string

func (a seiAddr) Network() string { return "olcrtc-sei" }
func (a seiAddr) String() string  { return string(a) }

// ---- Factory ----

// Factory is the SEIChannel transport factory.
type Factory struct{}

func (f *Factory) Kind() transport.Kind { return transport.KindSEIChannel }
func (f *Factory) NewDialer() transport.Dialer {
	return &Dialer{}
}
func (f *Factory) NewListener(peer provider.Peer, cfg transport.Config) (transport.Listener, error) {
	return NewListener(peer, cfg)
}

// Dialer opens outbound SEIChannel transports.
type Dialer struct{}

func (d *Dialer) Dial(ctx context.Context, peer provider.Peer, _ transport.Config) (transport.Transport, error) {
	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video", "olcrtc-sei",
	)
	if err != nil {
		return nil, fmt.Errorf("new sei track: %w", err)
	}
	sender, err := peer.PeerConnection().AddTrack(track)
	if err != nil {
		return nil, fmt.Errorf("add sei track: %w", err)
	}
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := sender.Read(buf); rtcpErr != nil {
				return
			}
		}
	}()
	return newSEISenderConn(track, sender), nil
}

// Listener accepts inbound SEI video tracks.
type Listener struct {
	queue  chan transport.Transport
	closed chan struct{}
	once   sync.Once
}

func NewListener(peer provider.Peer, _ transport.Config) (*Listener, error) {
	l := &Listener{
		queue:  make(chan transport.Transport, 16),
		closed: make(chan struct{}),
	}
	peer.PeerConnection().OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeVideo {
			return
		}
		c := newSEIReceiverConn(receiver)
		select {
		case l.queue <- c:
		case <-l.closed:
		}
	})
	return l, nil
}

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

func (l *Listener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}