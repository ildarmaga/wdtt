package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

const (
	rawIfaceName  = "wdtt-raw"
	rawServerAddr = "10.70.66.1"
	rawServerCIDR = "10.70.66.1/16" // qWDTT: /16, multi-IP на воркеры
	rawSubnet     = "10.70.0.0/16"
	rawIPTComment = "WDTT_RAW_MANAGED"
	rawDefaultMTU = 1280
	rawTunFlags   = unix.IFF_TUN | unix.IFF_NO_PI
)

// rawSession — одна DTLS-сессия в режиме RAW (IP over WRAP, без WireGuard).
type rawSession struct {
	deviceID   string
	registryID string
	ip         string
	password   string
	isMain     bool
	conn       net.Conn
	cancel     context.CancelFunc
	downCh     chan []byte  // async downlink — TUN read не ждёт DTLS Write
	writeMu    sync.Mutex   // все Write в clientConn (downWriter + keepalive)
	lastSeen   atomic.Int64 // последний uplink/keepalive; stale TURN исключается из downlink
	framed     atomic.Bool  // эта DTLS-сессия понимает RA frames
	chunked    bool         // клиент договорился о CHUNK1
	up         int64
	down       int64
}

// rawSessionGroup — все DTLS-воркеры одного device делят один 10.70.x.y.
// Legacy sticky (без RA-frame) или multipath RR + reorder (клиент ≥0.3.267).
type rawSessionGroup struct {
	mu             sync.Mutex
	ip             string
	deviceID       string
	sessions       []*rawSession
	flowAff        map[uint64]*rawSession
	flowExp        map[uint64]int64
	framed         atomic.Bool // клиент шлёт RA-frame → downlink тоже framed RR
	framedLastSeen atomic.Int64
	outSeq         rawOutSeq
	reorder        *rawReorder
	rr             uint32
	chunkIndex     int
	chunkCount     int
	chunkStartedAt time.Time
}

const (
	rawDirectPortOffset = 3
	rawDownChBuf        = 8192
	rawFlowAffTTL       = 3 * time.Minute
	rawFlowAffMax       = 8192
	// Multipath flowlet: per-packet RR создаёт максимальный TCP reorder.
	rawMPChunkPackets         = 16
	rawFramedModeTTL          = 10 * time.Second
	rawMaxSessionsPerIdentity = 27
	rawMaxSessionsGlobal      = 512
	rawChunkMaxDwell          = 15 * time.Millisecond
	// Новый RAW-клиент шлёт keepalive каждые 3s; v0.3.271 — каждые 10s.
	// 25s сохраняет совместимость и вместо 30 минут быстро убирает stale path.
	rawSessionFreshTTL = 25 * time.Second
)

var (
	rawTunOnce    sync.Once
	rawTunDev     *os.File
	rawTunErr     error
	rawTunMu      sync.Mutex
	rawTunWriteMu sync.Mutex

	rawSessionsByIP    sync.Map // string IP -> *rawSessionGroup
	rawIPByDevice      sync.Map // credential-scoped device key -> string IP
	rawIPSeq           atomic.Uint32
	rawModeEnabled     atomic.Bool
	rawDownlinkStarted sync.Once
	rawActiveSessions  atomic.Int64
)

// rawDirectConn is a net.Conn over one authenticated RTP/WRAP flow.
type rawDirectConn struct {
	pc   net.PacketConn
	addr net.Addr
}

func (c *rawDirectConn) Read(b []byte) (int, error) {
	for {
		n, _, err := c.pc.ReadFrom(b)
		if err == nil {
			return n, nil
		}
		if _, networkError := err.(net.Error); networkError {
			return 0, err
		}
	}
}

func (c *rawDirectConn) Write(b []byte) (int, error) { return c.pc.WriteTo(b, c.addr) }
func (c *rawDirectConn) Close() error                { return c.pc.Close() }
func (c *rawDirectConn) LocalAddr() net.Addr         { return c.pc.LocalAddr() }
func (c *rawDirectConn) RemoteAddr() net.Addr        { return c.addr }
func (c *rawDirectConn) SetDeadline(t time.Time) error {
	return c.pc.SetDeadline(t)
}
func (c *rawDirectConn) SetReadDeadline(t time.Time) error {
	return c.pc.SetReadDeadline(t)
}
func (c *rawDirectConn) SetWriteDeadline(t time.Time) error {
	return c.pc.SetWriteDeadline(t)
}

// rawDirectChallenges — одноразовые challenge для RAWCONF на direct-порту
// (без DTLS нет handshake-nonce; chal привязан к RemoteAddr, иначе replay с другого UDP source).
type rawDirectChallenge struct {
	exp    int64
	remote string
}

var rawDirectChallenges sync.Map // chalHex -> rawDirectChallenge

const rawDirectChallengeTTL = 30 * time.Second

func rawChallengeRemoteKey(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	return addr.String()
}

func issueRawDirectChallenge(remote net.Addr) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	chal := hex.EncodeToString(b[:])
	rawDirectChallenges.Store(chal, rawDirectChallenge{
		exp:    time.Now().Add(rawDirectChallengeTTL).UnixNano(),
		remote: rawChallengeRemoteKey(remote),
	})
	return chal, nil
}

