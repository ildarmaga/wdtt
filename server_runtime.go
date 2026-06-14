package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/ipc"
	"golang.zx2c4.com/wireguard/tun"
)

// ==================== Пул буферов ====================

var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 1600)
		return &b
	},
}

func getBuf() *[]byte  { return bufPool.Get().(*[]byte) }
func putBuf(b *[]byte) { bufPool.Put(b) }

// ==================== Оптимизация ====================

func enableBBR() {
	log.Println("[SYS] Оптимизация TCP...")
	out, _ := runCmd("bash", "-c", "sysctl net.ipv4.tcp_congestion_control")
	if strings.Contains(out, "bbr") {
		log.Println("[SYS] BBR уже активен ✓")
		return
	}
	cmds := [][]string{
		{"sysctl", "-w", "net.core.default_qdisc=fq"},
		{"sysctl", "-w", "net.ipv4.tcp_congestion_control=bbr"},
		{"sysctl", "-w", "net.core.rmem_max=25165824"},
		{"sysctl", "-w", "net.core.wmem_max=25165824"},
		{"sysctl", "-w", "net.ipv4.tcp_rmem=4096 87380 25165824"},
		{"sysctl", "-w", "net.ipv4.tcp_wmem=4096 65536 25165824"},
	}
	for _, cmd := range cmds {
		runCmd(cmd[0], cmd[1:]...)
	}
	log.Println("[SYS] BBR включен ✓")
}

// ==================== Статистика ====================

var (
	totalBytesFromClient int64
	totalBytesToClient   int64
	activeConns          int32 // DTLS-сессии (воркеры)
	activeUsers          int32 // уникальные устройства онлайн
	totalConns           int64
	natType              string = "Инициализация..."
	serverStartTime      time.Time
)

type wgPeerCounters struct {
	rx int64
	tx int64
}

// wgPeerTrafficLast — последние rx/tx с wg show dump (учёт по пирам, не по DTLS-сессии).
var wgPeerTrafficLast sync.Map

// wgActivitySample — активность WG-пира для онлайн-статуса панели.
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
	if password != "" {
		touchUserLastSeen(password)
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
// Надёжнее per-session flush: один peer может иметь несколько DTLS-прокси без GETCONF.
func syncTrafficFromWGPeers() {
	peers, err := collectWGPeerStats()
	if err != nil {
		wgTrafficErrOnce.Do(func() {
			log.Printf("[WG] учёт трафика: %v", err)
		})
		return
	}
	dbMutex.Lock()
	defer dbMutex.Unlock()
	for _, peer := range peers {
		if peer.pubKeyB64 == "" {
			continue
		}
		var deviceID string
		for id, dev := range db.Devices {
			if dev != nil && dev.PubKey == peer.pubKeyB64 {
				deviceID = id
				break
			}
		}
		if deviceID == "" {
			continue
		}
		pass := passwordForDeviceLocked(deviceID)
		if pass == "" {
			pass = bindOrphanDeviceToMainLocked(deviceID)
		}
		if pass == "" {
			continue
		}
		cur := wgPeerCounters{rx: peer.rx, tx: peer.tx}
		prevVal, hadPrev := wgPeerTrafficLast.Load(deviceID)
		wgPeerTrafficLast.Store(deviceID, cur)
		if !hadPrev {
			continue
		}
		prev := prevVal.(wgPeerCounters)
		if peer.rx < prev.rx || peer.tx < prev.tx {
			continue
		}
		dRx := peer.rx - prev.rx
		dTx := peer.tx - prev.tx
		if dRx > 0 {
			addTrafficLocked(pass, dRx, false)
		}
		if dTx > 0 {
			addTrafficLocked(pass, dTx, true)
		}
	}
}

type wgPeerInfo struct {
	pubKeyB64     string
	allowedIP     string
	endpoint      string
	lastHandshake int64
	rx, tx        int64
}

func deviceForWGPeerLocked(peer wgPeerInfo) (deviceID, userIP, password string, isMain bool) {
	ip := strings.TrimSuffix(peer.allowedIP, "/32")
	for id, dev := range db.Devices {
		if dev == nil {
			continue
		}
		if peer.pubKeyB64 != "" && dev.PubKey == peer.pubKeyB64 {
			pass := passwordForDeviceLocked(id)
			if pass == "" {
				continue
			}
			if dev.IP != "" {
				ip = dev.IP
			}
			return id, ip, pass, pass == db.MainPassword
		}
	}
	if ip != "" {
		id, pass, main := deviceForWGIPLocked(ip)
		return id, ip, pass, main
	}
	return "", "", "", false
}

func collectWGPeerInfos() ([]wgPeerInfo, error) {
	serverWGDevMu.RLock()
	dev := serverWGDev
	serverWGDevMu.RUnlock()
	if dev != nil {
		text, err := dev.IpcGet()
		if err == nil {
			if peers := parseWGPeerInfosIpc(text); len(peers) > 0 {
				return peers, nil
			}
		}
	}
	out, err := exec.Command("wg", "show", wgIfaceName, "dump").Output()
	if err != nil {
		return nil, fmt.Errorf("IpcGet/wg show dump: %w", err)
	}
	return parseWGPeerInfosDump(string(out)), nil
}

func parseWGPeerInfosIpc(text string) []wgPeerInfo {
	var peers []wgPeerInfo
	var cur *wgPeerInfo
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "public_key":
			b64, err := hexKeyToB64(v)
			if err != nil {
				continue
			}
			peers = append(peers, wgPeerInfo{pubKeyB64: b64})
			cur = &peers[len(peers)-1]
		case "endpoint":
			if cur != nil {
				cur.endpoint = strings.TrimSpace(v)
			}
		case "last_handshake_time_sec":
			if cur != nil {
				cur.lastHandshake, _ = strconv.ParseInt(v, 10, 64)
			}
		case "allowed_ip":
			if cur != nil && cur.allowedIP == "" {
				cur.allowedIP = strings.Split(strings.TrimSpace(v), "/")[0]
			}
		case "rx_bytes":
			if cur != nil {
				cur.rx, _ = strconv.ParseInt(v, 10, 64)
			}
		case "tx_bytes":
			if cur != nil {
				cur.tx, _ = strconv.ParseInt(v, 10, 64)
			}
		}
	}
	return peers
}

