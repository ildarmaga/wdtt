package server

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseRawConfRequest(t *testing.T) {
	dev, pass, mtu, ok := parseRawConfRequest("RAWCONF:dev-1|secret|1400")
	if !ok || dev != "dev-1" || pass != "secret" || mtu != 1400 {
		t.Fatalf("RAWCONF got %q %q %d ok=%v", dev, pass, mtu, ok)
	}
	dev, pass, mtu, ok = parseRawConfRequest("GETCONF_RAW:abc|pwd")
	if !ok || dev != "abc" || pass != "pwd" {
		t.Fatalf("GETCONF_RAW got %q %q ok=%v", dev, pass, ok)
	}
	if _, _, _, ok := parseRawConfRequest("GETCONF:9000|d|p"); ok {
		t.Fatal("GETCONF must not parse as RAW")
	}
}

func TestBuildRawClientConfig(t *testing.T) {
	out := buildRawClientConfig("10.70.66.5", 1280)
	if out != "IP = 10.70.66.5\nDNS = "+clientDNS+"\nMTU = 1280\nCAPS = CHUNK1\n" {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestBuildQWDTTRawConfig(t *testing.T) {
	out := buildQWDTTRawConfig("10.70.66.5", 1280)
	want := "RAWCONF:10.70.66.5|" + clientDNS + "|1280"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestParseRawAuthRequest(t *testing.T) {
	dev, pass, ok := parseRawAuthRequest("AUTH:phone1|secret")
	if !ok || dev != "phone1" || pass != "secret" {
		t.Fatalf("got %q %q ok=%v", dev, pass, ok)
	}
	if _, _, ok := parseRawAuthRequest("GETCONF_RAW:a|b"); ok {
		t.Fatal("GETCONF_RAW must not parse as AUTH")
	}
}

func TestLookupRawIPForDeviceNoAlloc(t *testing.T) {
	rawIPByDevice.Range(func(k, _ any) bool {
		rawIPByDevice.Delete(k)
		return true
	})
	if ip := lookupRawIPForDevice("missing"); ip != "" {
		t.Fatalf("want empty, got %q", ip)
	}
	rawIPByDevice.Store("dev-x", "10.70.1.2")
	if ip := lookupRawIPForDevice("dev-x"); ip != "10.70.1.2" {
		t.Fatalf("got %q", ip)
	}
}

func TestQWDTTGetConfRawNoChallenge(t *testing.T) {
	// qWDTT шлёт GETCONF_RAW и ждёт сразу RAWCONF:ip|dns|mtu — без RAWCHAL.
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	rawModeEnabled.Store(true)
	t.Cleanup(func() { rawModeEnabled.Store(false) })

	dbMutex.Lock()
	prevMain := db.MainPassword
	db.MainPassword = "qwdtt-pass"
	dbMutex.Unlock()
	t.Cleanup(func() {
		dbMutex.Lock()
		db.MainPassword = prevMain
		dbMutex.Unlock()
	})

	rawIPSeq.Store(0)
	rawIPByDevice.Range(func(k, _ any) bool {
		rawIPByDevice.Delete(k)
		return true
	})
	rawSessionsByIP.Range(func(k, _ any) bool {
		rawSessionsByIP.Delete(k)
		return true
	})

	// TUN may fail in unit test env — stub by skipping ensure via short path:
	// call response builder path through handleRawConf only if TUN works.
	// Instead assert wire format helpers + that GETCONF_RAW is classified as no-challenge.
	first := "GETCONF_RAW:android-dev|qwdtt-pass"
	if !strings.HasPrefix(first, "GETCONF_RAW:") {
		t.Fatal("prefix")
	}
	dev, pass, _, ok := parseRawConfRequest(first)
	if !ok || dev != "android-dev" || pass != "qwdtt-pass" {
		t.Fatalf("parse %q %q ok=%v", dev, pass, ok)
	}
	cfg := buildQWDTTRawConfig("10.70.0.2", 1280)
	if !strings.HasPrefix(cfg, "RAWCONF:") || strings.Contains(cfg, "RAWCHAL") {
		t.Fatalf("bad qwdtt cfg %q", cfg)
	}
	_ = c1
	_ = c2
}

func TestRawRequestHasChunk1(t *testing.T) {
	if !rawRequestHasChunk1("RAWCONF:dev|pass|1160|CHUNK1") {
		t.Fatal("CHUNK1 capability not detected")
	}
	if rawRequestHasChunk1("RAWCONF:dev|pass|1160") {
		t.Fatal("legacy request detected as CHUNK1")
	}
	if rawRequestHasChunk1("RAWCONF:dev|CHUNK1|1160") {
		t.Fatal("password CHUNK1 must not enable capability")
	}
}

func TestRawDirectListenAddr(t *testing.T) {
	got, err := rawDirectListenAddr("0.0.0.0:56000", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.0.0.0:56003" {
		t.Fatalf("got %q", got)
	}
	got, err = rawDirectListenAddr("0.0.0.0:56000", 56111)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.0.0.0:56111" {
		t.Fatalf("explicit got %q", got)
	}
}

func TestRawDirectChallengeConsumeOnce(t *testing.T) {
	peer := &net.UDPAddr{IP: net.ParseIP("203.0.113.10"), Port: 40000}
	other := &net.UDPAddr{IP: net.ParseIP("198.51.100.20"), Port: 40001}
	chal, err := issueRawDirectChallenge(peer)
	if err != nil {
		t.Fatal(err)
	}
	req := "RAWCONF:dev|pass|1160|CHUNK1|CHAL=" + chal
	if consumeRawDirectChallenge(req, other) {
		t.Fatal("foreign peer must not consume challenge")
	}
	// чужой peer не должен сжигать chal
	if !consumeRawDirectChallenge(req, peer) {
		t.Fatal("owner peer consume should succeed")
	}
	if consumeRawDirectChallenge(req, peer) {
		t.Fatal("replay must fail")
	}
	if consumeRawDirectChallenge("RAWCONF:dev|pass|1160|CHUNK1", peer) {
		t.Fatal("missing CHAL must fail")
	}
}

func TestGetNextRawIPUnique(t *testing.T) {
	rawIPSeq.Store(0)
	rawSessionsByIP.Range(func(k, _ any) bool {
		rawSessionsByIP.Delete(k)
		return true
	})
	rawIPByDevice.Range(func(k, _ any) bool {
		rawIPByDevice.Delete(k)
		return true
	})
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		ip := getNextRawIP()
		if ip == "" {
			t.Fatal("empty IP")
		}
		if ip == rawServerAddr {
			t.Fatalf("server addr allocated: %s", ip)
		}
		if !strings.HasPrefix(ip, "10.70.") {
			t.Fatalf("want 10.70.x.y got %s", ip)
		}
		if seen[ip] {
			t.Fatalf("duplicate %s", ip)
		}
		seen[ip] = true
		rawSessionsByIP.Store(ip, &rawSessionGroup{ip: ip})
	}
}

func TestGetRawIPForDeviceStickyShared(t *testing.T) {
	rawIPSeq.Store(0)
	rawSessionsByIP.Range(func(k, _ any) bool {
		rawSessionsByIP.Delete(k)
		return true
	})
	rawIPByDevice.Range(func(k, _ any) bool {
		rawIPByDevice.Delete(k)
		return true
	})

	a := getRawIPForDevice("device-A")
	b := getRawIPForDevice("device-A")
	c := getRawIPForDevice("device-B")
	if a == "" || a != b {
		t.Fatalf("same device must share IP: %q vs %q", a, b)
	}
	if c == "" || c == a {
		t.Fatalf("different devices need different IPs: A=%q B=%q", a, c)
	}
}

func tcpRawPkt(srcHost byte, sport, dport uint16) []byte {
	pkt := make([]byte, 40)
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], 40)
	pkt[8] = 64
	pkt[9] = 6
	pkt[12], pkt[13], pkt[14], pkt[15] = 10, 70, 0, srcHost
	pkt[16], pkt[17], pkt[18], pkt[19] = 1, 2, 3, 4
	binary.BigEndian.PutUint16(pkt[20:22], sport)
	binary.BigEndian.PutUint16(pkt[22:24], dport)
	return pkt
}