func consumeRawDirectChallenge(firstStr string, remote net.Addr) bool {
	wantRemote := rawChallengeRemoteKey(remote)
	if wantRemote == "" {
		return false
	}
	for _, part := range strings.Split(strings.TrimSpace(firstStr), "|") {
		part = strings.TrimSpace(part)
		upper := strings.ToUpper(part)
		if !strings.HasPrefix(upper, "CHAL=") {
			continue
		}
		chal := strings.TrimSpace(part[len("CHAL="):])
		v, ok := rawDirectChallenges.Load(chal)
		if !ok {
			return false
		}
		entry, ok := v.(rawDirectChallenge)
		if !ok {
			return false
		}
		if entry.remote != wantRemote {
			// чужой source не сжигает challenge владельца
			return false
		}
		if entry.exp < time.Now().UnixNano() {
			rawDirectChallenges.Delete(chal)
			return false
		}
		if _, loaded := rawDirectChallenges.LoadAndDelete(chal); !loaded {
			return false
		}
		return true
	}
	return false
}

func ensureRawDirectFirewall(listenAddr string) {
	_, portText, err := net.SplitHostPort(listenAddr)
	if err != nil || portText == "" {
		return
	}
	if !commandExists("iptables") {
		return
	}
	comment := "WDTT_RAW_DIRECT"
	check := exec.Command("iptables", "-C", "INPUT", "-p", "udp", "--dport", portText,
		"-m", "comment", "--comment", comment, "-j", "ACCEPT")
	if check.Run() == nil {
		return
	}
	out, addErr := exec.Command("iptables", "-I", "INPUT", "1", "-p", "udp", "--dport", portText,
		"-m", "comment", "--comment", comment, "-j", "ACCEPT").CombinedOutput()
	if addErr != nil {
		log.Printf("[RAW-DIRECT] iptables open %s: %v (%s)", portText, addErr, strings.TrimSpace(string(out)))
		return
	}
	log.Printf("[RAW-DIRECT] iptables INPUT udp/%s ACCEPT", portText)
}

func handleDirectRawConn(ctx context.Context, clientConn net.Conn) {
	atomic.AddInt64(&totalConns, 1)
	atomic.AddInt32(&activeConns, 1)
	defer atomic.AddInt32(&activeConns, -1)
	remote := clientConn.RemoteAddr()
	defer clearWrapAuth(remote)

	buf := make([]byte, 2048)
	readOnce := func() (string, error) {
		if err := clientConn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
			return "", err
		}
		n, err := clientConn.Read(buf)
		_ = clientConn.SetReadDeadline(time.Time{})
		if err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	}

	first, err := readOnce()
	if err != nil {
		return
	}
	if !isRawConfPacket(first) {
		log.Printf("[RAW-DIRECT] unexpected first packet from %s", remote)
		return
	}
	if !consumeRawDirectChallenge(first, remote) {
		chal, chalErr := issueRawDirectChallenge(remote)
		if chalErr != nil {
			log.Printf("[RAW-DIRECT] challenge: %v", chalErr)
			return
		}
		if _, err := clientConn.Write([]byte("RAWCHAL:" + chal)); err != nil {
			return
		}
		first, err = readOnce()
		if err != nil {
			return
		}
		if !isRawConfPacket(first) || !consumeRawDirectChallenge(first, remote) {
			log.Printf("[RAW-DIRECT] challenge rejected from %s", remote)
			_, _ = clientConn.Write([]byte("DENIED:wrong_password"))
			return
		}
	}
	handleRawConf(ctx, clientConn, first, lookupWrapAuth(remote))
}

func ensureRawTUN() error {
	rawTunOnce.Do(func() {
		rawTunErr = startRawTUN()
	})
	return rawTunErr
}

func createRawTUNFile(name string) (*os.File, error) {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/net/tun: %w", err)
	}
	ifr, err := unix.NewIfreq(name)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	ifr.SetUint16(rawTunFlags)
	if err := unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("TUNSETIFF: %w", err)
	}
	if err := unix.SetNonblock(fd, false); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), "/dev/net/tun"), nil
}

