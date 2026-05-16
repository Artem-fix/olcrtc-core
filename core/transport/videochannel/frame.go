// Package videochannel implements steganographic data transport via H.264
// video frames sent over a WebRTC video track. Data bytes are encoded into
// the luma plane of synthetic video frames and decoded on the receiver.
//
// Security note: the payload is encrypted by the session layer (AEAD) before
// being handed to this transport, so the video encoding itself is only a
// carrier — not a security boundary.
package videochannel

import (
	"encoding/binary"
	"fmt"
)

const (
	// FrameHeaderSize is the byte overhead per encoded frame:
	// [4 magic][4 seq][4 payload_len] = 12 bytes.
	FrameHeaderSize = 12

	frameMagic = uint32(0x4F4C4356) // "OLCV"

	// MaxPayloadSize is the maximum bytes we embed per frame.
	// Chosen to fit comfortably in a 320×240 luma plane (76800 bytes).
	MaxPayloadSize = 65536
)

// FrameHeader is the per-frame metadata embedded at offset 0 of the luma plane.
type FrameHeader struct {
	Magic      uint32 // Must equal frameMagic.
	Seq        uint32 // Monotonically increasing sequence number.
	PayloadLen uint32 // Number of payload bytes following the header.
}

// MarshalHeader serialises the header into buf (must be ≥FrameHeaderSize).
func MarshalHeader(h FrameHeader, buf []byte) error {
	if len(buf) < FrameHeaderSize {
		return fmt.Errorf("buffer too small: need %d, have %d", FrameHeaderSize, len(buf))
	}
	binary.BigEndian.PutUint32(buf[0:4], h.Magic)
	binary.BigEndian.PutUint32(buf[4:8], h.Seq)
	binary.BigEndian.PutUint32(buf[8:12], h.PayloadLen)
	return nil
}

// UnmarshalHeader deserialises a FrameHeader from the first FrameHeaderSize bytes of buf.
func UnmarshalHeader(buf []byte) (FrameHeader, error) {
	if len(buf) < FrameHeaderSize {
		return FrameHeader{}, fmt.Errorf("buffer too small: need %d, have %d", FrameHeaderSize, len(buf))
	}
	h := FrameHeader{
		Magic:      binary.BigEndian.Uint32(buf[0:4]),
		Seq:        binary.BigEndian.Uint32(buf[4:8]),
		PayloadLen: binary.BigEndian.Uint32(buf[8:12]),
	}
	if h.Magic != frameMagic {
		return FrameHeader{}, fmt.Errorf("bad magic: 0x%08X", h.Magic)
	}
	if h.PayloadLen > MaxPayloadSize {
		return FrameHeader{}, fmt.Errorf("payload length too large: %d", h.PayloadLen)
	}
	return h, nil
}

// EncodeFrame writes header + payload into a raw luma buffer (width*height bytes).
// Remaining bytes are set to 0x80 (mid-grey) to produce a valid-looking picture.
func EncodeFrame(luma []byte, seq uint32, payload []byte) error {
	if len(payload) > MaxPayloadSize {
		return fmt.Errorf("payload too large: %d > %d", len(payload), MaxPayloadSize)
	}
	total := FrameHeaderSize + len(payload)
	if len(luma) < total {
		return fmt.Errorf("luma buffer too small: need %d, have %d", total, len(luma))
	}

	h := FrameHeader{
		Magic:      frameMagic,
		Seq:        seq,
		PayloadLen: uint32(len(payload)),
	}
	if err := MarshalHeader(h, luma); err != nil {
		return err
	}
	copy(luma[FrameHeaderSize:], payload)

	// Fill remainder with mid-grey so the frame is visually plausible.
	for i := total; i < len(luma); i++ {
		luma[i] = 0x80
	}
	return nil
}

// DecodeFrame extracts the payload from a raw luma buffer.
func DecodeFrame(luma []byte) (seq uint32, payload []byte, err error) {
	h, err := UnmarshalHeader(luma)
	if err != nil {
		return 0, nil, err
	}
	end := FrameHeaderSize + int(h.PayloadLen)
	if len(luma) < end {
		return 0, nil, fmt.Errorf("luma buffer truncated: need %d, have %d", end, len(luma))
	}
	out := make([]byte, h.PayloadLen)
	copy(out, luma[FrameHeaderSize:end])
	return h.Seq, out, nil
}