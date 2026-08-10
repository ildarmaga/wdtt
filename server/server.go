package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	"github.com/pion/dtls/v3"

	"golang.zx2c4.com/wireguard/device"
)

func rawDirectListenAddr(dtlsAddr string, rawPort int) (string, error) {
	host, portText, err := net.SplitHostPort(dtlsAddr)
	if err != nil {
		return "", err
	}
	dtlsPort, err := strconv.Atoi(portText)
	if err != nil || dtlsPort < 1 || dtlsPort > 65535 {
		return "", fmt.Errorf("invalid DTLS port %q", portText)
	}
	port := rawPort
	if port <= 0 {
		if dtlsPort > 65535-rawDirectPortOffset {
			return "", fmt.Errorf("invalid DTLS port %q for RAW offset", portText)
		}
		port = dtlsPort + rawDirectPortOffset
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid RAW port %d", port)
	}
	if port == dtlsPort {
		return "", fmt.Errorf("RAW port must differ from DTLS")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

const (
	wgIfaceName             = "wdtt0"
	wgServerAddr            = "10.66.66.1"
	wgServerCIDR            = wgServerAddr + "/24"
	defaultInternalWGPort   = 56001
	defaultClientDNS        = "1.1.1.1"
	defaultMaxUsers         = 10
	maxUsersSubnetLimit     = 249
	defaultWgMTU            = 1280
	defaultPanelTCPPort     = 2860 // fallback до panel.db
	defaultSubTCPPort       = 2096
	keepalive               = 25 // default; overridden from panel.db via wgKeepaliveSec
	defaultStatsIntervalSec = 2
)

var (
	wgKeepaliveSec   = keepalive
	statsIntervalSec = defaultStatsIntervalSec
)

var (
	clientDNS             = defaultClientDNS
	maxGeneratedPasswords = defaultMaxUsers
	dtlsHandshakeTimeout  = 30 * time.Second
	wgMTU                 = defaultWgMTU
)

// Ограничение параллельных DTLS handshake (burst от multi-worker клиентов)
var dtlsHandshakeSem = make(chan struct{}, 64)

// ==================== База данных и Бот ====================

type ClientDevice struct {
	DeviceID string `json:"device_id"`
	IP       string `json:"ip"`
	PrivKey  string `json:"priv_key"`
	PubKey   string `json:"pub_key"`
}

type PasswordEntry struct {
	DeviceID      string   `json:"device_id,omitempty"`     // legacy, синхронизируется с device_ids[0]
	DeviceIDs     []string `json:"device_ids,omitempty"`    // привязанные устройства
	MaxDevices    int      `json:"max_devices,omitempty"`   // 0 = 1, лимит слотов
	ExpiresAt     int64    `json:"expires_at"`              // unix timestamp
	DownBytes     int64    `json:"down_bytes"`              // скачано клиентом
	UpBytes       int64    `json:"up_bytes"`                // отдано клиентом
	TotalBytes    int64    `json:"total_bytes,omitempty"`   // 0 = без лимита
	MaxDownMBps   float64  `json:"max_down_mbps,omitempty"` // 0 = без лимита, MB/s
	MaxUpMBps     float64  `json:"max_up_mbps,omitempty"`   // 0 = без лимита, MB/s
	Comment       string   `json:"comment,omitempty"`
	VkHash        string   `json:"vk_hash,omitempty"`
	WbRoom        string   `json:"wb_room,omitempty"`
	Ports         string   `json:"ports,omitempty"` // "dtls,wg,tun"
	IsDeactivated bool     `json:"is_deactivated,omitempty"`
	SubID         string   `json:"sub_id,omitempty"`
	LastSeenAt    int64    `json:"last_seen_at,omitempty"`
}

type Database struct {
	MainPassword string                    `json:"main_password"`
	AdminID      string                    `json:"admin_id"`
	BotToken     string                    `json:"bot_token"`
	Passwords    map[string]*PasswordEntry `json:"passwords"`
	Devices      map[string]*ClientDevice  `json:"devices"`
}

var (
	db           *Database
	dbMutex      sync.Mutex
	trafficDirty atomic.Bool
)

var serverWrapKeys = newWrapKeyStore()

const (
	passChars            = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"
	generatedPasswordLen = 16
)

func generatePassword() string {
	b := make([]byte, generatedPasswordLen)
	randomBytes := make([]byte, len(b))
	if _, err := rand.Read(randomBytes); err != nil {
		now := time.Now().UnixNano()
		for i := range b {
			b[i] = passChars[int(now+int64(i))%len(passChars)]
		}
		return string(b)
	}
	for i, raw := range randomBytes {
		b[i] = passChars[int(raw)%len(passChars)]
	}
	return string(b)
}

var publicIP string = ""

func getPublicIP() string {
	if publicIP != "" {
		return publicIP
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return "YOUR_SERVER_IP"
	}
	defer resp.Body.Close()
	ipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "YOUR_SERVER_IP"
	}
	publicIP = string(bytes.TrimSpace(ipBytes))
	return publicIP
}

func initDB(mainPass, adminID, botToken string) {
	if incoming, err := loadDatabaseFromDiskSource(); err != nil {
		log.Printf("[DB] load: %v", err)
		db = &Database{
			Passwords: make(map[string]*PasswordEntry),
			Devices:   make(map[string]*ClientDevice),
		}
	} else {
		db = incoming
	}
	if db.Passwords == nil {
		db.Passwords = make(map[string]*PasswordEntry)
	}
	if db.Devices == nil {
		db.Devices = make(map[string]*ClientDevice)
	}
	migrateDatabaseDevices()
	applyDBGlobalFromCLI(mainPass, adminID, botToken)
	if db.MainPassword != "" {
		ensureMainPasswordEntryLocked()
	}
	saveDB()
	if err := refreshWrapKeysFromDBLocked(); err != nil {
		log.Fatalf("[WRAP] init keys: %v", err)
	}
	if serverPanelDBReady() {
		log.Printf("[DB] users loaded from %s (sqlite primary)", panelDBPath)
	}
}

func saveDB() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	return saveDBLocked()
}