func startRawTUN() error {
	runCmdSilent("ip", "link", "del", rawIfaceName)
	time.Sleep(50 * time.Millisecond)

	mtu := wgMTU
	if mtu < 576 {
		mtu = rawDefaultMTU
	}
	dev, err := createRawTUNFile(rawIfaceName)
	if err != nil {
		return fmt.Errorf("CreateTUN %s: %w", rawIfaceName, err)
	}
	name := rawIfaceName
	for _, cmd := range [][]string{
		{"ip", "addr", "add", rawServerCIDR, "dev", name},
		{"ip", "link", "set", "mtu", strconv.Itoa(mtu), "dev", name},
		{"ip", "link", "set", name, "up"},
	} {
		out, err := runCmd(cmd[0], cmd[1:]...)
		if err != nil && !strings.Contains(out, "File exists") {
			dev.Close()
			return fmt.Errorf("%s: %s", strings.Join(cmd, " "), out)
		}
	}
	if err := setupRawNAT(name); err != nil {
		dev.Close()
		return err
	}
	// MSS/DF для 10.70/16 (раньше clamp был только на WG 10.66 → speedtest blackhole).
	if out, err := exec.Command("/usr/local/bin/wdtt-mtu-rules.sh", "up").CombinedOutput(); err != nil {
		log.Printf("[RAW] wdtt-mtu-rules.sh up: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	rawTunDev = dev
	rawDownlinkStarted.Do(func() {
		go rawTUNDownlinkLoop()
		go rawSessionReaperLoop()
		if os.Getenv("WDTT_RAW_BLOB") == "1" {
			go rawBlobServerLoop()
		}
	})
	log.Printf("[RAW] TUN %s поднят (%s), MTU %d", name, rawServerCIDR, mtu)
	return nil
}

// rawBlobServerLoop — HTTP blob на 10.70.66.1:18080 для uspeed/bench без hairpin через VK.
// Только внутри RAW subnet (слушает rawServerAddr).
func rawBlobServerLoop() {
	slots := make(chan struct{}, 4)
	mux := http.NewServeMux()
	mux.HandleFunc("/blob", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		n := int64(1 << 20) // 1 MiB default
		if s := r.URL.Query().Get("bytes"); s != "" {
			if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > 0 {
				if v > 16<<20 {
					v = 16 << 20
				}
				n = v
			}
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
		w.WriteHeader(http.StatusOK)
		buf := make([]byte, 32<<10)
		var sent int64
		for sent < n {
			chunk := int64(len(buf))
			if n-sent < chunk {
				chunk = n - sent
			}
			if _, err := w.Write(buf[:chunk]); err != nil {
				return
			}
			sent += chunk
		}
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok\n")
	})
	addr := net.JoinHostPort(rawServerAddr, "18080")
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("[RAW-BLOB] listen %s: %v", addr, err)
		return
	}
	log.Printf("[RAW-BLOB] http://%s/blob?bytes=N", addr)
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       15 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Printf("[RAW-BLOB] serve: %v", err)
	}
}

func setupRawNAT(iface string) error {
	_ = os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644)
	ext := getDefaultInterface()
	log.Printf("[RAW-NAT] Внешний: %s", ext)
	if commandExists("iptables") {
		// Сносим и старый /24, и текущий /16 (comment тот же).
		for _, subnet := range []string{rawSubnet, "10.70.66.0/24"} {
			for i := 0; i < 5; i++ {
				_ = exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", subnet, "-o", ext,
					"-m", "comment", "--comment", rawIPTComment, "-j", "MASQUERADE").Run()
			}
		}
		for i := 0; i < 5; i++ {
			_ = exec.Command("iptables", "-D", "FORWARD", "-i", iface, "-m", "comment", "--comment", rawIPTComment, "-j", "ACCEPT").Run()
			_ = exec.Command("iptables", "-D", "FORWARD", "-o", iface, "-m", "comment", "--comment", rawIPTComment, "-j", "ACCEPT").Run()
			_ = exec.Command("iptables", "-D", "INPUT", "-i", iface, "-m", "comment", "--comment", rawIPTComment, "-j", "ACCEPT").Run()
		}
		_ = exec.Command("iptables", "-t", "nat", "-I", "POSTROUTING", "1", "-s", rawSubnet, "-o", ext,
			"-m", "comment", "--comment", rawIPTComment, "-j", "MASQUERADE").Run()
		_ = exec.Command("iptables", "-I", "FORWARD", "1", "-i", iface, "-m", "comment", "--comment", rawIPTComment, "-j", "ACCEPT").Run()
		_ = exec.Command("iptables", "-I", "FORWARD", "1", "-o", iface, "-m", "comment", "--comment", rawIPTComment, "-j", "ACCEPT").Run()
		// Без INPUT ACCEPT UFW/policy DROP режет local (ping/HTTP на 10.70.66.1) — как wdtt0.
		_ = exec.Command("iptables", "-I", "INPUT", "1", "-i", iface, "-m", "comment", "--comment", rawIPTComment, "-j", "ACCEPT").Run()
		return nil
	}
	log.Printf("[RAW-NAT] WARNING: iptables нет — NAT для %s может не работать", rawSubnet)
	return nil
}

// getNextRawIP выдаёт свободный 10.70.x.y из пула.
func getNextRawIP() string {
	used := make(map[string]bool)
	rawSessionsByIP.Range(func(k, _ any) bool {
		if ip, ok := k.(string); ok {
			used[ip] = true
		}
		return true
	})
	rawIPByDevice.Range(func(_, v any) bool {
		if ip, ok := v.(string); ok {
			used[ip] = true
		}
		return true
	})
	for attempt := 0; attempt < 65000; attempt++ {
		n := rawIPSeq.Add(1)
		b3 := int((n / 254) % 256)
		b4 := int(n%254) + 1 // 1..254
		ip := fmt.Sprintf("10.70.%d.%d", b3, b4)
		if ip == rawServerAddr || used[ip] {
			continue
		}
		return ip
	}
	return ""
}

// getRawIPForDevice — один IP на device: все воркеры шарят его → multipath по TURN.
func getRawIPForDevice(deviceID string) string {
	if deviceID == "" {
		deviceID = "unknown"
	}
	if v, ok := rawIPByDevice.Load(deviceID); ok {
		if ip, _ := v.(string); ip != "" {
			return ip
		}
	}
	ip := getNextRawIP()
	if ip == "" {
		return ""
	}
	actual, _ := rawIPByDevice.LoadOrStore(deviceID, ip)
	if s, ok := actual.(string); ok && s != "" {
		return s
	}
	return ip
}