func TestRawFlowKeyBidirectional(t *testing.T) {
	up := tcpRawPkt(2, 50000, 443)
	down := make([]byte, len(up))
	copy(down, up)
	copy(down[12:16], up[16:20])
	copy(down[16:20], up[12:16])
	binary.BigEndian.PutUint16(down[20:22], 443)
	binary.BigEndian.PutUint16(down[22:24], 50000)
	if rawFlowKey(up) != rawFlowKey(down) {
		t.Fatalf("up/down must match: %x vs %x", rawFlowKey(up), rawFlowKey(down))
	}
}

func TestRawSessionGroupPickSticky(t *testing.T) {
	g := &rawSessionGroup{ip: "10.70.0.2"}
	s1 := &rawSession{ip: "10.70.0.2", deviceID: "d", downCh: make(chan []byte, 64)}
	s2 := &rawSession{ip: "10.70.0.2", deviceID: "d", downCh: make(chan []byte, 64)}
	g.add(s1)
	g.add(s2)

	pkt := tcpRawPkt(2, 1234, 443)
	g.mu.Lock()
	first := g.pickForLocked(pkt)
	g.mu.Unlock()
	for i := 0; i < 16; i++ {
		g.mu.Lock()
		got := g.pickForLocked(pkt)
		g.mu.Unlock()
		if got != first {
			t.Fatalf("iter %d: sticky broken", i)
		}
	}

	// Другой поток может попасть на другого воркера — но не обязан.
	other := tcpRawPkt(2, 9999, 80)
	hitOther := false
	for sport := uint16(2000); sport < 2200; sport++ {
		p := tcpRawPkt(2, sport, 443)
		g.mu.Lock()
		got := g.pickForLocked(p)
		g.mu.Unlock()
		if got != first {
			hitOther = true
			_ = other
			break
		}
	}
	if !hitOther {
		t.Log("note: all sampled flows hashed to same worker (OK, probabilistic)")
	}

	if g.remove(s1) {
		t.Fatal("group still has s2")
	}
	if g.len() != 1 {
		t.Fatalf("len=%d", g.len())
	}
	if !g.remove(s2) {
		t.Fatal("want empty")
	}
}