// saveDBLocked пишет БД; вызывать только при уже удержанном dbMutex.
func saveDBLocked() error {
	if !serverPanelDBReady() {
		return fmt.Errorf("panel.db not available at %s", panelDBPath)
	}
	err := saveDatabaseToSQLite(db)
	if err == nil {
		rememberUsersRevFromDisk()
	}
	return err
}

func isPasswordExpired(entry *PasswordEntry) bool {
	if entry == nil {
		return true
	}
	if entry.ExpiresAt == 0 {
		return false // бессрочный
	}
	return time.Now().Unix() > entry.ExpiresAt
}

func isTrafficExceeded(entry *PasswordEntry) bool {
	if entry == nil || entry.TotalBytes <= 0 {
		return false
	}
	return entry.UpBytes+entry.DownBytes >= entry.TotalBytes
}

func passwordForDeviceLocked(deviceID string) string {
	if deviceID == "" {
		return ""
	}
	var fallback string
	for pass, entry := range db.Passwords {
		if entry == nil || !entryHasDevice(entry, deviceID) {
			continue
		}
		if pass == db.MainPassword {
			if fallback == "" {
				fallback = pass
			}
			continue
		}
		return pass
	}
	return fallback
}

// bindOrphanDeviceToMainLocked привязывает устройство из wdtt_devices без записи в wdtt_user_devices к главному паролю.
func bindOrphanDeviceToMainLocked(deviceID string) string {
	if deviceID == "" || db.MainPassword == "" {
		return ""
	}
	if _, ok := db.Devices[deviceID]; !ok {
		return ""
	}
	if pass := passwordForDeviceLocked(deviceID); pass != "" {
		return pass
	}
	ensureMainPasswordEntryLocked()
	if db.Passwords[db.MainPassword] == nil {
		return ""
	}
	// Главный пароль = безлимит устройств: трафик учитываем на него,
	// но НЕ пишем устройство в привязки (wdtt_user_devices) — иначе панель
	// копила бы устройства владельца. Резолв чисто в памяти.
	return db.MainPassword
}

func deviceForWGIPLocked(ip string) (deviceID, password string, isMain bool) {
	if ip == "" {
		return "", "", false
	}
	for id, dev := range db.Devices {
		if dev == nil || dev.IP != ip {
			continue
		}
		pass := passwordForDeviceLocked(id)
		return id, pass, pass != "" && pass == db.MainPassword
	}
	return "", "", false
}

