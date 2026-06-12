package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"crypto/cipher"

	"github.com/pion/dtls/v3"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/ipc"
	"golang.zx2c4.com/wireguard/tun"

	dtlsnet "github.com/pion/dtls/v3/pkg/net"
	pionudp "github.com/pion/transport/v4/udp"
)

const (
	wgIfaceName              = "wdtt0"
	wgServerAddr             = "10.66.66.1"
	wgServerCIDR             = wgServerAddr + "/24"
	defaultInternalWGPort    = 56001
	defaultClientDNS         = "1.1.1.1"
	defaultMaxUsers          = 10
	maxUsersSubnetLimit      = 249
	defaultWgMTU             = 1280
	keepalive                = 25
)

var (
	clientDNS             = defaultClientDNS
	maxGeneratedPasswords = defaultMaxUsers
	dtlsHandshakeTimeout  = 30 * time.Second
	wgMTU                 = defaultWgMTU
)

// ==================== База данных и Бот ====================

type ClientDevice struct {
	DeviceID string `json:"device_id"`
	IP       string `json:"ip"`
	PrivKey  string `json:"priv_key"`
	PubKey   string `json:"pub_key"`
}

type PasswordEntry struct {
	DeviceID      string   `json:"device_id,omitempty"`  // legacy, синхронизируется с device_ids[0]
	DeviceIDs     []string `json:"device_ids,omitempty"` // привязанные устройства
	MaxDevices    int      `json:"max_devices,omitempty"` // 0 = 1, лимит слотов
	ExpiresAt     int64   `json:"expires_at"` // unix timestamp
	DownBytes     int64   `json:"down_bytes"` // скачано клиентом
	UpBytes       int64   `json:"up_bytes"`   // отдано клиентом
	TotalBytes    int64   `json:"total_bytes,omitempty"` // 0 = без лимита
	MaxDownMBps   float64 `json:"max_down_mbps,omitempty"` // 0 = без лимита, MB/s
	MaxUpMBps     float64 `json:"max_up_mbps,omitempty"`   // 0 = без лимита, MB/s
	Comment       string  `json:"comment,omitempty"`
	VkHash        string  `json:"vk_hash,omitempty"`
	Ports         string  `json:"ports,omitempty"` // "dtls,wg,tun"
	IsDeactivated bool    `json:"is_deactivated,omitempty"`
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
	dbFile       string
	trafficDirty atomic.Bool
)

var serverWrapKeys = newWrapKeyStore()

const (
	passChars            = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"
	generatedPasswordLen = 16
)

