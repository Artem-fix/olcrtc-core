package videochannel_test

import (
	"bytes"
	"testing"

	"github.com/openlibrecommunity/olcrtc-core/core/transport/videochannel"
)

func TestFrame_RoundTrip(t *testing.T) {
	t.Parallel()

	luma := make([]byte, 320*240)
	payload := []byte("olcrtc videochannel frame test payload")

	if err := videochannel.EncodeFrame(luma, 42, payload); err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}

	seq, got, err := videochannel.DecodeFrame(luma)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if seq != 42 {
		t.Fatalf("seq: got %d want 42", seq)
	}
	if !bytes.Equal(payload, got) {
		t.Fatalf("payload mismatch")
	}
}

func TestFrame_MaxPayload(t *testing.T) {
	t.Parallel()

	luma := make([]byte, 320*240)
	big := bytes.Repeat([]byte{0xAB}, videochannel.MaxPayloadSize)

	if err := videochannel.EncodeFrame(luma, 0, big); err != nil {
		t.Fatalf("EncodeFrame max payload: %v", err)
	}

	_, got, err := videochannel.DecodeFrame(luma)
	if err != nil {
		t.Fatalf("DecodeFrame max payload: %v", err)
	}
	if !bytes.Equal(big, got) {
		t.Fatalf("max payload data mismatch")
	}
}

func TestFrame_BadMagic(t *testing.T) {
	t.Parallel()

	luma := make([]byte, 320*240)
	// Don't encode anything — magic will be 0x00000000.
	_, _, err := videochannel.DecodeFrame(luma)
	if err == nil {
		t.Fatal("expected error for bad magic, got nil")
	}
}