// resolveConnByWGLocalPort определяет устройство, когда клиент шлёт WG-пакеты без GETCONF
// (повторное подключение с сохранённым конфигом). wg endpoint = 127.0.0.1:<localPort>.
func resolveConnByWGLocalPort(localPort int) (deviceID, userIP, password string, isMain bool) {
	if localPort <= 0 {
		return "", "", "", false
	}
	peers, err := collectWGPeerInfos()
	if err != nil {
		return "", "", "", false
	}
	needle := fmt.Sprintf(":%d", localPort)
	dbMutex.Lock()
	defer dbMutex.Unlock()
	for _, peer := range peers {
		if peer.endpoint == "" || !strings.Contains(peer.endpoint, needle) {
			continue
		}
		id, ip, pass, main := deviceForWGPeerLocked(peer)
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
			main = true
		}
		if ip == "" {
			ip = peer.allowedIP
		}
		return id, ip, pass, main
	}
	return "", "", "", false
}

func tryResolveConnFromWG(wgConn net.Conn) (deviceID, userIP, password string, isMain bool) {
	localPort := 0
	if ua, ok := wgConn.LocalAddr().(*net.UDPAddr); ok && ua != nil {
		localPort = ua.Port
	}
	for i := 0; i < 20; i++ {
		if i > 0 {
			time.Sleep(25 * time.Millisecond)
		}
		if id, ip, pass, main := resolveConnByWGLocalPort(localPort); id != "" {
			log.Printf("[WG] Без GETCONF: device=%s ip=%s pass=%s", id, ip, maskPassword(pass))
			return id, ip, pass, main
		}
	}
	return "", "", "", false
}

func removeWGDeviceLocked(wgDev *device.Device, deviceID string) {
	if deviceID == "" {
		return
	}
	dev, ok := db.Devices[deviceID]
	if !ok {
		return
	}
	removePeerFromWG(wgDev, dev)
	delete(db.Devices, deviceID)
}

func removeAllEntryDevicesLocked(wgDev *device.Device, entry *PasswordEntry) {
	for _, devID := range allEntryDeviceIDs(entry) {
		removeWGDeviceLocked(wgDev, devID)
	}
	clearEntryDevices(entry)
}

func resolveTrafficPassword(connPassword string, connIsMainPass bool, connDeviceID string) string {
	if !connIsMainPass && connPassword != "" {
		return connPassword
	}
	if pass := passwordForDeviceLocked(connDeviceID); pass != "" {
		return pass
	}
	if connIsMainPass && db.MainPassword != "" {
		return db.MainPassword
	}
	return ""
}

func ensureMainPasswordEntryLocked() {
	if db.MainPassword == "" {
		return
	}
	entry, ok := db.Passwords[db.MainPassword]
	if !ok || entry == nil {
		db.Passwords[db.MainPassword] = &PasswordEntry{Comment: paneldb.MainUserComment}
		return
	}
	if entry.Comment == "" || entry.Comment == "Владелец" {
		entry.Comment = paneldb.MainUserComment
	}
	// Главный пароль — без лимита устройств (MaxDevices не используется).
	entry.MaxDevices = 0
}

func touchUserLastSeen(password string) {
	password = strings.TrimSpace(password)
	if password == "" {
		return
	}
	now := time.Now().Unix()
	dbMutex.Lock()
	if password == db.MainPassword {
		ensureMainPasswordEntryLocked()
	}
	e, ok := db.Passwords[password]
	if !ok || e == nil {
		dbMutex.Unlock()
		return
	}
	if now <= e.LastSeenAt {
		dbMutex.Unlock()
		return
	}
	e.LastSeenAt = now
	dbMutex.Unlock()
	_ = updateLastSeenInSQLite(password, now)
}

func touchOnlineUsersLastSeenBatch() {
	now := time.Now().Unix()
	var passwords []string
	forEachOnlinePassword(func(pass string) {
		passwords = append(passwords, pass)
	})
	if len(passwords) == 0 {
		return
	}
	updates := make(map[string]int64, len(passwords))
	dbMutex.Lock()
	for _, password := range passwords {
		if password == db.MainPassword {
			ensureMainPasswordEntryLocked()
		}
		e, ok := db.Passwords[password]
		if !ok || e == nil || now <= e.LastSeenAt {
			continue
		}
		e.LastSeenAt = now
		updates[password] = now
	}
	dbMutex.Unlock()
	if len(updates) > 0 {
		_ = updateLastSeenBatchInSQLite(updates)
	}
}