func rawDeviceIdentity(deviceID, password string) string {
	sum := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%s:%x", deviceID, sum[:8])
}

func (g *rawSessionGroup) add(sess *rawSession) {
	if !g.tryAdd(sess, 0) {
		panic("unreachable: unlimited rawSessionGroup.add rejected")
	}
}

func (g *rawSessionGroup) tryAdd(sess *rawSession, limit int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if limit > 0 && len(g.sessions) >= limit {
		return false
	}
	if sess.lastSeen.Load() == 0 {
		sess.lastSeen.Store(time.Now().UnixNano())
	}
	g.sessions = append(g.sessions, sess)
	if g.deviceID == "" {
		g.deviceID = sess.deviceID
	}
	if g.flowAff == nil {
		g.flowAff = make(map[uint64]*rawSession)
		g.flowExp = make(map[uint64]int64)
	}
	if g.reorder == nil {
		g.reorder = newRawReorder()
	}
	// Store под замком: иначе remove→Delete может выкинуть группу, в которую мы только что вступили.
	if g.ip != "" {
		rawSessionsByIP.Store(g.ip, g)
	}
	return true
}

func (g *rawSessionGroup) liveSessionsLocked(now time.Time) []*rawSession {
	live := make([]*rawSession, 0, len(g.sessions))
	cutoff := now.Add(-rawSessionFreshTTL).UnixNano()
	for _, sess := range g.sessions {
		if sess == nil || sess.downCh == nil {
			continue
		}
		if sess.lastSeen.Load() < cutoff {
			if sess.cancel != nil {
				sess.cancel()
			}
			continue
		}
		live = append(live, sess)
	}
	return live
}

func (g *rawSessionGroup) cancelStale(now time.Time) {
	cutoff := now.Add(-rawSessionFreshTTL).UnixNano()
	var stale []context.CancelFunc
	g.mu.Lock()
	for _, sess := range g.sessions {
		if sess != nil && sess.lastSeen.Load() < cutoff && sess.cancel != nil {
			stale = append(stale, sess.cancel)
		}
	}
	g.mu.Unlock()
	for _, cancel := range stale {
		cancel()
	}
}

func rawSessionReaperLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for now := range ticker.C {
		rawSessionsByIP.Range(func(_, value any) bool {
			if group, ok := value.(*rawSessionGroup); ok && group != nil {
				group.cancelStale(now)
			}
			return true
		})
	}
}

func (g *rawSessionGroup) remove(sess *rawSession) (empty bool) {
	g.mu.Lock()
	for i, s := range g.sessions {
		if s == sess {
			g.sessions = append(g.sessions[:i], g.sessions[i+1:]...)
			break
		}
	}
	for k, s := range g.flowAff {
		if s == sess {
			delete(g.flowAff, k)
			delete(g.flowExp, k)
		}
	}
	ch := sess.downCh
	sess.downCh = nil
	empty = len(g.sessions) == 0
	if empty && g.ip != "" {
		rawSessionsByIP.CompareAndDelete(g.ip, g)
	}
	g.mu.Unlock()
	if ch != nil {
		close(ch)
	}
	return empty
}

// rawFlowKey — XOR src/dst + ports: одинаков для uplink и downlink одного TCP-потока.
func rawFlowKey(pkt []byte) uint64 {
	if len(pkt) < 20 || pkt[0]>>4 != 4 {
		var h uint32
		for i := 0; i < len(pkt) && i < 64; i++ {
			h = h*131 + uint32(pkt[i])
		}
		return uint64(h)
	}
	ihl := int(pkt[0]&0x0f) * 4
	h := binary.BigEndian.Uint32(pkt[12:16]) ^ binary.BigEndian.Uint32(pkt[16:20])
	h ^= uint32(pkt[9]) * 0x9e3779b9
	if len(pkt) >= ihl+4 && (pkt[9] == 6 || pkt[9] == 17) {
		a := binary.BigEndian.Uint16(pkt[ihl : ihl+2])
		b := binary.BigEndian.Uint16(pkt[ihl+2 : ihl+4])
		if a > b {
			a, b = b, a
		}
		// min|max — симметрично up/down; multiply размазывает биты под %N.
		h ^= (uint32(a)<<16 | uint32(b)) * 0x9e3779b9
	}
	return uint64(h)
}

func rawIPv4SourceMatches(pkt []byte, assignedIP string) bool {
	if len(pkt) < 20 || pkt[0]>>4 != 4 {
		return false
	}
	ihl := int(pkt[0]&0x0f) * 4
	total := int(binary.BigEndian.Uint16(pkt[2:4]))
	if ihl < 20 || total < ihl || total > len(pkt) {
		return false
	}
	expected := net.ParseIP(assignedIP).To4()
	return expected != nil && net.IP(pkt[12:16]).Equal(expected)
}

func (g *rawSessionGroup) expireFlowsLocked(now int64) {
	if len(g.flowAff) < rawFlowAffMax {
		return
	}
	for k, exp := range g.flowExp {
		if now > exp {
			delete(g.flowAff, k)
			delete(g.flowExp, k)
		}
	}
	if len(g.flowAff) >= rawFlowAffMax {
		for k := range g.flowAff {
			delete(g.flowAff, k)
			delete(g.flowExp, k)
			break
		}
	}
}