func parseWGPeerInfosDump(text string) []wgPeerInfo {
	var peers []wgPeerInfo
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		allowed := strings.Split(fields[3], ",")[0]
		if allowed == "off" {
			continue
		}
		handshake, _ := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		rx, errRx := strconv.ParseInt(strings.TrimSpace(fields[5]), 10, 64)
		tx, errTx := strconv.ParseInt(strings.TrimSpace(fields[6]), 10, 64)
		if errRx != nil || errTx != nil {
			continue
		}
		peers = append(peers, wgPeerInfo{
			pubKeyB64:     fields[0],
			endpoint:      fields[2],
			allowedIP:     strings.TrimSuffix(allowed, "/32"),
			lastHandshake: handshake,
			rx:            rx,
			tx:            tx,
		})
	}
	return peers
}

// syncOnlineFromWGPeers восстанавливает presence по активным WG-пирам (multi-worker relay без GETCONF).
func syncOnlineFromWGPeers() {
	peers, err := collectWGPeerInfos()
	if err != nil {
		return
	}
	now := time.Now()
	nowUnix := now.Unix()
	dbMutex.Lock()
	defer dbMutex.Unlock()
	onlineUsersMutex.Lock()
	defer onlineUsersMutex.Unlock()
	for _, peer := range peers {
		if peer.lastHandshake <= 0 || nowUnix-peer.lastHandshake > wgPeerOnlineMaxAgeSec() {
			continue
		}
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
		label := userLabel(pass, isMain)
		if info, ok := onlineUsers[id]; ok && info != nil {
			info.LastActivity = now
			if info.Sessions <= 0 {
				info.Sessions = 1
			}
			if ip != "" {
				info.IP = ip
			}
			if pass != "" {
				info.Password = pass
			}
			if label != "" {
				info.Label = label
			}
			continue
		}
		onlineUsers[id] = &onlineUserInfo{
			Label:        label,
			Password:     pass,
			IP:           ip,
			Sessions:     1,
			LastActivity: now,
		}
	}
}

func collectWGPeerStats() ([]wgPeerStat, error) {
	peers, err := collectWGPeerInfos()
	if err != nil {
		return nil, err
	}
	stats := make([]wgPeerStat, 0, len(peers))
	for _, p := range peers {
		if p.pubKeyB64 == "" {
			continue
		}
		stats = append(stats, wgPeerStat{pubKeyB64: p.pubKeyB64, rx: p.rx, tx: p.tx})
	}
	return stats, nil
}