func addTrafficLocked(password string, bytes int64, isDownload bool) bool {
	if password == "" {
		return true
	}
	if password == db.MainPassword {
		ensureMainPasswordEntryLocked()
	}
	e, ok := db.Passwords[password]
	if !ok || e == nil || isPasswordExpired(e) || e.IsDeactivated || isTrafficExceeded(e) {
		return false
	}
	if isDownload {
		e.DownBytes += bytes
	} else {
		e.UpBytes += bytes
	}
	trafficDirty.Store(true)
	if isTrafficExceeded(e) {
		e.IsDeactivated = true
		trafficDirty.Store(true)
		if err := saveTrafficToSQLiteLocked(); err != nil {
			if !errors.Is(err, errTrafficFlushFenced) {
				log.Printf("[DB] save traffic deactivate: %v", err)
			}
		} else {
			trafficDirty.Store(false)
		}
	}
	return true
}

func buildPublicWdttLink(srvIP, ports, password, remark, vkHash string) string {
	link, err := buildWdttShareLinkFromPorts(srvIP, ports, password, remark, vkHash)
	if err != nil {
		return ""
	}
	return link
}

func getNextIP() string {
	used := make(map[string]bool)
	for _, dev := range db.Devices {
		used[dev.IP] = true
	}
	for i := 2; i <= 250; i++ {
		ip := fmt.Sprintf("10.66.66.%d", i)
		if !used[ip] {
			return ip
		}
	}
	return ""
}

// ==================== Main ====================

func Run() {
	RunManaged()
}

func RunManaged() {
	initServerFlags()
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("══════════════════════════════════════════")
	log.Println("   WDTT Server v2 (Multi-User)")
	log.Println("══════════════════════════════════════════")

	cfg := baseServerConfigFromFlags()
	cfg.mergeFromPanelDB()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

	for {
		ctx, cancel := context.WithCancel(context.Background())
		restartCh := make(chan struct{}, 1)
		setRestartNotify(restartCh)

		done := make(chan struct{})
		go func() {
			defer close(done)
			runServerOnce(ctx, cfg)
		}()

		select {
		case <-sig:
			log.Println("[SERVER] Shutdown signal received, draining connections...")
			cancel()
			<-done
			return
		case <-restartCh:
			log.Println("[SERVER] In-process restart requested (panel stays up)")
			cancel()
			<-done
			cfg = baseServerConfigFromFlags()
			cfg.mergeFromPanelDB()
		}
	}
}