// noteUplink закрепляет 5-tuple на DTLS-сессию, которая реально приняла пакет.
// Без этого downlink hash%N часто уходит в другой TURN → асимметрия → TCP download ~1–2 Мбит.
func (g *rawSessionGroup) noteUplink(sess *rawSession, pkt []byte) {
	if g == nil || sess == nil || len(pkt) < 20 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.flowAff == nil {
		g.flowAff = make(map[uint64]*rawSession)
		g.flowExp = make(map[uint64]int64)
	}
	now := time.Now().UnixNano()
	key := rawFlowKey(pkt)
	g.expireFlowsLocked(now)
	g.flowAff[key] = sess
	g.flowExp[key] = now + int64(rawFlowAffTTL)
}

func (g *rawSessionGroup) pickForLocked(pkt []byte) *rawSession {
	nowTime := time.Now()
	liveSessions := g.liveSessionsLocked(nowTime)
	n := len(liveSessions)
	if n == 0 {
		return nil
	}
	now := nowTime.UnixNano()
	key := rawFlowKey(pkt)
	if s, ok := g.flowAff[key]; ok {
		if exp, ok2 := g.flowExp[key]; ok2 && now <= exp {
			for _, live := range liveSessions {
				if live == s {
					g.flowExp[key] = now + int64(rawFlowAffTTL)
					return s
				}
			}
		}
		delete(g.flowAff, key)
		delete(g.flowExp, key)
	}
	// Нет uplink-affinity (первый пакет — downlink): hash%N, один TCP = один TURN.
	sess := liveSessions[uint32(key)%uint32(n)]
	g.expireFlowsLocked(now)
	g.flowAff[key] = sess
	g.flowExp[key] = now + int64(rawFlowAffTTL)
	return sess
}

func rawDownChunkSizeFor(pktSize int) int {
	switch {
	case pktSize > 1100:
		return 64
	case pktSize >= 701:
		return 24
	case pktSize >= 301:
		return 8
	case pktSize >= 101:
		return 3
	default:
		return 1
	}
}

// enqueueDown кладёт downlink-пакет в очередь сессии.
// ok=false при drop (нет сессии / downCh полон).
func (g *rawSessionGroup) enqueueDown(pkt []byte) (ok bool, nbytes int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []byte
	var sess *rawSession
	now := time.Now()
	key := rawFlowKey(pkt)
	if affinity, ok := g.flowAff[key]; ok && affinity != nil &&
		!affinity.framed.Load() && !affinity.chunked {
		if exp := g.flowExp[key]; exp >= now.UnixNano() &&
			affinity.lastSeen.Load() >= now.Add(-rawSessionFreshTTL).UnixNano() {
			out = append([]byte(nil), pkt...)
			sess = affinity
			g.flowExp[key] = now.Add(rawFlowAffTTL).UnixNano()
		}
	}
	if sess == nil {
		liveSessions := g.liveSessionsLocked(now)
		chunkedSessions := make([]*rawSession, 0, len(liveSessions))
		for _, candidate := range liveSessions {
			if candidate.chunked {
				chunkedSessions = append(chunkedSessions, candidate)
			}
		}
		if n := len(chunkedSessions); n > 0 {
			if g.chunkStartedAt.IsZero() {
				g.chunkStartedAt = now
			} else if now.Sub(g.chunkStartedAt) >= rawChunkMaxDwell {
				g.chunkIndex = (g.chunkIndex + 1) % n
				g.chunkCount = 0
				g.chunkStartedAt = now
			}
			if g.chunkIndex >= n {
				g.chunkIndex = 0
			}
			out = append([]byte(nil), pkt...)
			start := g.chunkIndex
			for i := 0; i < n; i++ {
				idx := (start + i) % n
				candidate := chunkedSessions[idx]
				if candidate == nil || candidate.downCh == nil {
					continue
				}
				select {
				case candidate.downCh <- out:
					g.chunkIndex = idx
					g.chunkCount++
					if g.chunkCount >= rawDownChunkSizeFor(len(pkt)) {
						g.chunkIndex = (idx + 1) % n
						g.chunkCount = 0
						g.chunkStartedAt = now
					}
					return true, len(out)
				default:
				}
			}
			return false, len(out)
		}
	}
	framedRecent := g.framed.Load() &&
		now.UnixNano()-g.framedLastSeen.Load() <= int64(rawFramedModeTTL)
	if sess == nil && framedRecent {
		liveSessions := g.liveSessionsLocked(now)
		framedSessions := make([]*rawSession, 0, len(liveSessions))
		for _, candidate := range liveSessions {
			if candidate.framed.Load() {
				framedSessions = append(framedSessions, candidate)
			}
		}
		n := len(framedSessions)
		if n > 0 {
			seq := g.outSeq.Next()
			out = rawFrameEncode(seq, pkt)
			chunk := g.rr / rawMPChunkPackets
			g.rr++
			sess = framedSessions[chunk%uint32(n)]
		}
	}
	if sess == nil {
		out = append([]byte(nil), pkt...)
		sess = g.pickForLocked(pkt)
	}
	if sess == nil || sess.downCh == nil {
		return false, len(out)
	}
	select {
	case sess.downCh <- out:
		return true, len(out)
	default:
		// переполнен — дроп лучше, чем стоп всего TUN read
		return false, len(out)
	}
}