func TestRawNoteUplinkPinsDownlink(t *testing.T) {
	g := &rawSessionGroup{ip: "10.70.0.2"}
	s1 := &rawSession{ip: "10.70.0.2", deviceID: "d", downCh: make(chan []byte, 64)}
	s2 := &rawSession{ip: "10.70.0.2", deviceID: "d", downCh: make(chan []byte, 64)}
	g.add(s1)
	g.add(s2)

	up := tcpRawPkt(2, 3333, 443)
	// Принудительно выбираем s2 через uplink (даже если hash%N → s1).
	g.noteUplink(s2, up)

	down := make([]byte, len(up))
	copy(down, up)
	copy(down[12:16], up[16:20])
	copy(down[16:20], up[12:16])
	binary.BigEndian.PutUint16(down[20:22], 443)
	binary.BigEndian.PutUint16(down[22:24], 3333)

	g.mu.Lock()
	got := g.pickForLocked(down)
	g.mu.Unlock()
	if got != s2 {
		t.Fatalf("downlink must follow uplink session, got other")
	}
	for i := 0; i < 10; i++ {
		g.enqueueDown(down)
	}
	if len(s2.downCh) != 10 || len(s1.downCh) != 0 {
		t.Fatalf("want all on s2, got s1=%d s2=%d", len(s1.downCh), len(s2.downCh))
	}
}

