package server

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"
)

const (
	rawFrameMagic0     = 'R'
	rawFrameMagic1     = 'A'
	rawFrameHeader     = 6
	rawReorderMax      = 2048
	rawReorderStallTTL = 40 * time.Millisecond
)

func rawFrameEncode(seq uint32, ip []byte) []byte {
	out := make([]byte, rawFrameHeader+len(ip))
	out[0] = rawFrameMagic0
	out[1] = rawFrameMagic1
	binary.BigEndian.PutUint32(out[2:6], seq)
	copy(out[rawFrameHeader:], ip)
	return out
}

func rawFrameDecode(pkt []byte) (seq uint32, ip []byte, ok bool) {
	if len(pkt) < rawFrameHeader+20 {
		return 0, nil, false
	}
	if pkt[0] != rawFrameMagic0 || pkt[1] != rawFrameMagic1 {
		return 0, nil, false
	}
	seq = binary.BigEndian.Uint32(pkt[2:6])
	ip = pkt[rawFrameHeader:]
	if ip[0]>>4 != 4 {
		return 0, nil, false
	}
	return seq, ip, true
}

func isRawFrame(pkt []byte) bool {
	return len(pkt) >= 2 && pkt[0] == rawFrameMagic0 && pkt[1] == rawFrameMagic1
}

type rawReorder struct {
	mu        sync.Mutex
	next      uint32
	inited    bool
	buf       map[uint32][]byte
	waitSince time.Time
}

func newRawReorder() *rawReorder {
	return &rawReorder{buf: make(map[uint32][]byte)}
}

func (r *rawReorder) Push(seq uint32, ip []byte) [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.inited {
		r.next = seq
		r.inited = true
	}
	if seq < r.next && r.next-seq < 0x80000000 {
		return nil
	}
	if _, exists := r.buf[seq]; exists {
		return nil
	}
	if len(r.buf) >= rawReorderMax {
		r.next = seq
		r.buf = make(map[uint32][]byte)
		r.waitSince = time.Time{}
	}
	r.buf[seq] = append([]byte(nil), ip...)

	var out [][]byte
	for {
		if p, ok := r.buf[r.next]; ok {
			out = append(out, p)
			delete(r.buf, r.next)
			r.next++
			r.waitSince = time.Time{}
			continue
		}
		if len(r.buf) == 0 {
			r.waitSince = time.Time{}
			break
		}
		now := time.Now()
		if r.waitSince.IsZero() {
			r.waitSince = now
			break
		}
		if now.Sub(r.waitSince) < rawReorderStallTTL {
			break
		}
		var minSeq uint32
		first := true
		for s := range r.buf {
			if first || s < minSeq {
				minSeq = s
				first = false
			}
		}
		r.next = minSeq
		r.waitSince = time.Time{}
	}
	return out
}

type rawOutSeq struct{ v atomic.Uint32 }

func (s *rawOutSeq) Next() uint32 { return s.v.Add(1) - 1 }