func (g *rawSessionGroup) len() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.sessions)
}

func (g *rawSessionGroup) first() *rawSession {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.sessions) == 0 {
		return nil
	}
	return g.sessions[0]
}

func buildRawClientConfig(clientIP string, mtu int) string {
	if mtu < 576 {
		mtu = rawDefaultMTU
	}
	return fmt.Sprintf("IP = %s\nDNS = %s\nMTU = %d\nCAPS = CHUNK1\n", clientIP, clientDNS, mtu)
}

func rawRequestHasChunk1(firstStr string) bool {
	body := strings.TrimSpace(firstStr)
	switch {
	case strings.HasPrefix(body, "RAWCONF:"):
		body = strings.TrimPrefix(body, "RAWCONF:")
	case strings.HasPrefix(body, "GETCONF_RAW:"):
		body = strings.TrimPrefix(body, "GETCONF_RAW:")
	default:
		return false
	}
	parts := strings.Split(strings.TrimSpace(body), "|")
	// RAWCONF: device|pass|mtu|CAPS... — CAPS только после MTU (index >= 3)
	for i := 3; i < len(parts); i++ {
		if strings.EqualFold(strings.TrimSpace(parts[i]), "CHUNK1") {
			return true
		}
		upper := strings.ToUpper(strings.TrimSpace(parts[i]))
		if strings.HasPrefix(upper, "CAPS=") && strings.Contains(upper, "CHUNK1") {
			return true
		}
	}
	return false
}

func parseRawConfRequest(firstStr string) (deviceID, password string, mtu int, ok bool) {
	mtu = wgMTU
	if mtu < 576 {
		mtu = rawDefaultMTU
	}
	switch {
	case strings.HasPrefix(firstStr, "RAWCONF:"):
		parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(firstStr, "RAWCONF:")), "|")
		if len(parts) < 2 {
			return "", "", 0, false
		}
		deviceID = strings.TrimSpace(parts[0])
		password = strings.TrimSpace(parts[1])
		if len(parts) > 2 {
			if n, err := strconv.Atoi(strings.TrimSpace(parts[2])); err == nil && n >= 576 && n <= 1500 {
				mtu = n
			}
		}
		return deviceID, password, mtu, true
	case strings.HasPrefix(firstStr, "GETCONF_RAW:"):
		parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(firstStr, "GETCONF_RAW:")), "|")
		if len(parts) < 2 {
			return "", "", 0, false
		}
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), mtu, true
	default:
		return "", "", 0, false
	}
}

func handleRawConf(ctx context.Context, clientConn net.Conn, firstStr, wrapAuthPass string) {
	if !rawModeEnabled.Load() {
		_, _ = clientConn.Write([]byte("NOCONF"))
		log.Printf("[RAW] Отказ: RAW выключен в панели (от %s)", clientConn.RemoteAddr())
		return
	}
	deviceID, password, mtu, ok := parseRawConfRequest(firstStr)
	if !ok {
		_, _ = clientConn.Write([]byte("DENIED:wrong_password"))
		log.Printf("[RAW] Отказ: плохой RAWCONF от %s: %q", clientConn.RemoteAddr(), trimPreview(firstStr, 48))
		return
	}
	if deviceID == "" {
		deviceID = "unknown"
	}
	if wrapAuthPass != "" {
		if password == "" {
			password = wrapAuthPass
		} else if password != wrapAuthPass {
			_, _ = clientConn.Write([]byte("DENIED:wrong_password"))
			log.Printf("[RAW] Отказ: password != WRAP auth from %s", clientConn.RemoteAddr())
			return
		}
	}

	dbMutex.Lock()
	isMainPass := password != "" && password == db.MainPassword
	entry, isGenPass := db.Passwords[password]
	valid := isMainPass || (isGenPass && !isPasswordExpired(entry) && !isTrafficExceeded(entry))
	if valid && isGenPass && entry.IsDeactivated {
		_, _ = clientConn.Write([]byte("DENIED:deactivated"))
		dbMutex.Unlock()
		return
	}
	if valid && isGenPass && !isMainPass && !entryCanAcceptDevice(entry, deviceID) {
		_, _ = clientConn.Write([]byte("DENIED:device_mismatch"))
		dbMutex.Unlock()
		return
	}
	if !valid {
		if isGenPass && isTrafficExceeded(entry) {
			_, _ = clientConn.Write([]byte("DENIED:traffic_exceeded"))
		} else if isGenPass && isPasswordExpired(entry) {
			_, _ = clientConn.Write([]byte("DENIED:expired"))
		} else {
			_, _ = clientConn.Write([]byte("DENIED:wrong_password"))
		}
		dbMutex.Unlock()
		return
	}
	if isGenPass && !isMainPass && !entryHasDevice(entry, deviceID) {
		if bindDeviceToEntry(entry, deviceID) {
			_ = persistUserBindingsSQLiteLocked(password, entry)
		}
	}
	if isMainPass {
		ensureMainPasswordEntryLocked()
	}
	dbMutex.Unlock()

	if err := ensureRawTUN(); err != nil {
		log.Printf("[RAW] TUN: %v", err)
		_, _ = clientConn.Write([]byte("NOCONF"))
		return
	}

	ip := getRawIPForDevice(rawDeviceIdentity(deviceID, password))
	if ip == "" {
		_, _ = clientConn.Write([]byte("NOCONF"))
		log.Printf("[RAW] Отказ: пул IP исчерпан (от %s)", clientConn.RemoteAddr())
		return
	}

	resp := buildRawClientConfig(ip, mtu)
	if _, err := clientConn.Write([]byte(resp)); err != nil {
		return
	}
	log.Printf("[RAW] Сессия %s зарегистрирована (ip=%s, shared=true) from %s", deviceID, ip, clientConn.RemoteAddr())

	runRawRelay(ctx, clientConn, deviceID, ip, password, isMainPass, rawRequestHasChunk1(firstStr))
}