func loadInboundSettings(configDir string) {
	clientDNS = defaultClientDNS
	maxGeneratedPasswords = defaultMaxUsers
	dtlsHandshakeTimeout = 30 * time.Second
	maxDTLSPerDevice = 0
	wgMTU = defaultWgMTU
	data, err := os.ReadFile(filepath.Join(configDir, "inbound.json"))
	if err != nil {
		return
	}
	var raw struct {
		DNS                 string `json:"dns"`
		MaxUsers            int    `json:"max_users"`
		HandshakeTimeoutSec int    `json:"handshake_timeout_sec"`
		MaxDtlsPerDevice    int    `json:"max_dtls_per_device"`
		MTU                 int    `json:"mtu"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return
	}
	if dns := strings.TrimSpace(raw.DNS); dns != "" {
		clientDNS = dns
	}
	if raw.MaxUsers >= 1 && raw.MaxUsers <= maxUsersSubnetLimit {
		maxGeneratedPasswords = raw.MaxUsers
	}
	if raw.HandshakeTimeoutSec >= 5 && raw.HandshakeTimeoutSec <= 600 {
		dtlsHandshakeTimeout = time.Duration(raw.HandshakeTimeoutSec) * time.Second
	}
	if raw.MaxDtlsPerDevice >= 0 && raw.MaxDtlsPerDevice <= 50 {
		maxDTLSPerDevice = int32(raw.MaxDtlsPerDevice)
	}
	log.Printf("[CFG] inbound: DTLS timeout=%s, max сессий/device=%d (0=без лимита)", dtlsHandshakeTimeout, maxDTLSPerDevice)
	if raw.MTU >= 576 && raw.MTU <= 1500 {
		wgMTU = raw.MTU
	}
}

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

func stripVkUrl(url string) string {
	url = strings.TrimSpace(url)
	if idx := strings.LastIndex(url, "/"); idx != -1 {
		url = url[idx+1:]
	}
	if idx := strings.Index(url, "?"); idx != -1 {
		url = url[:idx]
	}
	return strings.TrimSpace(url)
}

type wrapKeyEntry struct {
	id  string
	key []byte
}

type wrapKeyStore struct {
	mu      sync.RWMutex
	entries []wrapKeyEntry
}

func newWrapKeyStore() *wrapKeyStore {
	return &wrapKeyStore{}
}

func deriveWrapKey(password string) ([]byte, error) {
	if password == "" {
		return nil, errors.New("empty password")
	}
	key := make([]byte, wrapKeyLen)
	reader := hkdf.New(
		sha256.New,
		[]byte(password),
		[]byte("WDTT-WRAP-v1"),
		[]byte("rtp-obfs/chacha20poly1305"),
	)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive wrap key: %w", err)
	}
	return key, nil
}

func wrapKeyID(password string) string {
	sum := sha256.Sum256([]byte("WDTT-WRAP-ID-v1\x00" + password))
	return hex.EncodeToString(sum[:8])
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func (s *wrapKeyStore) SetPasswords(mainPassword string, generated []string) error {
	next := make([]wrapKeyEntry, 0, len(generated)+1)
	seen := make(map[string]struct{}, len(generated)+1)

	if mainPassword != "" {
		key, err := deriveWrapKey(mainPassword)
		if err != nil {
			return err
		}
		next = append(next, wrapKeyEntry{id: "main", key: key})
		seen["main"] = struct{}{}
	}

	for _, password := range generated {
		if password == "" {
			continue
		}
		id := "pass:" + wrapKeyID(password)
		if _, exists := seen[id]; exists {
			continue
		}
		key, err := deriveWrapKey(password)
		if err != nil {
			for _, entry := range next {
				zeroBytes(entry.key)
			}
			return err
		}
		next = append(next, wrapKeyEntry{id: id, key: key})
		seen[id] = struct{}{}
	}

	s.mu.Lock()
	old := s.entries
	s.entries = next
	s.mu.Unlock()
	for _, entry := range old {
		aeadCache.Delete(string(entry.key))
		zeroBytes(entry.key)
	}
	return nil
}

func (s *wrapKeyStore) AddPassword(password string) error {
	key, err := deriveWrapKey(password)
	if err != nil {
		return err
	}
	id := "pass:" + wrapKeyID(password)

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range s.entries {
		if entry.id == id {
			zeroBytes(key)
			return nil
		}
	}
	s.entries = append(s.entries, wrapKeyEntry{id: id, key: key})
	return nil
}

func (s *wrapKeyStore) RemovePassword(password string) {
	id := "pass:" + wrapKeyID(password)

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, entry := range s.entries {
		if entry.id != id {
			continue
		}
		aeadCache.Delete(string(entry.key))
		zeroBytes(entry.key)
		copy(s.entries[i:], s.entries[i+1:])
		s.entries[len(s.entries)-1] = wrapKeyEntry{}
		s.entries = s.entries[:len(s.entries)-1]
		return
	}
}

func (s *wrapKeyStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *wrapKeyStore) Unwrap(raw, dst []byte) ([]byte, int, error) {
	if !obfsIsRTPPacket(raw) {
		return nil, 0, errors.New("wrap: non-obfs packet")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.entries) == 0 {
		return nil, 0, errors.New("wrap: no active keys")
	}
	for _, entry := range s.entries {
		m, err := obfsUnwrapPacket(entry.key, raw, dst)
		if err == nil {
			return append([]byte(nil), entry.key...), m, nil
		}
	}
	return nil, 0, errors.New("wrap: auth failed")
}

func refreshWrapKeysFromDBLocked() error {
	passwords := make([]string, 0, len(db.Passwords))
	for password, entry := range db.Passwords {
		if !isPasswordExpired(entry) {
			passwords = append(passwords, password)
		}
	}
	return serverWrapKeys.SetPasswords(db.MainPassword, passwords)
}

func initDB(dir, mainPass, adminID, botToken string) {
	dbFile = filepath.Join(dir, "passwords.json")
	db = &Database{
		Passwords: make(map[string]*PasswordEntry),
		Devices:   make(map[string]*ClientDevice),
	}
	data, err := os.ReadFile(dbFile)
	if err == nil {
		json.Unmarshal(data, db)
	}
	if db.Passwords == nil {
		db.Passwords = make(map[string]*PasswordEntry)
	}
	if db.Devices == nil {
		db.Devices = make(map[string]*ClientDevice)
	}
	migrateDatabaseDevices()
	db.MainPassword = mainPass
	db.AdminID = adminID
	db.BotToken = botToken
	if db.MainPassword != "" {
		ensureMainPasswordEntryLocked()
	}
	saveDB()
	if err := refreshWrapKeysFromDBLocked(); err != nil {
		log.Fatalf("[WRAP] init keys: %v", err)
	}
}

func saveDB() error {
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		log.Printf("[DB] save: marshal: %v", err)
		return err
	}
	tmp := dbFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		log.Printf("[DB] save: write: %v", err)
		return err
	}
	if err := os.Rename(tmp, dbFile); err != nil {
		log.Printf("[DB] save: rename: %v", err)
		return err
	}
	return nil
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

func deviceForWGIPLocked(ip string) (deviceID, password string, isMain bool) {
	if ip == "" {
		return "", "", false
	}
	for id, dev := range db.Devices {
		if dev == nil || dev.IP != ip {
			continue
		}
		pass := passwordForDeviceLocked(id)
		if pass == "" {
			continue
		}
		return id, pass, pass == db.MainPassword
	}
	return "", "", false
}

// resolveConnByWGLocalPort определяет устройство, когда клиент шлёт WG-пакеты без GETCONF
// (повторное подключение с сохранённым конфигом). wg show endpoint = 127.0.0.1:<localPort>.
func resolveConnByWGLocalPort(localPort int) (deviceID, userIP, password string, isMain bool) {
	if localPort <= 0 {
		return "", "", "", false
	}
	out, err := exec.Command("wg", "show", "wdtt0", "dump").Output()
	if err != nil {
		return "", "", "", false
	}
	needle := fmt.Sprintf(":%d", localPort)
	dbMutex.Lock()
	defer dbMutex.Unlock()
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 || !strings.Contains(fields[2], needle) {
			continue
		}
		allowed := strings.Split(fields[3], ",")[0]
		if allowed == "off" || !strings.Contains(allowed, ".") {
			continue
		}
		userIP = strings.TrimSuffix(allowed, "/32")
		pubKey := fields[0]
		for id, dev := range db.Devices {
			if dev == nil {
				continue
			}
			if dev.PubKey == pubKey || dev.IP == userIP {
				pass := passwordForDeviceLocked(id)
				if pass == "" {
					continue
				}
				if dev.IP != "" {
					userIP = dev.IP
				}
				return id, userIP, pass, pass == db.MainPassword
			}
		}
		if id, pass, main := deviceForWGIPLocked(userIP); id != "" {
			return id, userIP, pass, main
		}
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
	if _, ok := db.Passwords[db.MainPassword]; ok {
		return
	}
	db.Passwords[db.MainPassword] = &PasswordEntry{Comment: "Владелец"}
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
		saveDB()
		trafficDirty.Store(false)
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

func botLoop(token string, adminIDstr string, wgDev *device.Device) {
	if token == "" || adminIDstr == "" {
		return
	}
	adminID, _ := strconv.ParseInt(adminIDstr, 10, 64)
	if adminID == 0 {
		return
	}

	// Устанавливаем команды для синей кнопки Menu
	go func() {
		cmds := `{"commands":[{"command":"start","description":"Главное меню"},{"command":"new","description":"Создать временный пароль"},{"command":"list","description":"Управление доступами"}]}`
		resp, err := http.Post(fmt.Sprintf("https://api.telegram.org/bot%s/setMyCommands", token), "application/json", strings.NewReader(cmds))
		if err == nil {
			resp.Body.Close()
		}
	}()

	offset := 0
	client := &http.Client{Timeout: 65 * time.Second}

	// Состояние ожидания ввода
	var waitingForDays bool
	var waitingForPorts bool
	var waitingForHash bool
	var targetPassword string

	var tempDays int
	var tempPorts string // "dtls,wg,tun"

	for {
		url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?timeout=60&offset=%d", token, offset)
		resp, err := client.Get(url)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		var res struct {
			Ok     bool `json:"ok"`
			Result []struct {
				UpdateID int `json:"update_id"`
				Message  *struct {
					Chat struct {
						ID int64 `json:"id"`
					} `json:"chat"`
					Text string `json:"text"`
				} `json:"message"`
				CallbackQuery *struct {
					ID      string `json:"id"`
					Data    string `json:"data"`
					Message struct {
						MessageID int `json:"message_id"`
						Chat      struct {
							ID int64 `json:"id"`
						} `json:"chat"`
					} `json:"message"`
				} `json:"callback_query"`
			} `json:"result"`
		}

		err = json.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		for _, u := range res.Result {
			offset = u.UpdateID + 1

			// ═══ Callback кнопки ═══
			if u.CallbackQuery != nil && u.CallbackQuery.Message.Chat.ID == adminID {
				data := u.CallbackQuery.Data
				answerCallback(token, u.CallbackQuery.ID)

				if strings.HasPrefix(data, "viewpass_") {
					// Просмотр деталей пароля
					pass := strings.TrimPrefix(data, "viewpass_")
					dbMutex.Lock()
					entry, exists := db.Passwords[pass]
					if !exists || entry == nil {
						dbMutex.Unlock()
						sendTelegram(token, adminID, "❌ Пароль не найден", nil)
						continue
					}
					txt := fmt.Sprintf("🔑 *Пароль:* `%s`\n", pass)
					if entry.Ports != "" || entry.VkHash != "" {
						srvIP := getPublicIP()
						link := buildPublicWdttLink(srvIP, entry.Ports, pass, entry.Comment, entry.VkHash)
						txt += fmt.Sprintf("🔗 *Ссылка:* `%s`\n", link)
					}
					if entry.IsDeactivated {
						txt += "🔴 Статус: *ДЕАКТИВИРОВАН*\n"
					} else {
						txt += "🟢 Статус: *АКТИВЕН*\n"
					}
					
					if entry.ExpiresAt > 0 {
						expireTime := time.Unix(entry.ExpiresAt, 0)
						remaining := time.Until(expireTime)
						if remaining > 0 {
							txt += fmt.Sprintf("⏰ Истекает: %s (через %dd)\n", expireTime.Format("02.01.2006"), int(remaining.Hours()/24))
						} else {
							txt += "⏰ *ИСТЁК* ❌\n"
						}
					} else {
						txt += "⏰ Бессрочный ♾\n"
					}
					
					txt += fmt.Sprintf("\n📊 *Трафик:*\n• Скачано: %.2f MB\n• Отдано: %.2f MB\n", float64(entry.DownBytes)/(1024*1024), float64(entry.UpBytes)/(1024*1024))
					normalizeEntryDevices(entry)
					txt += fmt.Sprintf("\n📱 *Устройства:* %d/%d\n", len(entry.DeviceIDs), entryMaxDevices(entry))
					var kb []map[string]interface{}
					if len(entry.DeviceIDs) == 0 {
						txt += "_Ожидает первого подключения..._\n"
					} else {
						for i, devID := range entry.DeviceIDs {
							dev, devExists := db.Devices[devID]
							if devExists {
								txt += fmt.Sprintf("• `%s` → %s\n", devID, dev.IP)
							} else {
								txt += fmt.Sprintf("• `%s` (удалено)\n", devID)
							}
							kb = append(kb, map[string]interface{}{
								"text":          fmt.Sprintf("🗑 Отвязать #%d", i+1),
								"callback_data": fmt.Sprintf("ub1_%s_%d", pass, i),
							})
						}
						kb = append(kb, map[string]interface{}{
							"text":          "🗑 Отвязать все",
							"callback_data": "unbind_" + pass,
						})
					}
					dbMutex.Unlock()
					if entry.IsDeactivated {
						kb = append(kb, map[string]interface{}{
							"text":          "✅ Активировать",
							"callback_data": "react_" + pass,
						})
					} else {
						kb = append(kb, map[string]interface{}{
							"text":          "⏸ Деактивировать",
							"callback_data": "deact_" + pass,
						})
					}
					kb = append(kb, map[string]interface{}{
						"text":          "❌ Удалить пароль",
						"callback_data": "delpass_" + pass,
					})
					kb = append(kb, map[string]interface{}{
						"text":          "◀️ Назад к списку",
						"callback_data": "backlist",
					})
					var keyboard [][]map[string]interface{}
					for _, btn := range kb {
						keyboard = append(keyboard, []map[string]interface{}{btn})
					}
					sendTelegram(token, adminID, txt, map[string]interface{}{"inline_keyboard": keyboard})

				} else if strings.HasPrefix(data, "deact_") {
					pass := strings.TrimPrefix(data, "deact_")
					dbMutex.Lock()
					entry, exists := db.Passwords[pass]
					if exists && entry != nil {
						entry.IsDeactivated = true
						for _, devID := range allEntryDeviceIDs(entry) {
							if dev, devExists := db.Devices[devID]; devExists {
								if pubHex, err := b64ToHex(dev.PubKey); err == nil && pubHex != "" {
									wgDev.IpcSet(fmt.Sprintf("public_key=%s\nremove=true\n", pubHex))
								}
							}
						}
						saveDB()
					}
					dbMutex.Unlock()
					sendTelegram(token, adminID, fmt.Sprintf("⏸ Пароль `%s` деактивирован", pass), nil)

				} else if strings.HasPrefix(data, "react_") {
					pass := strings.TrimPrefix(data, "react_")
					dbMutex.Lock()
					entry, exists := db.Passwords[pass]
					if exists && entry != nil {
						entry.IsDeactivated = false
						saveDB()
					}
					dbMutex.Unlock()
					sendTelegram(token, adminID, fmt.Sprintf("✅ Пароль `%s` активирован", pass), nil)

				} else if data == "mainlink" {
					targetPassword = "main"
					var keyboard [][]map[string]interface{}
					keyboard = append(keyboard, []map[string]interface{}{
						{"text": "Да", "callback_data": "ports_def"},
						{"text": "Нет", "callback_data": "ports_custom"},
					})
					sendTelegram(token, adminID, "⚙️ Использовать стандартные порты для главного пароля (56000, 56001, 9000)?", map[string]interface{}{"inline_keyboard": keyboard})

				} else if strings.HasPrefix(data, "ub1_") {
					rest := strings.TrimPrefix(data, "ub1_")
					parts := strings.SplitN(rest, "_", 2)
					if len(parts) == 2 {
						pass := parts[0]
						idx, _ := strconv.Atoi(parts[1])
						dbMutex.Lock()
						entry, exists := db.Passwords[pass]
						if exists && entry != nil {
							normalizeEntryDevices(entry)
							if idx >= 0 && idx < len(entry.DeviceIDs) {
								devID := entry.DeviceIDs[idx]
								removeWGDeviceLocked(wgDev, devID)
								removeDeviceFromEntry(entry, devID)
								saveDB()
							}
						}
						dbMutex.Unlock()
						sendTelegram(token, adminID, fmt.Sprintf("✅ Устройство отвязано от `%s`", pass), nil)
					}

				} else if strings.HasPrefix(data, "unbind_") {
					pass := strings.TrimPrefix(data, "unbind_")
					dbMutex.Lock()
					entry, exists := db.Passwords[pass]
					if exists && entry != nil {
						removeAllEntryDevicesLocked(wgDev, entry)
						saveDB()
					}
					dbMutex.Unlock()
					sendTelegram(token, adminID, fmt.Sprintf("✅ Все устройства отвязаны от `%s`", pass), nil)

				} else if strings.HasPrefix(data, "delpass_") {
					pass := strings.TrimPrefix(data, "delpass_")
					dbMutex.Lock()
					entry, exists := db.Passwords[pass]
					if exists && entry != nil {
						for _, devID := range allEntryDeviceIDs(entry) {
							removeWGDeviceLocked(wgDev, devID)
						}
					}
					delete(db.Passwords, pass)
					serverWrapKeys.RemovePassword(pass)
					saveDB()
					dbMutex.Unlock()
					sendTelegram(token, adminID, fmt.Sprintf("✅ Пароль `%s` и его устройство удалены", pass), nil)

				} else if strings.HasPrefix(data, "deldev_") {
					devID := strings.TrimPrefix(data, "deldev_")
					dbMutex.Lock()
					dev, exists := db.Devices[devID]
					if exists {
						delete(db.Devices, devID)
						pubHex, _ := b64ToHex(dev.PubKey)
						wgDev.IpcSet(fmt.Sprintf("public_key=%s\nremove=true\n", pubHex))
						// Очищаем привязку из пароля
						for _, entry := range db.Passwords {
							if entry != nil {
								removeDeviceFromEntry(entry, devID)
							}
						}
						saveDB()
					}
					dbMutex.Unlock()
					sendTelegram(token, adminID, fmt.Sprintf("✅ Устройство `%s` удалено", devID), nil)

				} else if data == "backlist" {
					sendPasswordList(token, adminID, wgDev)
				} else if data == "ports_def" {
					tempPorts = "56000,56001,9000"
					waitingForHash = true
					sendTelegram(token, adminID, "🔑 Укажите VK хеш (или несколько через запятую):", nil)
				} else if data == "ports_custom" {
					waitingForPorts = true
					sendTelegram(token, adminID, "⚙️ Укажите через запятую 3 порта (DTLS,WG,TUN):\nНапример: 56000,56001,9000", nil)
				}
			}

			// ═══ Текстовые команды ═══
			msg := u.Message
			if msg == nil || msg.Chat.ID != adminID {
				continue
			}

			cmd := strings.TrimSpace(msg.Text)

			// Обработка ввода количества дней
			if waitingForDays {
				waitingForDays = false
				days, parseErr := strconv.Atoi(cmd)
				if parseErr != nil || days < 1 || days > 365 {
					sendTelegram(token, adminID, "❌ Неверное значение. Укажите число от 1 до 365, или отправьте /new заново.", nil)
					continue
				}
				tempDays = days
				
				var keyboard [][]map[string]interface{}
				keyboard = append(keyboard, []map[string]interface{}{
					{"text": "Да", "callback_data": "ports_def"},
					{"text": "Нет", "callback_data": "ports_custom"},
				})
				sendTelegram(token, adminID, "⚙️ Использовать стандартные порты (56000, 56001, 9000)?", map[string]interface{}{"inline_keyboard": keyboard})
				continue
			}

			if waitingForPorts {
				parts := strings.Split(cmd, ",")
				if len(parts) != 3 {
					sendTelegram(token, adminID, "❌ Неверный формат. Укажите 3 порта через запятую (например: 56000,56001,9000):", nil)
					continue
				}
				p1 := strings.TrimSpace(parts[0])
				p2 := strings.TrimSpace(parts[1])
				p3 := strings.TrimSpace(parts[2])
				
				if _, err := strconv.Atoi(p1); err != nil {
					sendTelegram(token, adminID, "❌ Неверный порт. Повторите ввод:", nil)
					continue
				}
				if _, err := strconv.Atoi(p2); err != nil {
					sendTelegram(token, adminID, "❌ Неверный порт. Повторите ввод:", nil)
					continue
				}
				if _, err := strconv.Atoi(p3); err != nil {
					sendTelegram(token, adminID, "❌ Неверный порт. Повторите ввод:", nil)
					continue
				}
				
				waitingForPorts = false
				tempPorts = fmt.Sprintf("%s,%s,%s", p1, p2, p3)
				waitingForHash = true
				sendTelegram(token, adminID, "🔑 Укажите VK хеш (или несколько через запятую):", nil)
				continue
			}

			if waitingForHash {
				hash := strings.ReplaceAll(cmd, " ", "")
				if strings.Contains(hash, "http") || strings.Contains(hash, "/") {
					sendTelegram(token, adminID, "❌ Пожалуйста, отправьте только хеш (или несколько хешей через запятую). Ссылки не поддерживаются.", nil)
					continue
				}
				if hash == "" {
					sendTelegram(token, adminID, "❌ Хеш не должен быть пустым.", nil)
					continue
				}
				waitingForHash = false

				if targetPassword == "main" {
					targetPassword = ""
					srvIP := getPublicIP()
					link := buildPublicWdttLink(srvIP, tempPorts, db.MainPassword, "WDTT", hash)
					sendTelegram(token, adminID, fmt.Sprintf("🔗 *Ссылка:* `%s`", link), nil)
					continue
				}

				dbMutex.Lock()
				suspendExpiredPasswordsLocked(wgDev)
				if countActivePasswordsLocked() >= maxGeneratedPasswords {
					dbMutex.Unlock()
					sendTelegram(token, adminID, fmt.Sprintf("❌ Лимит паролей: максимум %d активных. Удалите ненужный пароль через /list.", maxGeneratedPasswords), nil)
					continue
				}
				newPass := ""
				for i := 0; i < 10; i++ {
					candidate := generatePassword()
					if _, exists := db.Passwords[candidate]; !exists {
						newPass = candidate
						break
					}
				}
				if newPass == "" {
					dbMutex.Unlock()
					sendTelegram(token, adminID, "❌ Не удалось создать уникальный пароль. Повторите /new.", nil)
					continue
				}
				if err := serverWrapKeys.AddPassword(newPass); err != nil {
					dbMutex.Unlock()
					sendTelegram(token, adminID, "❌ Не удалось создать WRAP-ключ для пароля. Повторите /new.", nil)
					continue
				}
				expiresAt := time.Now().Add(time.Duration(tempDays) * 24 * time.Hour).Unix()
				db.Passwords[newPass] = &PasswordEntry{
					ExpiresAt: expiresAt,
					VkHash:    hash,
					Ports:     tempPorts,
				}
				saveDB()
				dbMutex.Unlock()

				expDate := time.Unix(expiresAt, 0).Format("02.01.2006")
				srvIP := getPublicIP()
				link := buildPublicWdttLink(srvIP, tempPorts, newPass, "", hash)

				sendTelegram(token, adminID, fmt.Sprintf("🔑 Новый пароль:\n`%s`\n\n⏰ Действует %d дн. (до %s)\n📱 Ожидает первого подключения\n\n🔗 *Ссылка:* `%s`", newPass, tempDays, expDate, link), nil)
				continue
			}

			if cmd == "/start" || cmd == "/help" {
				sendTelegram(token, adminID, "🤖 *WDTT VPN Manager*\n\n/new — Создать пароль\n/list — Список паролей", nil)

			} else if cmd == "/new" {
				dbMutex.Lock()
				suspendExpiredPasswordsLocked(wgDev)
				if countActivePasswordsLocked() >= maxGeneratedPasswords {
					dbMutex.Unlock()
					sendTelegram(token, adminID, fmt.Sprintf("❌ Лимит паролей: максимум %d активных. Удалите ненужный пароль через /list.", maxGeneratedPasswords), nil)
					continue
				}
				dbMutex.Unlock()
				waitingForDays = true
				sendTelegram(token, adminID, "📅 Введите срок действия пароля в днях (1–365):\n\n_Примеры: 30 = месяц, 365 = год_", nil)

			} else if cmd == "/list" {
				sendPasswordList(token, adminID, wgDev)
			}
		}
	}
}

func removePeerFromWG(wgDev *device.Device, dev *ClientDevice) {
	if wgDev == nil || dev == nil || dev.PubKey == "" {
		return
	}
	pubHex, err := b64ToHex(dev.PubKey)
	if err != nil {
		return
	}
	wgDev.IpcSet(fmt.Sprintf("public_key=%s\nremove=true\n", pubHex))
}

func upsertPeerInWG(wgDev *device.Device, dev *ClientDevice) {
	if wgDev == nil || dev == nil || dev.PubKey == "" || dev.IP == "" {
		return
	}
	pubHex, err := b64ToHex(dev.PubKey)
	if err != nil {
		return
	}
	wgDev.IpcSet(fmt.Sprintf("public_key=%s\nallowed_ip=%s/32\n", pubHex, dev.IP))
}

func countActivePasswordsLocked() int {
	n := 0
	for _, entry := range db.Passwords {
		if entry != nil && !isPasswordExpired(entry) {
			n++
		}
	}
	return n
}

// Отключает истёкших от WG, но оставляет в базе для продления в панели.
func suspendExpiredPasswordsLocked(wgDev *device.Device) int {
	suspended := 0
	for _, entry := range db.Passwords {
		if entry == nil || !isPasswordExpired(entry) {
			continue
		}
		for _, devID := range allEntryDeviceIDs(entry) {
			if dev, ok := db.Devices[devID]; ok {
				removePeerFromWG(wgDev, dev)
			}
		}
		suspended++
	}
	return suspended
}

func suspendExpiredPasswords(wgDev *device.Device) int {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	return suspendExpiredPasswordsLocked(wgDev)
}

func expiredPasswordJanitor(ctx context.Context, wgDev *device.Device) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n := suspendExpiredPasswords(wgDev); n > 0 {
				log.Printf("[DB] Отключено истёкших паролей (остались в базе): %d", n)
			}
		}
	}
}

func syncPersistedPeersToWG(wgDev *device.Device) {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	count := 0
	for deviceID, dev := range db.Devices {
		pass := passwordForDeviceLocked(deviceID)
		if pass == "" {
			continue
		}
		entry := db.Passwords[pass]
		if entry == nil || isPasswordExpired(entry) || entry.IsDeactivated || isTrafficExceeded(entry) {
			continue
		}
		upsertPeerInWG(wgDev, dev)
		count++
	}
	if count > 0 {
		log.Printf("[WG] Восстановлено сохранённых устройств: %d", count)
	}
}

func sendPasswordList(token string, adminID int64, wgDev *device.Device) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	suspendExpiredPasswordsLocked(wgDev)

	txt := "🔐 *Пароли:*\n\n"
	txt += fmt.Sprintf("🔒 Главный: `%s` (владелец)\n\n", db.MainPassword)

	var inlineKb []map[string]interface{}
	inlineKb = append(inlineKb, map[string]interface{}{
		"text":          "🔗 Ссылка на главный пароль",
		"callback_data": "mainlink",
	})

	if len(db.Passwords) == 0 {
		txt += "_Нет сгенерированных паролей._\n"
	} else {
		txt += fmt.Sprintf("_Активно: %d/%d (всего в базе: %d)_\n\n", countActivePasswordsLocked(), maxGeneratedPasswords, len(db.Passwords))
		for p, entry := range db.Passwords {
			status := "🟢"
			normalizeEntryDevices(entry)
			if len(entry.DeviceIDs) > 0 {
				status = "🔗"
			}
			expiry := "♾"
			if entry.ExpiresAt > 0 {
				remaining := time.Until(time.Unix(entry.ExpiresAt, 0))
				if remaining > 0 {
					expiry = fmt.Sprintf("%dd", int(remaining.Hours()/24)+1)
				} else {
					expiry = "❌"
				}
			}
			txt += fmt.Sprintf("%s `%s` (%s)\n", status, p, expiry)
			inlineKb = append(inlineKb, map[string]interface{}{
				"text":          "🔍 " + p,
				"callback_data": "viewpass_" + p,
			})
		}
	}

	txt += "\n🟢 = свободен | 🔗 = привязан"

	var replyMarkup interface{}
	if len(inlineKb) > 0 {
		var keyboard [][]map[string]interface{}
		for _, btn := range inlineKb {
			keyboard = append(keyboard, []map[string]interface{}{btn})
		}
		replyMarkup = map[string]interface{}{"inline_keyboard": keyboard}
	}
	sendTelegram(token, adminID, txt, replyMarkup)
}

func answerCallback(token, callbackID string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/answerCallbackQuery", token)
	payload := map[string]interface{}{"callback_query_id": callbackID}
	body, _ := json.Marshal(payload)
	http.Post(url, "application/json", bytes.NewBuffer(body))
}

func maskPassword(pass string) string {
	if len(pass) <= 3 {
		return pass
	}
	return pass[:3] + "****"
}

func sendTelegram(token string, chatID int64, text string, replyMarkup interface{}) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	if replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}
	body, _ := json.Marshal(payload)
	http.Post(url, "application/json", bytes.NewBuffer(body))
}

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

const userIdleTimeout = 60 * time.Second

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
	delete(onlineUsers, deviceID)
	onlineUsersMutex.Unlock()
	atomic.AddInt32(&activeUsers, -1)
	log.Printf("[ОТКЛ] %s | %s | WG %s", label, deviceID, ip)
}

func userTouchActivity(deviceID string) {
	if deviceID == "" {
		return
	}
	now := time.Now()
	onlineUsersMutex.Lock()
	defer onlineUsersMutex.Unlock()

	info, exists := onlineUsers[deviceID]
	if !exists {
		return
	}
	info.LastActivity = now
}

func userPresenceSweep() {
	now := time.Now()
	var stale []string

	onlineUsersMutex.Lock()
	for deviceID, info := range onlineUsers {
		if info.Sessions > 0 && now.Sub(info.LastActivity) >= userIdleTimeout {
			stale = append(stale, deviceID)
		}
	}
	onlineUsersMutex.Unlock()

	for _, deviceID := range stale {
		onlineUsersMutex.Lock()
		info, exists := onlineUsers[deviceID]
		if !exists || info.Sessions <= 0 || now.Sub(info.LastActivity) < userIdleTimeout {
			onlineUsersMutex.Unlock()
			continue
		}
		label, ip, sessions := info.Label, info.IP, info.Sessions
		delete(onlineUsers, deviceID)
		onlineUsersMutex.Unlock()
		atomic.AddInt32(&activeUsers, -1)
		log.Printf("[ОТКЛ] stale %s | %s | WG %s (sessions=%d)", label, deviceID, ip, sessions)
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

func snapshotOnlineUsers() []map[string]string {
	onlineUsersMutex.Lock()
	defer onlineUsersMutex.Unlock()
	out := make([]map[string]string, 0, len(onlineUsers))
	for deviceID, info := range onlineUsers {
		if info.Sessions <= 0 {
			continue
		}
		out = append(out, map[string]string{
			"device_id": deviceID,
			"user":      info.Label,
			"password":  info.Password,
			"ip":        info.IP,
			"sessions":  strconv.Itoa(info.Sessions),
		})
	}
	return out
}

func mergeOnlineWithWGPeers(sessionOnline []map[string]string) []map[string]string {
	byDevice := make(map[string]map[string]string, len(sessionOnline)+2)
	for _, o := range sessionOnline {
		if id := o["device_id"]; id != "" {
			byDevice[id] = o
		}
	}
	wgOut, err := exec.Command("wg", "show", "wdtt0", "dump").Output()
	if err != nil {
		result := make([]map[string]string, 0, len(byDevice))
		for _, o := range byDevice {
			result = append(result, o)
		}
		return result
	}
	now := time.Now().Unix()
	dbMutex.Lock()
	defer dbMutex.Unlock()
	for _, line := range strings.Split(string(wgOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			continue
		}
		handshake, _ := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		if handshake <= 0 || now-handshake > 180 {
			continue
		}
		allowed := strings.Split(fields[3], ",")[0]
		ip := strings.TrimSuffix(allowed, "/32")
		id, pass, isMain := deviceForWGIPLocked(ip)
		if id == "" || byDevice[id] != nil {
			continue
		}
		byDevice[id] = map[string]string{
			"device_id": id,
			"user":      userLabel(pass, isMain),
			"password":  pass,
			"ip":        ip,
			"sessions":  "1",
		}
	}
	result := make([]map[string]string, 0, len(byDevice))
	for _, o := range byDevice {
		result = append(result, o)
	}
	return result
}

// syncTrafficFromWGPeers начисляет трафик по дельте rx/tx из wg show dump.
// Надёжнее per-session flush: один peer может иметь несколько DTLS-прокси без GETCONF.
func syncTrafficFromWGPeers() {
	out, err := exec.Command("wg", "show", "wdtt0", "dump").Output()
	if err != nil {
		return
	}
	dbMutex.Lock()
	defer dbMutex.Unlock()
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		pubKey := fields[0]
		rx, errRx := strconv.ParseInt(strings.TrimSpace(fields[5]), 10, 64)
		tx, errTx := strconv.ParseInt(strings.TrimSpace(fields[6]), 10, 64)
		if errRx != nil || errTx != nil {
			continue
		}
		var deviceID string
		for id, dev := range db.Devices {
			if dev != nil && dev.PubKey == pubKey {
				deviceID = id
				break
			}
		}
		if deviceID == "" {
			continue
		}
		pass := passwordForDeviceLocked(deviceID)
		if pass == "" {
			continue
		}
		cur := wgPeerCounters{rx: rx, tx: tx}
		prevVal, hadPrev := wgPeerTrafficLast.Load(deviceID)
		wgPeerTrafficLast.Store(deviceID, cur)
		if !hadPrev {
			continue
		}
		prev := prevVal.(wgPeerCounters)
		if rx < prev.rx || tx < prev.tx {
			continue
		}
		dRx := rx - prev.rx
		dTx := tx - prev.tx
		if dRx > 0 {
			addTrafficLocked(pass, dRx, false)
		}
		if dTx > 0 {
			addTrafficLocked(pass, dTx, true)
		}
	}
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
			online := mergeOnlineWithWGPeers(snapshotOnlineUsers())
			users := int32(len(online))
			atomic.StoreInt32(&activeUsers, users)
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

			if trafficDirty.Load() {
				dbMutex.Lock()
				saveDB()
				trafficDirty.Store(false)
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

// ==================== Main ====================

func main() {
	listen := flag.String("listen", "0.0.0.0:56000", "DTLS адрес")
	wgPort := flag.Int("wg-port", defaultInternalWGPort, "WireGuard UDP порт")
	configDir := flag.String("config-dir", "/etc/wdtt", "директория конфигурации")
	mainPass := flag.String("password", "", "пароль владельца")
	adminID := flag.String("admin", "", "Telegram Admin ID")
	botToken := flag.String("bot-token", "", "Telegram Bot Token")
	handshakeTimeout := flag.Duration("handshake-timeout", 30*time.Second, "таймаут DTLS handshake")
	adminAddr := flag.String("admin-addr", "127.0.0.1:2861", "HTTP admin (localhost): /health, POST /admin/reload")
	maxDTLSPerDeviceFlag := flag.Int("max-dtls-per-device", 0, "макс. параллельных GETCONF на device_id (0 = без лимита)")
	flag.Parse()
	dtlsHandshakeTimeout = *handshakeTimeout
	maxDTLSPerDevice = int32(*maxDTLSPerDeviceFlag)
	adminListenAddr = *adminAddr

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("══════════════════════════════════════════")
	log.Println("   WDTT Server v2 (Multi-User)")
	log.Println("══════════════════════════════════════════")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		log.Println("[SERVER] Shutdown signal received, draining connections...")
		cancel()
	}()

	initDB(*configDir, *mainPass, *adminID, *botToken)
	loadInboundSettings(*configDir)
	log.Printf("[CFG] DNS клиентов: %s, MTU: %d, лимит активных: %d", clientDNS, wgMTU, maxGeneratedPasswords)
	log.Printf("[CFG] Admin HTTP: %s (hot-reload /health)", adminListenAddr)

	keys, err := loadOrGenerateKeys(*configDir)
	if err != nil {
		log.Fatalf("[WG] Ключи: %v", err)
	}

	enableBBR()

	wgDev, err := startUserspaceWG(keys, *wgPort)
	if err != nil {
		log.Fatalf("[WG] Запуск: %v", err)
	}
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

	go statsLoop(ctx, *configDir)
	go userPresenceLoop(ctx)
	go expiredPasswordJanitor(ctx, wgDev)
	go getconfFailJanitor(ctx)
	go botLoop(*botToken, *adminID, wgDev)
	startAdminServer(ctx, wgDev)

	addr, _ := net.ResolveUDPAddr("udp", *listen)
	cert, err := loadOrGenerateDTLSCert(*configDir)
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

	listener, err := dtls.NewListenerWithOptions(wrapListener, dtls.WithCertificates(cert), dtls.WithExtendedMasterSecret(dtls.RequireExtendedMasterSecret), dtls.WithCipherSuites(dtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256), dtls.WithConnectionIDGenerator(dtls.RandomCIDGenerator(8)))
	if err != nil {
		log.Fatalf("[DTLS] %v", err)
	}
	context.AfterFunc(ctx, func() { listener.Close() })

	wgEndpoint := fmt.Sprintf("127.0.0.1:%d", *wgPort)

	log.Printf("   DTLS: %s | WG: %s | NAT: %s", *listen, wgEndpoint, natType)
	log.Printf("   WRAP: password HKDF + RTP AEAD | keys: %d", serverWrapKeys.Count())
	log.Println("[SERVER] Готов")

	var wg sync.WaitGroup
	for {
		dtlsConn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
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
					saveDB()
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

func handleConn(ctx context.Context, clientConn net.Conn, wgEndpoint string, wgDev *device.Device, keys *wgKeys) {
	atomic.AddInt64(&totalConns, 1)

	var connPassword string
	var connIsMainPass bool
	var connDeviceID string
	var connUserIP string

	dtlsConn, ok := clientConn.(*dtls.Conn)
	if !ok {
		return
	}

	remoteAddr := clientConn.RemoteAddr()
	hsTimeout := relayHandshakeTimeoutFor(remoteAddr)
	hctx, hcancel := context.WithTimeout(ctx, hsTimeout)
	if err := dtlsConn.HandshakeContext(hctx); err != nil {
		log.Printf("[DTLS] Handshake failed from %s (timeout %s): %v", remoteAddr, hsTimeout, err)
		relayNoteHandshakeFail(remoteAddr)
		hcancel()
		return
	}
	relayNoteHandshakeOK(remoteAddr)
	hcancel()

	atomic.AddInt32(&activeConns, 1)
	defer atomic.AddInt32(&activeConns, -1)

	buf := make([]byte, 1600)
	clientConn.SetReadDeadline(time.Now().Add(30 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		log.Printf("[DTLS] First read failed from %s: %v", clientConn.RemoteAddr(), err)
		return
	}
	clientConn.SetReadDeadline(time.Time{})

	firstPacket := buf[:n]
	firstStr := string(firstPacket)

	if !strings.HasPrefix(firstStr, "GETCONF:") {
		preview := firstStr
		if len(preview) > 32 {
			preview = preview[:32] + "..."
		}
		log.Printf("[DTLS] Unexpected first packet from %s: %q", clientConn.RemoteAddr(), preview)
	}

	if strings.HasPrefix(firstStr, "GETCONF:") {
		parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(firstStr, "GETCONF:")), "|")
		clientPort := "9000"
		deviceID := "unknown"
		password := ""
		if len(parts) > 0 {
			clientPort = parts[0]
		}
		if len(parts) > 1 {
			deviceID = parts[1]
		}
		if len(parts) > 2 {
			password = parts[2]
		}

		dbMutex.Lock()

		// Проверяем пароль
		isMainPass := password != "" && password == db.MainPassword
		entry, isGenPass := db.Passwords[password]
		valid := isMainPass || (isGenPass && !isPasswordExpired(entry) && !isTrafficExceeded(entry))

		if valid && isGenPass && entry.IsDeactivated {
			clientConn.Write([]byte("DENIED:deactivated"))
			log.Printf("[WG] Отказ: пароль %s деактивирован, запрос от %s", maskPassword(password), deviceID)
			dbMutex.Unlock()
		} else if valid && isGenPass && !entryCanAcceptDevice(entry, deviceID) {
			clientConn.Write([]byte("DENIED:device_mismatch"))
			log.Printf("[WG] Отказ: пароль %s — лимит устройств %d/%d, запрос от %s",
				maskPassword(password), len(allEntryDeviceIDs(entry)), entryMaxDevices(entry), deviceID)
			dbMutex.Unlock()
		} else if valid {
			if maxDTLSPerDevice > 0 && deviceSessionCount(deviceID) >= int(maxDTLSPerDevice) {
				clientConn.Write([]byte("DENIED:too_many_sessions"))
				log.Printf("[WG] Отказ: лимит %d DTLS-сессий для %s", maxDTLSPerDevice, deviceID)
				dbMutex.Unlock()
				return
			}

			connPassword = password
			connIsMainPass = isMainPass

			if isGenPass && !entryHasDevice(entry, deviceID) {
				if bindDeviceToEntry(entry, deviceID) {
					saveDB()
					log.Printf("[WG] Пароль %s привязан к %s (%d/%d)",
						maskPassword(password), deviceID, len(entry.DeviceIDs), entryMaxDevices(entry))
				}
			}

			dev, exists := db.Devices[deviceID]
			if !exists {
				dev = &ClientDevice{DeviceID: deviceID, IP: getNextIP()}
				privB64, pubB64, keyErr := generateKeyPair()
				if keyErr == nil && dev.IP != "" {
					dev.PrivKey = privB64
					dev.PubKey = pubB64
					db.Devices[deviceID] = dev
					saveDB()
					log.Printf("[WG] Новое устройство %s (IP: %s)", deviceID, dev.IP)
				} else {
					dev = nil
				}
			} else if dev.PrivKey == "" || dev.PubKey == "" {
				privB64, pubB64, keyErr := generateKeyPair()
				if keyErr == nil {
					dev.PrivKey = privB64
					dev.PubKey = pubB64
					if dev.IP == "" {
						dev.IP = getNextIP()
					}
					db.Devices[deviceID] = dev
					saveDB()
					log.Printf("[WG] Восстановлены ключи устройства %s (IP: %s)", deviceID, dev.IP)
				} else {
					dev = nil
				}
			}
			if dev != nil {
				connDeviceID = deviceID
				connUserIP = dev.IP
				upsertPeerInWG(wgDev, dev)
				if isGenPass && entry != nil {
					applySpeedLimitForEntryUnlocked(entry)
				}
				clientConn.Write([]byte(buildClientConfig(keys.serverPublic, dev.PrivKey, dev.IP, clientPort)))
				log.Printf("[WG] GETCONF OK: device=%s ip=%s from %s", deviceID, dev.IP, clientConn.RemoteAddr())
				getconfFailReset(clientConn.RemoteAddr())
			} else {
				clientConn.Write([]byte("NOCONF"))
			}
			dbMutex.Unlock()
		} else {
			if isGenPass && isTrafficExceeded(entry) {
				clientConn.Write([]byte("DENIED:traffic_exceeded"))
				log.Printf("[WG] Отказ: лимит трафика для %s, от %s", maskPassword(password), deviceID)
			} else if isGenPass && isPasswordExpired(entry) {
				clientConn.Write([]byte("DENIED:expired"))
				log.Printf("[WG] Отказ: пароль %s истёк, от %s", maskPassword(password), deviceID)
			} else {
				clientConn.Write([]byte("DENIED:wrong_password"))
				getconfFailLog(clientConn.RemoteAddr(), "wrong_password")
			}
			dbMutex.Unlock()
		}
		if connDeviceID != "" {
			relaySessionEvictIdle(connDeviceID, relayStaleEvictIdle)
		}

		clientConn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		n, err = clientConn.Read(buf)
		if err != nil {
			return
		}
		clientConn.SetReadDeadline(time.Time{})
		firstPacket = buf[:n]
		firstStr = string(firstPacket)
	}

	if firstStr == "READY" {
		clientConn.Write([]byte("READY_OK"))
		clientConn.SetReadDeadline(time.Now().Add(10 * time.Minute))
		n, err = clientConn.Read(buf)
		if err != nil {
			return
		}
		clientConn.SetReadDeadline(time.Time{})
		firstPacket = buf[:n]
	}

	// WG прокси
	wgConn, err := net.Dial("udp", wgEndpoint)
	if err != nil {
		return
	}
	defer wgConn.Close()

	if uc, ok := wgConn.(*net.UDPConn); ok {
		uc.SetReadBuffer(2 * 1024 * 1024)
		uc.SetWriteBuffer(2 * 1024 * 1024)
	}

	if _, err := wgConn.Write(firstPacket); err != nil {
		return
	}
	atomic.AddInt64(&totalBytesFromClient, int64(len(firstPacket)))

	// Клиент с сохранённым конфигом часто шлёт WG-пакеты сразу, без GETCONF.
	if connDeviceID == "" {
		id, ip, pass, main := tryResolveConnFromWG(wgConn)
		if id != "" {
			connDeviceID, connUserIP, connPassword, connIsMainPass = id, ip, pass, main
		}
	}

	pctx, pcancel := context.WithCancel(ctx)
	defer pcancel()

	var relaySess *relaySession
	var sessionEntered bool
	var sessionMu sync.Mutex

	enterSession := func() (ok, done bool) {
		sessionMu.Lock()
		defer sessionMu.Unlock()
		if sessionEntered {
			return true, true
		}
		if connDeviceID == "" {
			localPort := 0
			if ua, ok := wgConn.LocalAddr().(*net.UDPAddr); ok && ua != nil {
				localPort = ua.Port
			}
			if id, ip, pass, main := resolveConnByWGLocalPort(localPort); id != "" {
				connDeviceID, connUserIP, connPassword, connIsMainPass = id, ip, pass, main
				log.Printf("[WG] Без GETCONF: device=%s ip=%s pass=%s", id, ip, maskPassword(pass))
			}
		}
		if connDeviceID == "" {
			return false, false
		}
		if !userSessionEnter(connDeviceID, connUserIP, userLabel(connPassword, connIsMainPass), connPassword) {
			return false, true
		}
		sessionEntered = true
		relaySess = relaySessionRegister(connDeviceID, pcancel)
		return true, true
	}

	defer func() {
		sessionMu.Lock()
		id := connDeviceID
		entered := sessionEntered
		rs := relaySess
		sessionMu.Unlock()
		if entered && id != "" {
			userSessionLeave(id)
			if rs != nil {
				relaySessionUnregister(id, rs)
			}
		}
	}()

	if ok, done := enterSession(); done && !ok {
		log.Printf("[WG] Отказ: лимит %d DTLS-сессий для device=%s", maxDTLSPerDevice, connDeviceID)
		return
	}

	// Учёт трафика в passwords.json — syncTrafficFromWGPeers (statsLoop).
	// Здесь только сброс локальных счётчиков и проверка лимита/деактивации.
	var sessUpBytes, sessDownBytes int64
	flushTraffic := func() bool {
		atomic.SwapInt64(&sessUpBytes, 0)
		atomic.SwapInt64(&sessDownBytes, 0)
		if ok, done := enterSession(); done && !ok {
			return false
		}
		dbMutex.Lock()
		defer dbMutex.Unlock()
		pass := resolveTrafficPassword(connPassword, connIsMainPass, connDeviceID)
		if pass == "" {
			return true
		}
		e, ok := db.Passwords[pass]
		if !ok || e == nil {
			return true
		}
		return !isTrafficExceeded(e) && !e.IsDeactivated
	}
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-pctx.Done():
				flushTraffic()
				return
			case <-t.C:
				if !flushTraffic() {
					pcancel()
					return
				}
			}
		}
	}()

	context.AfterFunc(pctx, func() {
		clientConn.SetDeadline(time.Now())
		wgConn.SetDeadline(time.Now())
	})

	var proxyWg sync.WaitGroup
	proxyWg.Add(2)

	// Клиент → WG
	go func() {
		defer proxyWg.Done()
		defer pcancel()
		b := getBuf()
		defer putBuf(b)
		for {
			select {
			case <-pctx.Done():
				return
			default:
			}
			clientConn.SetReadDeadline(time.Now().Add(30 * time.Minute))
			nn, err := clientConn.Read(*b)
			if err != nil {
				return
			}
			// Skip DTLS keepalive packets (1-byte 0xFF ping from client)
			if nn == 1 && (*b)[0] == 0xFF {
				if relaySess != nil {
					relaySess.touch()
				}
				continue
			}
			atomic.AddInt64(&totalBytesFromClient, int64(nn))
			if ok, done := enterSession(); done && !ok {
				return
			}
			userTouchActivity(connDeviceID)
			if relaySess != nil {
				relaySess.touch()
			}
			atomic.AddInt64(&sessUpBytes, int64(nn))
			if _, err := wgConn.Write((*b)[:nn]); err != nil {
				return
			}
		}
	}()

	// WG → Клиент
	go func() {
		defer proxyWg.Done()
		defer pcancel()
		b := getBuf()
		defer putBuf(b)
		for {
			select {
			case <-pctx.Done():
				return
			default:
			}
			wgConn.SetReadDeadline(time.Now().Add(30 * time.Minute))
			nn, err := wgConn.Read(*b)
			if err != nil {
				if isNetTimeout(err) {
					if pctx.Err() != nil {
						return
					}
					continue
				}
				return
			}
			atomic.AddInt64(&totalBytesToClient, int64(nn))
			if ok, done := enterSession(); done && !ok {
				return
			}
			userTouchActivity(connDeviceID)
			if relaySess != nil {
				relaySess.touch()
			}
			atomic.AddInt64(&sessDownBytes, int64(nn))
			if _, err := clientConn.Write((*b)[:nn]); err != nil {
				return
			}
		}
	}()

	proxyWg.Wait()
}

const (
	wrapNonceLen = 12
	wrapKeyLen   = 32
)

var aeadCache sync.Map

func getAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != wrapKeyLen {
		return nil, fmt.Errorf("obfs: key must be %d bytes", wrapKeyLen)
	}
	keyStr := string(key)
	if val, ok := aeadCache.Load(keyStr); ok {
		return val.(cipher.AEAD), nil
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	aeadCache.Store(keyStr, aead)
	return aead, nil
}

// ==================== RTP Обфускация ====================

type ObfsConfig struct {
	SSRC        uint32
	PayloadType uint8
	PaddingMax  int
}

type ObfsState struct {
	mu      sync.Mutex
	initSeq uint16
	initTs  uint32
	count   uint64
}

func NewObfsConfig() *ObfsConfig {
	var buf [4]byte
	rand.Read(buf[:])
	return &ObfsConfig{
		SSRC:        binary.BigEndian.Uint32(buf[:]),
		PayloadType: 111,
		PaddingMax:  24,
	}
}

func NewObfsState() *ObfsState {
	var buf [6]byte
	rand.Read(buf[:])
	return &ObfsState{
		initSeq: binary.BigEndian.Uint16(buf[0:2]),
		initTs:  binary.BigEndian.Uint32(buf[2:6]),
		count:   0,
	}
}

func obfsBuildNonce(ssrc uint32, seq uint16, ts uint32) []byte {
	n := make([]byte, 12)
	binary.BigEndian.PutUint32(n[0:4], ssrc)
	binary.BigEndian.PutUint16(n[4:6], seq)
	binary.BigEndian.PutUint32(n[8:12], ts)
	return n
}

func obfsWrapPacket(key, payload []byte, cfg *ObfsConfig, state *ObfsState) ([]byte, error) {
	if len(key) != wrapKeyLen {
		return nil, fmt.Errorf("obfs: key must be %d bytes (got %d)", wrapKeyLen, len(key))
	}
	if len(payload) == 0 {
		return nil, errors.New("obfs: empty payload")
	}
	state.mu.Lock()
	c := state.count
	state.count++
	state.mu.Unlock()

	seq := state.initSeq + uint16(c)
	ts := state.initTs + uint32(c)*960 + uint32(c>>16)

	nonce := obfsBuildNonce(cfg.SSRC, seq, ts)
	padRand := 0
	if cfg.PaddingMax > 0 {
		var rndBuf [1]byte
		rand.Read(rndBuf[:])
		padRand = int(rndBuf[0]) % cfg.PaddingMax
	}
	padTotal := padRand + 1
	outLen := 12 + len(payload) + chacha20poly1305.Overhead + padTotal
	out := make([]byte, outLen)

	out[0] = 0x80 | 0x20
	out[1] = cfg.PayloadType & 0x7F
	binary.BigEndian.PutUint16(out[2:4], seq)
	binary.BigEndian.PutUint32(out[4:8], ts)
	binary.BigEndian.PutUint32(out[8:12], cfg.SSRC)

	aead, err := getAEAD(key)
	if err != nil {
		return nil, fmt.Errorf("obfs: cipher init: %w", err)
	}
	sealed := aead.Seal(out[12:12], nonce, payload, out[:12])
	padStart := 12 + len(sealed)
	if padRand > 0 {
		rand.Read(out[padStart : padStart+padRand])
	}
	out[outLen-1] = byte(padTotal)
	return out, nil
}

func obfsUnwrapPacket(key, wire, dst []byte) (int, error) {
	if len(key) != wrapKeyLen {
		return 0, fmt.Errorf("obfs: key must be %d bytes (got %d)", wrapKeyLen, len(key))
	}
	if len(wire) < 13 {
		return 0, errors.New("obfs: packet too short")
	}
	if (wire[0] >> 6) != 2 {
		return 0, errors.New("obfs: not RTP v2")
	}
	seq := binary.BigEndian.Uint16(wire[2:4])
	ts := binary.BigEndian.Uint32(wire[4:8])
	ssrc := binary.BigEndian.Uint32(wire[8:12])

	payloadEnd := len(wire)
	if wire[0]&0x20 != 0 {
		padLen := int(wire[len(wire)-1])
		if padLen == 0 || padLen > payloadEnd-12 {
			return 0, fmt.Errorf("obfs: invalid padding length %d", padLen)
		}
		payloadEnd -= padLen
	}
	ciphertextLen := payloadEnd - 12
	if ciphertextLen <= chacha20poly1305.Overhead {
		return 0, errors.New("obfs: no payload")
	}
	if ciphertextLen-chacha20poly1305.Overhead > len(dst) {
		return 0, errors.New("obfs: dst buffer too small")
	}
	nonce := obfsBuildNonce(ssrc, seq, ts)
	aead, err := getAEAD(key)
	if err != nil {
		return 0, fmt.Errorf("obfs: cipher init: %w", err)
	}
	plain, err := aead.Open(dst[:0], nonce, wire[12:payloadEnd], wire[:12])
	if err != nil {
		return 0, fmt.Errorf("obfs: auth: %w", err)
	}
	return len(plain), nil
}

func obfsIsRTPPacket(wire []byte) bool {
	if len(wire) < 13 {
		return false
	}
	if (wire[0] >> 6) != 2 {
		return false
	}
	pt := wire[1] & 0x7F
	return pt == 111
}

func listenWrapped(addr *net.UDPAddr, keys *wrapKeyStore) (dtlsnet.PacketListener, error) {
	if keys == nil || keys.Count() == 0 {
		return nil, errors.New("wrap: no active keys")
	}
	inner, err := (&pionudp.ListenConfig{
		ReadBufferSize:  8 << 20,
		WriteBufferSize: 2 << 20,
	}).Listen("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("wrap: udp listen: %w", err)
	}
	log.Printf("[WRAP] UDP buffers: read=8MB write=2MB on %s", addr.String())
	return &wrapPacketListener{
		inner: dtlsnet.PacketListenerFromListener(inner),
		keys:  keys,
	}, nil
}

type wrapPacketListener struct {
	inner dtlsnet.PacketListener
	keys  *wrapKeyStore
}

func (l *wrapPacketListener) Accept() (net.PacketConn, net.Addr, error) {
	pc, addr, err := l.inner.Accept()
	if err != nil {
		return pc, addr, err
	}
	return &wrapPacketConn{inner: pc, keys: l.keys}, addr, nil
}

func (l *wrapPacketListener) Close() error   { return l.inner.Close() }
func (l *wrapPacketListener) Addr() net.Addr { return l.inner.Addr() }

type wrapPacketConn struct {
	inner     net.PacketConn
	keys      *wrapKeyStore
	key       []byte
	selected  int32
	authLog   int32
	obfsCfg   *ObfsConfig
	obfsWrite *ObfsState
}

func (c *wrapPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	// Extra space for RTP header (12) + AEAD tag (16) + padding.
	buf := make([]byte, len(p)+80)
	n, addr, err := c.inner.ReadFrom(buf)
	if err != nil {
		return 0, addr, err
	}
	raw := buf[:n]
	if relayShouldDrop(addr) {
		return 0, addr, errors.New("relay stale: dropped")
	}

	if atomic.LoadInt32(&c.selected) == 0 {
		key, m, uErr := c.keys.Unwrap(raw, p)
		if uErr != nil {
			if atomic.CompareAndSwapInt32(&c.authLog, 0, 1) {
				log.Printf("[WRAP] Отказ: RTP AEAD auth failed from %s (keys=%d)", addr.String(), c.keys.Count())
			}
			return 0, addr, uErr
		}
		c.key = key
		c.obfsCfg = NewObfsConfig()
		c.obfsWrite = NewObfsState()
		atomic.StoreInt32(&c.selected, 1)
		if atomic.CompareAndSwapInt32(&c.authLog, 0, 1) {
			log.Printf("[WRAP] OK: ключ выбран для %s (keys=%d)", addr.String(), c.keys.Count())
		}
		return m, addr, nil
	}

	m, uErr := obfsUnwrapPacket(c.key, raw, p)
	if uErr != nil {
		return 0, addr, fmt.Errorf("obfs unwrap: %w", uErr)
	}
	return m, addr, nil
}

func (c *wrapPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if atomic.LoadInt32(&c.selected) == 0 || len(c.key) != wrapKeyLen {
		return 0, errors.New("wrap: key not selected")
	}
	if c.obfsCfg == nil || c.obfsWrite == nil {
		c.obfsCfg = NewObfsConfig()
		c.obfsWrite = NewObfsState()
	}
	wrapped, wErr := obfsWrapPacket(c.key, p, c.obfsCfg, c.obfsWrite)
	if wErr != nil {
		return 0, fmt.Errorf("obfs wrap: %w", wErr)
	}
	if _, err := c.inner.WriteTo(wrapped, addr); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *wrapPacketConn) Close() error                       { return c.inner.Close() }
func (c *wrapPacketConn) LocalAddr() net.Addr                { return c.inner.LocalAddr() }
func (c *wrapPacketConn) SetDeadline(t time.Time) error      { return c.inner.SetDeadline(t) }
func (c *wrapPacketConn) SetReadDeadline(t time.Time) error  { return c.inner.SetReadDeadline(t) }
func (c *wrapPacketConn) SetWriteDeadline(t time.Time) error { return c.inner.SetWriteDeadline(t) }