type wgPeerStat struct {
	pubKeyB64 string
	rx, tx    int64
}

func hexKeyToB64(hexKey string) (string, error) {
	b, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil || len(b) != 32 {
		return "", fmt.Errorf("bad wg public_key hex")
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func statsLoop(ctx context.Context, configDir string) {
	serverStartTime = time.Now()
	statsFile := filepath.Join(configDir, "server.log")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fromC := atomic.LoadInt64(&totalBytesFromClient)
			toC := atomic.LoadInt64(&totalBytesToClient)
			sessions := atomic.LoadInt32(&activeConns)
			refreshWGActivity()
			online := snapshotOnlineUsers()
			users := int32(len(online))
			atomic.StoreInt32(&activeUsers, users)
			for _, o := range online {
				if pass := strings.TrimSpace(o["password"]); pass != "" {
					touchUserLastSeen(pass)
				}
			}
			total := atomic.LoadInt64(&totalConns)
			uptime := time.Since(serverStartTime)

			log.Printf("[СТАТ] Пользователей: %d | Сессий: %d | Всего: %d | NAT: %s | ↑%.2f МБ | ↓%.2f МБ",
				users, sessions, total, natType,
				float64(fromC)/1024/1024,
				float64(toC)/1024/1024,
			)

			// Пишем server.log
			dbMutex.Lock()
			numPasswords := len(db.Passwords)
			numDevices := len(db.Devices)
			dbMutex.Unlock()

			uptimeStr := formatUptime(uptime)
			downGB := float64(toC) / (1024 * 1024 * 1024)
			upGB := float64(fromC) / (1024 * 1024 * 1024)

			statsJSON, _ := json.Marshal(map[string]interface{}{
				"active_users": users,
				"active":       users,
				"sessions":     sessions,
				"total":        total,
				"nat":          natType,
				"uptime":       uptimeStr,
				"down_gb":      fmt.Sprintf("%.2f", downGB),
				"up_gb":        fmt.Sprintf("%.2f", upGB),
				"passwords":    numPasswords,
				"devices":      numDevices,
				"online":       online,
				"timestamp":    time.Now().Unix(),
			})
			os.WriteFile(statsFile, statsJSON, 0644)

			syncTrafficFromWGPeers()
			syncVPNLocalServices(wgIfaceName)
			relayEvictAllIdle(relayStaleEvictIdle)

			if trafficDirty.Load() {
				dbMutex.Lock()
				if err := saveDB(); err != nil {
					log.Printf("[DB] save traffic: %v", err)
				} else {
					trafficDirty.Store(false)
				}
				dbMutex.Unlock()
			}
		}
	}
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dд %dч %dм", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dч %dм", hours, mins)
	}
	return fmt.Sprintf("%dм", mins)
}

// ==================== Утилиты ====================

func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runCmdSilent(name string, args ...string) string {
	out, _ := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out))
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func isNetTimeout(err error) bool {
	ne, ok := err.(net.Error)
	return ok && ne.Timeout()
}

func getDefaultInterface() string {
	out := runCmdSilent("bash", "-c", "ip route show default | awk '/default/ {print $5}' | head -1")
	if out != "" {
		return strings.TrimSpace(out)
	}
	out = runCmdSilent("bash", "-c", "ip -o link show | awk -F': ' '{print $2}' | grep -v -E 'lo|wg|tun|wdtt' | head -1")
	if out != "" {
		return strings.TrimSpace(out)
	}
	return "eth0"
}

// ==================== Ключи ====================

type wgKeys struct {
	serverPrivate, serverPublic, clientPrivate, clientPublic string
}

func b64ToHex(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	if len(b) != 32 {
		return "", fmt.Errorf("key length %d != 32", len(b))
	}
	return hex.EncodeToString(b), nil
}

func generateKeyPair() (privB64, pubB64 string, err error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return "", "", err
	}
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64
	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(priv[:]),
		base64.StdEncoding.EncodeToString(pub), nil
}

func loadOrGenerateKeys(dir string) (*wgKeys, error) {
	f := filepath.Join(dir, "wg-keys.dat")
	if data, err := os.ReadFile(f); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) >= 4 {
			keys := &wgKeys{
				serverPrivate: strings.TrimSpace(lines[0]),
				serverPublic:  strings.TrimSpace(lines[1]),
				clientPrivate: strings.TrimSpace(lines[2]),
				clientPublic:  strings.TrimSpace(lines[3]),
			}
			for _, k := range []string{keys.serverPrivate, keys.serverPublic,
				keys.clientPrivate, keys.clientPublic} {
				if _, err := b64ToHex(k); err != nil {
					goto generate
				}
			}
			log.Printf("[WG] Ключи загружены из %s", f)
			return keys, nil
		}
	}
