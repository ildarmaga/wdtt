package server

import "testing"

func TestWgPeerRecentlyHandshook(t *testing.T) {
	userOnlineTimeoutSec = 25
	wgKeepaliveSec = 25
	now := int64(1_000_000)
	maxAge := wgPeerOnlineMaxAgeSec()

	if wgPeerRecentlyHandshook(0, now) {
		t.Fatal("zero handshake must not count as recent")
	}
	if !wgPeerRecentlyHandshook(now-int64(maxAge/2), now) {
		t.Fatal("recent handshake should count as active")
	}
	if wgPeerRecentlyHandshook(now-int64(maxAge+1), now) {
		t.Fatal("stale handshake must not count as active")
	}
}