func runRawRelay(ctx context.Context, clientConn net.Conn, deviceID, ip, password string, isMain, chunked bool) {
	sessionIdentity := rawDeviceIdentity(deviceID, password)
	if rawActiveSessions.Add(1) > rawMaxSessionsGlobal {
		rawActiveSessions.Add(-1)
		log.Printf("[RAW] Отказ: глобальный лимит сессий %d", rawMaxSessionsGlobal)
		return
	}
	defer rawActiveSessions.Add(-1)

	pctx, pcancel := context.WithCancel(ctx)
	defer pcancel()
	downBuf := rawDownChBuf
	if chunked {
		downBuf = 256
	}

	sess := &rawSession{
		deviceID:   deviceID,
		registryID: sessionIdentity,
		ip:         ip,
		password:   password,
		isMain:     isMain,
		conn:       clientConn,
		cancel:     pcancel,
		downCh:     make(chan []byte, downBuf),
		chunked:    chunked,
	}

	group := &rawSessionGroup{ip: ip, deviceID: deviceID}
	if existing, loaded := rawSessionsByIP.LoadOrStore(ip, group); loaded {
		if g, ok := existing.(*rawSessionGroup); ok && g != nil {
			group = g
		} else {
			rawSessionsByIP.Store(ip, group)
		}
	}
	if !group.tryAdd(sess, rawMaxSessionsPerIdentity) {
		log.Printf("[RAW] Отказ: лимит %d сессий для identity device=%s", rawMaxSessionsPerIdentity, deviceID)
		return
	}
	downCh := sess.downCh
	go rawSessionDownWriter(sess, downCh)
	defer func() {
		_ = group.remove(sess) // CompareAndDelete внутри, под замком группы
		log.Printf("[RAW] Сессия %s (ip=%s) завершена (осталось в группе: %d)", deviceID, ip, group.len())
	}()

	if !userSessionEnter(sessionIdentity, ip, userLabel(password, isMain), password) {
		log.Printf("[RAW] Отказ: лимит DTLS-сессий для device=%s", deviceID)
		return
	}
	defer userSessionLeave(sessionIdentity)
	relaySess := relaySessionRegister(sessionIdentity, pcancel)
	defer relaySessionUnregister(sessionIdentity, relaySess)

	// Как WG flushTraffic: начисление + mid-session отказ по квоте/expiry/deactivate.
	flushRaw := func() bool {
		up := atomic.SwapInt64(&sess.up, 0)
		down := atomic.SwapInt64(&sess.down, 0)
		dbMutex.Lock()
		defer dbMutex.Unlock()
		pass := resolveTrafficPassword(password, isMain, deviceID)
		if pass == "" {
			return true
		}
		e, ok := db.Passwords[pass]
		if !ok || e == nil {
			return true
		}
		if isPasswordExpired(e) || e.IsDeactivated || isTrafficExceeded(e) {
			return false
		}
		okAcc := true
		if up > 0 && !addTrafficLocked(pass, up, false) {
			okAcc = false
		}
		if down > 0 && !addTrafficLocked(pass, down, true) {
			okAcc = false
		}
		return okAcc
	}
	defer flushRaw()

	flush := time.NewTicker(time.Second)
	defer flush.Stop()
	go func() {
		for {
			select {
			case <-pctx.Done():
				flushRaw()
				return
			case <-flush.C:
				if !flushRaw() {
					pcancel()
					return
				}
			}
		}
	}()

	context.AfterFunc(pctx, func() {
		_ = clientConn.SetDeadline(time.Now())
	})

	buf := make([]byte, 2048)
	for {
		select {
		case <-pctx.Done():
			return
		default:
		}
		_ = clientConn.SetReadDeadline(time.Now().Add(30 * time.Minute))
		n, err := clientConn.Read(buf)
		if err != nil {
			return
		}
		sess.lastSeen.Store(time.Now().UnixNano())
		if n == 1 && buf[0] == 0xFF {
			if relaySess != nil {
				relaySess.touch()
			}
			userTouchActivity(sessionIdentity)
			sess.writeMu.Lock()
			_, _ = clientConn.Write([]byte{0xFF})
			sess.writeMu.Unlock()
			continue
		}
		userTouchActivity(sessionIdentity)
		if relaySess != nil {
			relaySess.touch()
		}
		atomic.AddInt64(&sess.up, int64(n))
		atomic.AddInt64(&totalBytesFromClient, int64(n))

		rawTunMu.Lock()
		dev := rawTunDev
		rawTunMu.Unlock()
		if dev == nil {
			return
		}

		if isRawFrame(buf[:n]) {
			sess.framed.Store(true)
			if !group.framed.Load() {
				group.framed.Store(true)
				log.Printf("[RAW] device=%s multipath RA-frame включён", deviceID)
			}
			group.framedLastSeen.Store(time.Now().UnixNano())
			_, ipPkt, ok := rawFrameDecode(buf[:n])
			if !ok || !rawIPv4SourceMatches(ipPkt, sess.ip) {
				continue
			}
			// IP допускает out-of-order delivery; TCP сам восстановит порядок.
			// RA reorder без таймера мог навсегда удержать хвост после дырки.
			group.noteUplink(sess, ipPkt)
			if err := writeRawTUN(dev, ipPkt); err != nil {
				log.Printf("[RAW] Ошибка записи в TUN (ip=%s): %v", ip, err)
				return
			}
			continue
		}
		if !rawIPv4SourceMatches(buf[:n], sess.ip) {
			continue // не IPv4 и не frame
		}
		group.noteUplink(sess, buf[:n])
		if err := writeRawTUN(dev, buf[:n]); err != nil {
			log.Printf("[RAW] Ошибка записи в TUN (ip=%s): %v", ip, err)
			return
		}
	}
}