generate:
	log.Println("[WG] Генерирую новые ключи...")
	sPriv, sPub, err := generateKeyPair()
	if err != nil {
		return nil, err
	}
	cPriv, cPub, err := generateKeyPair()
	if err != nil {
		return nil, err
	}
	keys := &wgKeys{sPriv, sPub, cPriv, cPub}
	os.MkdirAll(dir, 0700)
	os.WriteFile(f, []byte(fmt.Sprintf("%s\n%s\n%s\n%s\n",
		keys.serverPrivate, keys.serverPublic,
		keys.clientPrivate, keys.clientPublic)), 0600)
	log.Printf("[WG] Ключи сохранены в %s", f)
	return keys, nil
}

// ==================== NAT ====================

func setupFullConeNAT(wgIface string) error {
	log.Println("[NAT] ══════════════════════════════════════")

	os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644)

	setupForwardRules(wgIface)
	syncVPNLocalServices(wgIface)
	extIface := getDefaultInterface()
	log.Printf("[NAT] Внешний: %s", extIface)

	switch {
	case commandExists("iptables"):
		for i := 0; i < 5; i++ {
			exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", wgServerCIDR, "-o", extIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "MASQUERADE").Run()
		}
		exec.Command("iptables", "-t", "nat", "-I", "POSTROUTING", "1", "-s", wgServerCIDR, "-o", extIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "MASQUERADE").Run()
		natType = "MASQUERADE iptables ✅"
	case commandExists("nft"):
		setupNftNAT(extIface)
		natType = "MASQUERADE nft ✅"
	default:
		natType = "NAT не настроен: нет iptables/nft"
		log.Printf("[NAT] WARNING: %s", natType)
	}

	log.Printf("[NAT] Режим: %s", natType)
	log.Println("[NAT] ══════════════════════════════════════")
	return nil
}

var (
	vpnLocalPortsMu      sync.Mutex
	vpnLocalPortsApplied []int
)

func panelServicePorts() []int {
	panelPort, subPort := defaultPanelTCPPort, defaultSubTCPPort
	if p, s, ok, _ := loadPanelServicePortsFromSQLite(); ok {
		panelPort, subPort = p, s
	}
	ports := []int{panelPort}
	if subPort != panelPort {
		ports = append(ports, subPort)
	}
	return ports
}

func syncVPNLocalServices(wgIface string) {
	ports := panelServicePorts()
	vpnLocalPortsMu.Lock()
	defer vpnLocalPortsMu.Unlock()
	if portsEqual(vpnLocalPortsApplied, ports) && len(vpnLocalPortsApplied) > 0 {
		return
	}
	if !commandExists("iptables") {
		return
	}
	for _, port := range vpnLocalPortsApplied {
		removeVPNLocalPortRule(wgIface, port)
	}
	vpnLocalPortsApplied = append([]int(nil), ports...)
	for _, port := range ports {
		addVPNLocalPortRule(wgIface, port)
	}
	log.Printf("[NAT] Панель/подписка через VPN: tcp %v с %s", ports, wgIface)
}

func portsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func addVPNLocalPortRule(wgIface string, port int) {
	args := []string{"-i", wgIface, "-p", "tcp", "--dport", strconv.Itoa(port),
		"-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT"}
	exec.Command("iptables", append([]string{"-I", "INPUT", "1"}, args...)...).Run()
}

func removeVPNLocalPortRule(wgIface string, port int) {
	args := []string{"-i", wgIface, "-p", "tcp", "--dport", strconv.Itoa(port),
		"-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT"}
	for i := 0; i < 5; i++ {
		if exec.Command("iptables", append([]string{"-D", "INPUT"}, args...)...).Run() != nil {
			break
		}
	}
}

func setupNftNAT(extIface string) {
	exec.Command("nft", "add", "table", "ip", "wdtt").Run()
	exec.Command("nft", "add", "chain", "ip", "wdtt", "postrouting", "{ type nat hook postrouting priority 100; }").Run()
	exec.Command("nft", "add", "rule", "ip", "wdtt", "postrouting", "ip", "saddr", wgServerCIDR, "oifname", extIface, "masquerade").Run()
}

