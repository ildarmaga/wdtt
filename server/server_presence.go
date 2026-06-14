package main

import (
	"context"
	"log"
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

func refreshWGActivity() {
	peers, err := collectWGPeerInfos()
	if err != nil {
		return
	}
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
		if ip == "" {
			ip = peer.allowedIP
		}
		// Только rx (приём от клиента). tx растёт от серверного PersistentKeepalive
		// даже когда клиент уже отключился — это держало бы ложный онлайн.
		list = append(list, resolved{id: id, ip: ip, pass: pass, isMain: isMain, bytes: peer.rx, handshake: peer.lastHandshake})
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
			// Первая выборка: точка отсчёта — момент последнего handshake (если был),
			// чтобы старый мёртвый пир не считался онлайн после рестарта.
			init := time.Time{}
			if r.handshake > 0 {
				init = time.Unix(r.handshake, 0)
			}
			wgActivity[r.id] = &wgActivitySample{bytes: r.bytes, lastChange: init, label: label, ip: r.ip, password: r.pass}
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
	// Их DTLS-relay сессии глушим сразу, не дожидаясь 3-мин idle-эвикта.
	for id, s := range wgActivity {
		if s != nil && !s.lastChange.IsZero() && now.Sub(s.lastChange) >= timeout {
			offline = append(offline, id)
		}
	}
	wgActivityMu.Unlock()

	for _, id := range offline {
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
	// DTLS keepalive клиента = 15s; таймаут онлайн должен быть чуть больше, иначе ложный offline.
	if sec < 20 {
		sec = 20
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
	if sec < int64(keepalive)+10 {
		sec = int64(keepalive) + 10
	}
	return sec
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
	defer onlineUsersMutex.Unlock()

	info, exists := onlineUsers[deviceID]
	if !exists {
		info = &onlineUserInfo{Label: label, Password: password, IP: ip, LastActivity: now}
		onlineUsers[deviceID] = info
	}
	if maxDTLSPerDevice > 0 && info.Sessions >= int(maxDTLSPerDevice) {
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

func userPresenceSweep() {
	now := time.Now()
	var stale []string

	onlineUsersMutex.Lock()
	for deviceID, info := range onlineUsers {
		if info.Sessions > 0 && now.Sub(info.LastActivity) >= userOnlineTimeoutDuration() {
			stale = append(stale, deviceID)
		}
	}
	onlineUsersMutex.Unlock()

	for _, deviceID := range stale {
		onlineUsersMutex.Lock()
		info, exists := onlineUsers[deviceID]
		if !exists || info.Sessions <= 0 || now.Sub(info.LastActivity) < userOnlineTimeoutDuration() {
			onlineUsersMutex.Unlock()
			continue
		}
		label, ip, sessions := info.Label, info.IP, info.Sessions
		password := info.Password
		delete(onlineUsers, deviceID)
		onlineUsersMutex.Unlock()
		atomic.AddInt32(&activeUsers, -1)
		evicted := relayEvictDevice(deviceID)
		log.Printf("[ОТКЛ] stale %s | %s | WG %s (sessions=%d, relay_evict=%d)", label, deviceID, ip, sessions, evicted)
		if password != "" {
			touchUserLastSeen(password)
		}
	}
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

// snapshotOnlineUsers — онлайн для панели по дельте трафика WG-пира.
// DTLS keepalive НЕ учитывается: он идёт мимо WG и не отражает реальный туннель.
func snapshotOnlineUsers() []map[string]string {
	now := time.Now()
	timeout := time.Duration(wgPeerOnlineMaxAgeSec()) * time.Second
	wgActivityMu.Lock()
	defer wgActivityMu.Unlock()
	result := make([]map[string]string, 0, len(wgActivity))
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
		})
	}
	return result
}

// syncTrafficFromWGPeers начисляет трафик по дельте rx/tx WireGuard-пиров.
