package server

import (
	"bytes"
	"testing"
)

func TestObfsAcceptsVideoPT(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, wrapKeyLen)
	payload := []byte("dtls-payload-for-video-obfs")

	cfg := NewObfsConfig()
	cfg.PayloadType = 96
	cfg.PaddingMax = 60
	st := NewObfsState()

	wire, err := obfsWrapPacket(key, payload, cfg, st)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	if !obfsIsRTPPacket(wire) {
		t.Fatalf("video PT rejected by obfsIsRTPPacket, PT=%d", wire[1]&0x7F)
	}
	if wire[1]&0x7F != 96 {
		t.Fatalf("wire PT=%d", wire[1]&0x7F)
	}

	dst := make([]byte, len(payload)+8)
	n, err := obfsUnwrapPacket(key, wire, dst)
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if !bytes.Equal(dst[:n], payload) {
		t.Fatal("payload mismatch")
	}
}