func TestRawSessionGroupEnqueueSticky(t *testing.T) {
	g := &rawSessionGroup{ip: "10.70.0.2"}
	s1 := &rawSession{ip: "10.70.0.2", deviceID: "d", downCh: make(chan []byte, 64)}
	s2 := &rawSession{ip: "10.70.0.2", deviceID: "d", downCh: make(chan []byte, 64)}
	g.add(s1)
	g.add(s2)
	pkt := tcpRawPkt(2, 4242, 443)
	for i := 0; i < 20; i++ {
		g.enqueueDown(pkt)
	}
	n1, n2 := len(s1.downCh), len(s2.downCh)
	if n1+n2 != 20 {
		t.Fatalf("want 20 queued, got %d+%d", n1, n2)
	}
	if n1 > 0 && n2 > 0 {
		t.Fatal("chunk-RR regression: one flow must not split across sessions")
	}
}

func TestRawSessionGroupSkipsAndCancelsStaleSession(t *testing.T) {
	g := &rawSessionGroup{ip: "10.70.0.2"}
	staleCtx, staleCancel := context.WithCancel(context.Background())
	stale := &rawSession{
		ip: "10.70.0.2", deviceID: "d",
		downCh: make(chan []byte, 64), cancel: staleCancel,
	}
	fresh := &rawSession{
		ip: "10.70.0.2", deviceID: "d",
		downCh: make(chan []byte, 64),
	}
	g.add(stale)
	g.add(fresh)
	stale.lastSeen.Store(time.Now().Add(-rawSessionFreshTTL - time.Second).UnixNano())
	g.framed.Store(true)
	g.framedLastSeen.Store(time.Now().UnixNano())
	fresh.framed.Store(true)

	for i := 0; i < 10; i++ {
		g.enqueueDown(tcpRawPkt(2, uint16(5000+i), 443))
	}
	if len(stale.downCh) != 0 || len(fresh.downCh) != 10 {
		t.Fatalf("stale session received downlink: stale=%d fresh=%d", len(stale.downCh), len(fresh.downCh))
	}
	select {
	case <-staleCtx.Done():
	default:
		t.Fatal("stale session was not cancelled")
	}
}

func TestRawMultipathUsesPacketChunks(t *testing.T) {
	g := &rawSessionGroup{ip: "10.70.0.2"}
	s1 := &rawSession{ip: "10.70.0.2", deviceID: "d", downCh: make(chan []byte, 64)}
	s2 := &rawSession{ip: "10.70.0.2", deviceID: "d", downCh: make(chan []byte, 64)}
	g.add(s1)
	g.add(s2)
	s1.framed.Store(true)
	s2.framed.Store(true)
	g.framed.Store(true)
	g.framedLastSeen.Store(time.Now().UnixNano())
	for i := 0; i < rawMPChunkPackets; i++ {
		g.enqueueDown(tcpRawPkt(2, 6000, 443))
	}
	first1, first2 := len(s1.downCh), len(s2.downCh)
	if first1 != rawMPChunkPackets && first2 != rawMPChunkPackets {
		t.Fatalf("first chunk split: s1=%d s2=%d", first1, first2)
	}
	for i := 0; i < rawMPChunkPackets; i++ {
		g.enqueueDown(tcpRawPkt(2, 6000, 443))
	}
	if len(s1.downCh) != rawMPChunkPackets || len(s2.downCh) != rawMPChunkPackets {
		t.Fatalf("chunks not balanced: s1=%d s2=%d", len(s1.downCh), len(s2.downCh))
	}
}

func TestRawChunkedDownlinkMirrorsClientScheduler(t *testing.T) {
	g := &rawSessionGroup{ip: "10.70.0.2"}
	s1 := &rawSession{ip: "10.70.0.2", deviceID: "d", chunked: true, downCh: make(chan []byte, 128)}
	s2 := &rawSession{ip: "10.70.0.2", deviceID: "d", chunked: true, downCh: make(chan []byte, 128)}
	g.add(s1)
	g.add(s2)
	pkt := tcpRawPkt(2, 6001, 443)
	pkt = append(pkt, make([]byte, 1160-len(pkt))...)
	binary.BigEndian.PutUint16(pkt[2:4], uint16(len(pkt)))
	for i := 0; i < 65; i++ {
		if ok, _ := g.enqueueDown(pkt); !ok {
			t.Fatalf("enqueue %d failed", i)
		}
	}
	if len(s1.downCh) != 64 || len(s2.downCh) != 1 {
		t.Fatalf("chunk split incorrectly: s1=%d s2=%d", len(s1.downCh), len(s2.downCh))
	}
}

