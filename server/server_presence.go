package server

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.zx2c4.com/wireguard/device"
)

// Онлайн определяется по дельте трафика WG (rx+tx), а НЕ по DTLS keepalive:
// keepalive relay-ядра идёт мимо WG и держал бы «призрак» онлайн после отключения VPN.
// Подключённый (даже простаивающий) клиент шлёт WG PersistentKeepalive каждые keepalive сек,
// что двигает счётчики; при отключении туннеля трафик замирает → офлайн по таймауту.
type wgActivitySample struct {
	bytes      int64
	lastChange time.Time
	label      string
	ip         string
	password   string
}

var (
	wgActivity   = make(map[string]*wgActivitySample)
	wgActivityMu sync.Mutex
)

func clearWGActivity() {
	wgActivityMu.Lock()
	wgActivity = make(map[string]*wgActivitySample)
	wgActivityMu.Unlock()
}

func clearOnlineUsers() {
	onlineUsersMutex.Lock()
	onlineUsers = make(map[string]*onlineUserInfo)
	onlineUsersMutex.Unlock()
	atomic.StoreInt32(&activeUsers, 0)
}

func refreshWGActivity() {
	peers, err := collectWGPeerInfos()
	if err != nil {
		return
	}
	refreshWGActivityFromPeers(peers)
}

func refreshWGActivityFromPeers(peers []wgPeerInfo) {
	now := time.Now()

	type resolved struct {
		id, ip, pass string
		isMain       bool
		bytes        int64
		handshake    int64
	}
	var list []resolved
	dbMutex.Lock()
	for _, peer := range peers {
		id, ip, pass, isMain := deviceForWGPeerLocked(peer)
		if id == "" {
			continue
		}
		if pass == "" {
			pass = bindOrphanDeviceToMainLocked(id)
		}
		if pass == "" {
			continue
		}
		if pass == db.MainPassword {
			isMain = true
		}
		if ip == "" {
			ip = peer.allowedIP
		}
		// Онлайн по реальному трафику: rx (от клиента, ACK/keepalive) + tx (загрузка
		// на клиент). Счётчики берём из wg show dump если IPC userspace-WG их не отдаёт.
		list = append(list, resolved{id: id, ip: ip, pass: pass, isMain: isMain, bytes: peer.rx + peer.tx, handshake: peer.lastHandshake})
	}
	dbMutex.Unlock()

	timeout := time.Duration(wgPeerOnlineMaxAgeSec()) * time.Second
	var offline []string

	wgActivityMu.Lock()
	seen := make(map[string]struct{}, len(list))
	for _, r := range list {
		seen[r.id] = struct{}{}
		label := userLabel(r.pass, r.isMain)
		cur, ok := wgActivity[r.id]
		if !ok {
			// Не ставим lastChange=now для всех пиров из конфига: после рестарта в WG
			// остаются все устройства из БД, но offline-пиры не должны сразу быть «онлайн».
			// Онлайн: свежий WG handshake или рост rx/tx на следующих тиках.
			sample := &wgActivitySample{
				bytes:    r.bytes,
				label:    label,
				ip:       r.ip,
				password: r.pass,
			}
			if r.handshake > 0 && wgPeerRecentlyHandshook(r.handshake, now.Unix()) {
				sample.lastChange = now
			}
			wgActivity[r.id] = sample
			continue
		}
		if r.bytes != cur.bytes {
			cur.bytes = r.bytes
			cur.lastChange = now
		}
		cur.label = label
		cur.ip = r.ip
		cur.password = r.pass
	}
	for id := range wgActivity {
		if _, ok := seen[id]; !ok {
			delete(wgActivity, id)
		}
	}
	// Устройства, у которых WG rx замер дольше таймаута = реальный дисконнект.
	// НО: в режиме RAW WG peer counters не двигаются (трафик мимо WireGuard) —
	// иначе каждые stats-тик убивали все DTLS (лог: wg-idle … relay_evict=N).
	var stale []string
	for id, s := range wgActivity {
		if s != nil && !s.lastChange.IsZero() && now.Sub(s.lastChange) >= timeout {
			stale = append(stale, id)
		}
	}
	wgActivityMu.Unlock()

	for _, id := range stale {
		if deviceSessionCount(id) > 0 {
			wgActivityMu.Lock()
			if s := wgActivity[id]; s != nil {
				s.lastChange = now
			}
			wgActivityMu.Unlock()
			continue
		}
		offline = append(offline, id)
	}

	for _, id := range offline {
		if deviceSessionCount(id) > 0 {
			continue
		}
		if n := relayEvictDevice(id); n > 0 {
			log.Printf("[ОТКЛ] wg-idle %s relay_evict=%d", id, n)
		}
	}
}