func writeRawTUN(dev *os.File, pkt []byte) error {
	if dev == nil || len(pkt) == 0 {
		return nil
	}
	rawTunWriteMu.Lock()
	_, err := dev.Write(pkt)
	rawTunWriteMu.Unlock()
	return err
}

func rawSessionDownWriter(sess *rawSession, downCh <-chan []byte) {
	if downCh == nil {
		return
	}
	for pkt := range downCh {
		if sess.conn == nil {
			continue
		}
		sess.writeMu.Lock()
		_, err := sess.conn.Write(pkt)
		sess.writeMu.Unlock()
		if err != nil {
			if sess.cancel != nil {
				sess.cancel()
			}
			return
		}
		atomic.AddInt64(&sess.down, int64(len(pkt)))
		atomic.AddInt64(&totalBytesToClient, int64(len(pkt)))
		userTouchActivity(sess.registryID)
	}
}

func rawTUNDownlinkLoop() {
	buf := make([]byte, 2048)
	var readN, enqN, dropN, missN, enqBytes atomic.Uint64
	lastLog := time.Now()
	for {
		rawTunMu.Lock()
		dev := rawTunDev
		rawTunMu.Unlock()
		if dev == nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		n, err := dev.Read(buf)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if n < 20 {
			continue
		}
		readN.Add(1)
		pkt := buf[:n]
		if pkt[0]>>4 != 4 {
			continue
		}
		dst := net.IPv4(pkt[16], pkt[17], pkt[18], pkt[19]).String()
		v, ok := rawSessionsByIP.Load(dst)
		if !ok {
			missN.Add(1)
			continue
		}
		group, _ := v.(*rawSessionGroup)
		if group == nil {
			missN.Add(1)
			continue
		}
		okEnq, nbytes := group.enqueueDown(pkt)
		if okEnq {
			enqN.Add(1)
			enqBytes.Add(uint64(nbytes))
		} else {
			dropN.Add(1)
		}
		if time.Since(lastLog) > 5*time.Second {
			lastLog = time.Now()
			eb := enqBytes.Swap(0)
			log.Printf("[RAW-DOWN] read=%d enq=%d drop=%d miss=%d bytes=%d",
				readN.Swap(0), enqN.Swap(0), dropN.Swap(0), missN.Swap(0), eb)
		}
	}
}

func trimPreview(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func isRawConfPacket(firstStr string) bool {
	return strings.HasPrefix(firstStr, "RAWCONF:") || strings.HasPrefix(firstStr, "GETCONF_RAW:")
}

// cancelRawSessionsForPassword рвёт все RAW-сессии пароля (аналог removePeerFromWG).
func cancelRawSessionsForPassword(password string) int {
	password = strings.TrimSpace(password)
	if password == "" {
		return 0
	}
	n := 0
	rawSessionsByIP.Range(func(_, v any) bool {
		group, ok := v.(*rawSessionGroup)
		if !ok || group == nil {
			return true
		}
		var cancels []context.CancelFunc
		group.mu.Lock()
		for _, sess := range group.sessions {
			if sess == nil || sess.cancel == nil {
				continue
			}
			if sess.password == password {
				cancels = append(cancels, sess.cancel)
			}
		}
		group.mu.Unlock()
		for _, c := range cancels {
			c()
			n++
		}
		return true
	})
	return n
}

// cancelRawSessionsForDevice рвёт RAW-сессии deviceID (как снос WG peer).
func cancelRawSessionsForDevice(deviceID string) int {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return 0
	}
	n := 0
	rawSessionsByIP.Range(func(_, v any) bool {
		group, ok := v.(*rawSessionGroup)
		if !ok || group == nil {
			return true
		}
		var cancels []context.CancelFunc
		group.mu.Lock()
		for _, sess := range group.sessions {
			if sess == nil || sess.cancel == nil {
				continue
			}
			if sess.deviceID == deviceID || strings.HasPrefix(sess.registryID, deviceID+":") {
				cancels = append(cancels, sess.cancel)
			}
		}
		group.mu.Unlock()
		for _, c := range cancels {
			c()
			n++
		}
		return true
	})
	return n
}