func runServerOnce(ctx context.Context, cfg ServerConfig) {
	serverStartTime = time.Now()
	markServerVPNActive(false)

	disabled := false
	if startup, ok, err := loadStartupFromSQLite(); err == nil && ok && !startup.Enable {
		disabled = true
	}
	cfg.applyGlobals()

	initDB(cfg.MainPass, cfg.AdminID, cfg.BotToken)
	log.Printf("[CFG] Admin HTTP: %s (hot-reload /health, restart /admin/restart)", adminListenAddr)

	if disabled {
		log.Println("[SERVER] WDTT отключён в panel.db — VPN не слушает (admin HTTP активен)")
		startAdminServer(ctx, nil)
		<-ctx.Done()
		log.Println("[SERVER] Idle mode stopped")
		return
	}

	keys, err := loadOrGenerateKeys(cfg.ConfigDir)
	if err != nil {
		log.Fatalf("[WG] Ключи: %v", err)
	}

	enableBBR()

	wgDev, err := startUserspaceWG(keys, cfg.WgPort)
	if err != nil {
		log.Fatalf("[WG] Запуск: %v", err)
	}
	serverWGDevMu.Lock()
	serverWGDev = wgDev
	serverWGDevMu.Unlock()
	if n := suspendExpiredPasswords(wgDev); n > 0 {
		log.Printf("[DB] Отключено истёкших паролей при старте (остались в базе): %d", n)
	}
	syncPersistedPeersToWG(wgDev)
	syncAllSpeedLimits()
	defer func() {
		resetTcOnIface(wgIfaceName)
		wgDev.Close()
		runCmdSilent("ip", "link", "del", wgIfaceName)
	}()

	clearWGActivity()
	clearOnlineUsers()
	atomic.StoreInt32(&activeConns, 0)
	resetServerStatsCache(cfg.ConfigDir)
	go statsLoop(ctx, cfg.ConfigDir)
	go userPresenceLoop(ctx)
	go expiredPasswordJanitor(ctx, wgDev)
	go getconfFailJanitor(ctx)
	go relayFailJanitor(ctx)
	go botLoop(cfg.BotToken, cfg.AdminID, wgDev)
	startAdminServer(ctx, wgDev)

	addr, err := net.ResolveUDPAddr("udp", cfg.Listen)
	if err != nil {
		log.Fatalf("[DTLS] адрес %s: %v", cfg.Listen, err)
	}
	cert, err := loadOrGenerateDTLSCert(cfg.ConfigDir)
	if err != nil {
		log.Fatalf("[DTLS] Сертификат: %v", err)
	}
	if serverWrapKeys.Count() == 0 {
		log.Fatalf("[WRAP] нет активных паролей для WRAP")
	}

	wrapListener, err := listenWrapped(addr, serverWrapKeys)
	if err != nil {
		log.Fatalf("[WRAP] %v", err)
	}

	listener, err := dtls.NewListenerWithOptions(wrapListener,
		dtls.WithCertificates(cert),
		dtls.WithExtendedMasterSecret(dtls.RequireExtendedMasterSecret),
		dtls.WithCipherSuites(dtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256),
		dtls.WithConnectionIDGenerator(dtls.RandomCIDGenerator(8)),
		dtls.WithFlightInterval(100*time.Millisecond),
	)
	if err != nil {
		log.Fatalf("[DTLS] %v", err)
	}
	context.AfterFunc(ctx, func() { listener.Close() })

	wgEndpoint := fmt.Sprintf("127.0.0.1:%d", cfg.WgPort)
	var wg sync.WaitGroup

	rawAcceptDone := make(chan struct{})
	close(rawAcceptDone) // default: no direct listener
	rawListen, err := rawDirectListenAddr(cfg.Listen, cfg.RawDirectPort)
	if err != nil {
		log.Printf("[RAW-DIRECT] address: %v — WG/DTLS продолжают работать", err)
	} else {
		rawAddr, resolveErr := net.ResolveUDPAddr("udp", rawListen)
		if resolveErr != nil {
			log.Printf("[RAW-DIRECT] resolve %s: %v — WG/DTLS продолжают работать", rawListen, resolveErr)
		} else {
			rawWrapListener, listenErr := listenWrapped(rawAddr, serverWrapKeys)
			if listenErr != nil {
				log.Printf("[RAW-DIRECT] listen %s: %v — WG/DTLS продолжают работать", rawListen, listenErr)
			} else {
				ensureRawDirectFirewall(rawListen)
				rawAcceptDone = make(chan struct{})
				context.AfterFunc(ctx, func() { _ = rawWrapListener.Close() })
				go func() {
					defer close(rawAcceptDone)
					for {
						pc, remoteAddr, acceptErr := rawWrapListener.Accept()
						if acceptErr != nil {
							if ctx.Err() != nil {
								return
							}
							continue
						}
						wg.Add(1)
						go func(packetConn net.PacketConn, addr net.Addr) {
							defer wg.Done()
							conn := &rawDirectConn{pc: packetConn, addr: addr}
							defer conn.Close()
							handleDirectRawConn(ctx, conn)
						}(pc, remoteAddr)
					}
				}()
				log.Printf("   RAW direct (no DTLS): %s", rawListen)
			}
		}
	}

	log.Printf("   DTLS: %s | WG: %s | NAT: %s", cfg.Listen, wgEndpoint, natType)
	log.Printf("   WRAP: password HKDF + RTP AEAD | keys: %d", serverWrapKeys.Count())
	log.Println("[SERVER] Готов")
	markServerVPNActive(true)

	for {
		dtlsConn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				<-rawAcceptDone
				drain := make(chan struct{})
				go func() {
					wg.Wait()
					close(drain)
				}()
				select {
				case <-drain:
				case <-time.After(30 * time.Second):
					log.Println("[SERVER] Shutdown timeout (30s), active connections abandoned")
				}
				if trafficDirty.Load() {
					dbMutex.Lock()
					_ = saveTrafficToSQLiteLocked()
					trafficDirty.Store(false)
					dbMutex.Unlock()
				}
				log.Println("[SERVER] Stopped")
				return
			default:
			}
			continue
		}
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			defer c.Close()
			handleConn(ctx, c, wgEndpoint, wgDev, keys)
		}(dtlsConn)
	}
}

// ==================== Обработка соединений ====================