var (
	serverWGDev      *device.Device
	serverWGDevMu    sync.RWMutex
	wgTrafficErrOnce sync.Once
)

// DTLS keepalive клиента (PWDTT client/core/session.go).
const clientDTLSKeepaliveSec = 15

// Должен быть больше интервала DTLS keepalive клиента (15s) и запаса на jitter.
const defaultOnlineTimeoutSec = 15

var userOnlineTimeoutSec = defaultOnlineTimeoutSec

func userOnlineTimeoutDuration() time.Duration {
	sec := userOnlineTimeoutSec
	if sec < 5 {
		sec = defaultOnlineTimeoutSec
	}
	if sec > 600 {
		sec = 600
	}
	// DTLS keepalive клиента = 15s; таймаут stale должен пережить 2 ping + jitter.
	minSec := clientDTLSKeepaliveSec*2 + 10
	if sec < minSec {
		sec = minSec
	}
	return time.Duration(sec) * time.Second
}

func wgPeerOnlineMaxAgeSec() int64 {
	sec := int64(userOnlineTimeoutSec)
	if sec < 5 {
		sec = defaultOnlineTimeoutSec
	}
	if sec > 600 {
		sec = 600
	}
	// WG PersistentKeepalive=25s — меньше порога даёт офлайн между keepalive.
	if sec < int64(wgKeepaliveSec)+10 {
		sec = int64(wgKeepaliveSec) + 10
	}
	return sec
}

func wgPeerRecentlyHandshook(handshakeSec, nowUnix int64) bool {
	if handshakeSec <= 0 {
		return false
	}
	return nowUnix-handshakeSec <= wgPeerOnlineMaxAgeSec()
}

type onlineUserInfo struct {
	Label        string
	Password     string
	IP           string
	Sessions     int
	LastActivity time.Time
}

var (
	onlineUsers      = make(map[string]*onlineUserInfo)
	onlineUsersMutex sync.Mutex
)

func userLabel(password string, isMain bool) string {
	if isMain {
		return "main"
	}
	if password == "" {
		return "unknown"
	}
	return maskPassword(password)
}

func countOnlineUsersLocked() int32 {
	var n int32
	for _, info := range onlineUsers {
		if info.Sessions > 0 {
			n++
		}
	}
	return n
}

func deviceSessionCount(deviceID string) int {
	if deviceID == "" {
		return 0
	}
	onlineUsersMutex.Lock()
	defer onlineUsersMutex.Unlock()
	if info, ok := onlineUsers[deviceID]; ok && info != nil {
		return info.Sessions
	}
	return 0
}

// userSessionEnter регистрирует активную DTLS-прокси сессию. false = лимит maxDTLSPerDevice.
func userSessionEnter(deviceID, ip, label, password string) bool {
	if deviceID == "" {
		return false
	}
	now := time.Now()
	onlineUsersMutex.Lock()

	info, exists := onlineUsers[deviceID]
	if !exists {
		info = &onlineUserInfo{Label: label, Password: password, IP: ip, LastActivity: now}
		onlineUsers[deviceID] = info
	}
	if maxDTLSPerDevice > 0 && info.Sessions >= int(maxDTLSPerDevice) {
		onlineUsersMutex.Unlock()
		return false
	}
	wasOnline := info.Sessions > 0
	info.Sessions++
	info.LastActivity = now
	if ip != "" {
		info.IP = ip
	}
	if label != "" {
		info.Label = label
	}
	if password != "" {
		info.Password = password
	}
	if !wasOnline {
		atomic.AddInt32(&activeUsers, 1)
		log.Printf("[ПОДКЛ] %s | %s | WG %s", info.Label, deviceID, info.IP)
	}
	touchPass := password
	onlineUsersMutex.Unlock()
	if touchPass != "" {
		touchUserLastSeen(touchPass)
	}
	return true
}