func setupForwardRules(wgIface string) {
	if commandExists("iptables") {
		for i := 0; i < 5; i++ {
			exec.Command("iptables", "-D", "FORWARD", "-i", wgIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT").Run()
			exec.Command("iptables", "-D", "FORWARD", "-o", wgIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT").Run()
		}
		// -I 1: до UFW, иначе FORWARD policy DROP режет UDP/ответы
		exec.Command("iptables", "-I", "FORWARD", "1", "-i", wgIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT").Run()
		exec.Command("iptables", "-I", "FORWARD", "1", "-o", wgIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT").Run()
		return
	}
	if commandExists("nft") {
		exec.Command("nft", "add", "table", "inet", "wdtt").Run()
		exec.Command("nft", "add", "chain", "inet", "wdtt", "forward", "{ type filter hook forward priority 0; policy accept; }").Run()
		exec.Command("nft", "add", "rule", "inet", "wdtt", "forward", "iifname", wgIface, "accept").Run()
		exec.Command("nft", "add", "rule", "inet", "wdtt", "forward", "oifname", wgIface, "accept").Run()
	}
}

// ==================== WireGuard ====================

func startUserspaceWG(keys *wgKeys, wgPort int) (*device.Device, error) {
	runCmdSilent("ip", "link", "del", wgIfaceName)
	time.Sleep(100 * time.Millisecond)

	tunDev, err := tun.CreateTUN(wgIfaceName, wgMTU)
	if err != nil {
		return nil, fmt.Errorf("CreateTUN: %w", err)
	}

	ifaceName, err := tunDev.Name()
	if err != nil {
		tunDev.Close()
		return nil, fmt.Errorf("TUN name: %w", err)
	}

	logger := device.NewLogger(device.LogLevelError, "[WG] ")
	bind := conn.NewDefaultBind()
	dev := device.NewDevice(tunDev, bind, logger)

	serverPrivHex, _ := b64ToHex(keys.serverPrivate)

	if err := dev.IpcSet(fmt.Sprintf(
		"private_key=%s\nlisten_port=%d\n",
		serverPrivHex, wgPort,
	)); err != nil {
		dev.Close()
		return nil, fmt.Errorf("IpcSet: %w", err)
	}

	for _, d := range db.Devices {
		pubHex, _ := b64ToHex(d.PubKey)
		if pubHex != "" {
			dev.IpcSet(fmt.Sprintf("public_key=%s\nallowed_ip=%s/32\n", pubHex, d.IP))
		}
	}

	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("device.Up: %w", err)
	}

	if err := configureInterface(ifaceName); err != nil {
		dev.Close()
		return nil, err
	}

	if err := setupFullConeNAT(ifaceName); err != nil {
		dev.Close()
		return nil, err
	}

	go func() {
		uapiFile, err := ipc.UAPIOpen(ifaceName)
		if err != nil {
			return
		}
		uapi, err := ipc.UAPIListen(ifaceName, uapiFile)
		if err != nil {
			return
		}
		defer uapi.Close()
		for {
			c, err := uapi.Accept()
			if err != nil {
				return
			}
			go dev.IpcHandle(c)
		}
	}()

	log.Printf("[WG] Запущен на порту %d", wgPort)
	return dev, nil
}

func configureInterface(ifaceName string) error {
	for _, cmd := range [][]string{
		{"ip", "addr", "add", wgServerCIDR, "dev", ifaceName},
		{"ip", "link", "set", "mtu", fmt.Sprintf("%d", wgMTU), "dev", ifaceName},
		{"ip", "link", "set", ifaceName, "up"},
	} {
		out, err := runCmd(cmd[0], cmd[1:]...)
		if err != nil && !strings.Contains(out, "File exists") {
			return fmt.Errorf("%s: %s", strings.Join(cmd, " "), out)
		}
	}
	return nil
}

func buildClientConfig(serverPublic, clientPrivate, clientIP, clientPort string) string {
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s/32
DNS = %s
MTU = %d

[Peer]
PublicKey = %s
AllowedIPs = 0.0.0.0/0
Endpoint = 127.0.0.1:%s
PersistentKeepalive = %d`,
		clientPrivate, clientIP, clientDNS, wgMTU,
		serverPublic, clientPort, keepalive,
	)
}