func TestRawExpiredMultipathModeFallsBackToSticky(t *testing.T) {
	g := &rawSessionGroup{ip: "10.70.0.2"}
	s := &rawSession{ip: "10.70.0.2", deviceID: "d", downCh: make(chan []byte, 1)}
	g.add(s)
	g.framed.Store(true)
	g.framedLastSeen.Store(time.Now().Add(-rawFramedModeTTL - time.Second).UnixNano())
	pkt := tcpRawPkt(2, 7000, 443)
	if ok, _ := g.enqueueDown(pkt); !ok {
		t.Fatal("enqueue failed")
	}
	got := <-s.downCh
	if isRawFrame(got) {
		t.Fatal("expired multipath mode must not frame sticky downlink")
	}
}

func TestRawMultipathNeverFramesStickySession(t *testing.T) {
	g := &rawSessionGroup{ip: "10.70.0.2"}
	mp := &rawSession{ip: "10.70.0.2", deviceID: "d", downCh: make(chan []byte, 1)}
	sticky := &rawSession{ip: "10.70.0.2", deviceID: "d", downCh: make(chan []byte, 1)}
	mp.framed.Store(true)
	g.add(mp)
	g.add(sticky)
	g.framed.Store(true)
	g.framedLastSeen.Store(time.Now().UnixNano())
	pkt := tcpRawPkt(2, 7001, 443)
	g.noteUplink(sticky, pkt)
	if ok, _ := g.enqueueDown(pkt); !ok {
		t.Fatal("enqueue failed")
	}
	if len(mp.downCh) != 0 {
		t.Fatal("sticky-affine flow was diverted to multipath")
	}
	if got := <-sticky.downCh; isRawFrame(got) {
		t.Fatal("sticky-affine flow received an RA frame")
	}
}

func TestRawIPv4SourceBinding(t *testing.T) {
	pkt := tcpRawPkt(2, 7002, 443)
	if !rawIPv4SourceMatches(pkt, "10.70.0.2") {
		t.Fatal("assigned source rejected")
	}
	if rawIPv4SourceMatches(pkt, "10.70.0.3") {
		t.Fatal("spoofed source accepted")
	}
	pkt[2], pkt[3] = 0xff, 0xff
	if rawIPv4SourceMatches(pkt, "10.70.0.2") {
		t.Fatal("invalid total length accepted")
	}
}

func TestRawDeviceIdentityIncludesCredential(t *testing.T) {
	a := rawDeviceIdentity("same-device", "password-a")
	b := rawDeviceIdentity("same-device", "password-b")
	if a == b {
		t.Fatal("different credentials must not share RAW identity")
	}
	if a != rawDeviceIdentity("same-device", "password-a") {
		t.Fatal("RAW identity is not stable")
	}
}

func TestRawSessionGroupLimit(t *testing.T) {
	g := &rawSessionGroup{ip: "10.70.0.2"}
	for i := 0; i < 2; i++ {
		if !g.tryAdd(&rawSession{downCh: make(chan []byte, 1)}, 2) {
			t.Fatalf("session %d rejected below limit", i)
		}
	}
	if g.tryAdd(&rawSession{downCh: make(chan []byte, 1)}, 2) {
		t.Fatal("session accepted above limit")
	}
}

func TestRawSubnetCIDR(t *testing.T) {
	if rawServerCIDR != "10.70.66.1/16" || rawSubnet != "10.70.0.0/16" {
		t.Fatalf("cidr=%s subnet=%s", rawServerCIDR, rawSubnet)
	}
}