func userSessionLeave(deviceID string) {
	if deviceID == "" {
		return
	}
	onlineUsersMutex.Lock()
	info, exists := onlineUsers[deviceID]
	if !exists {
		onlineUsersMutex.Unlock()
		return
	}
	info.Sessions--
	if info.Sessions > 0 {
		onlineUsersMutex.Unlock()
		return
	}
	label, ip := info.Label, info.IP
	password := info.Password
	delete(onlineUsers, deviceID)
	onlineUsersMutex.Unlock()
	atomic.AddInt32(&activeUsers, -1)
	log.Printf("[ОТКЛ] %s | %s | WG %s", label, deviceID, ip)
	if password != "" {
		touchUserLastSeen(password)
	}
}

func userTouchActivity(deviceID string) {
	if deviceID == "" {
		return
	}
	now := time.Now()
	onlineUsersMutex.Lock()
	defer onlineUsersMutex.Unlock()
	info, exists := onlineUsers[deviceID]
	if !exists || info.Sessions <= 0 {
		return
	}
	info.LastActivity = now
}

// sessionActivityHeartbeat обновляет LastActivity пока DTLS-сессия жива (до прихода keepalive/WG).
func sessionActivityHeartbeat(ctx context.Context, deviceID string) {
	if deviceID == "" {
		return
	}
	userTouchActivity(deviceID)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			userTouchActivity(deviceID)
		}
	}
}

func userPresenceSweep() {
	// Не снимаем сессии по DTLS idle: keepalive клиента (15s) и heartbeat ниже
	// обновляют LastActivity; ложный stale рвал рабочие подключения.
	// Реальный offline — defer userSessionLeave при закрытии DTLS и wg-idle в refreshWGActivity.
	_ = userOnlineTimeoutDuration()
}

func userPresenceLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			userPresenceSweep()
		}
	}
}

// snapshotOnlineUsers — онлайн для панели: WG-пиры (по трафику) + активные RAW-сессии.
// DTLS keepalive НЕ учитывается: он идёт мимо WG и не отражает реальный туннель.
func snapshotOnlineUsers() []map[string]string {
	now := time.Now()
	timeout := time.Duration(wgPeerOnlineMaxAgeSec()) * time.Second
	wgActivityMu.Lock()
	result := make([]map[string]string, 0, len(wgActivity)+8)
	seen := make(map[string]struct{}, len(wgActivity)+8)
	for deviceID, s := range wgActivity {
		if s == nil || s.lastChange.IsZero() {
			continue
		}
		if now.Sub(s.lastChange) >= timeout {
			continue
		}
		result = append(result, map[string]string{
			"device_id": deviceID,
			"user":      s.label,
			"password":  s.password,
			"ip":        s.ip,
			"sessions":  "1",
			"mode":      "wg",
		})
		seen[deviceID] = struct{}{}
	}
	wgActivityMu.Unlock()

	rawSessionsByIP.Range(func(_, v any) bool {
		group, ok := v.(*rawSessionGroup)
		if !ok || group == nil {
			return true
		}
		sess := group.first()
		if sess == nil {
			return true
		}
		if _, dup := seen[sess.deviceID]; dup {
			return true
		}
		n := group.len()
		result = append(result, map[string]string{
			"device_id": sess.deviceID,
			"user":      userLabel(sess.password, sess.isMain),
			"password":  sess.password,
			"ip":        sess.ip,
			"sessions":  strconv.Itoa(n),
			"mode":      "raw",
		})
		seen[sess.deviceID] = struct{}{}
		return true
	})
	return result
}

func countRawSessions() int {
	n := 0
	rawSessionsByIP.Range(func(_, v any) bool {
		if g, ok := v.(*rawSessionGroup); ok && g != nil {
			n += g.len()
		}
		return true
	})
	return n
}

func forEachOnlinePassword(fn func(string)) {
	if fn == nil {
		return
	}
	wgActivityMu.Lock()
	defer wgActivityMu.Unlock()
	for _, s := range wgActivity {
		if s == nil {
			continue
		}
		if pass := strings.TrimSpace(s.password); pass != "" {
			fn(pass)
		}
	}
}

// syncTrafficFromWGPeers начисляет трафик по дельте rx/tx WireGuard-пиров.
