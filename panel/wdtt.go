package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type PasswordsDB struct {
	MainPassword string                    `json:"main_password"`
	AdminID      string                    `json:"admin_id,omitempty"`
	BotToken     string                    `json:"bot_token,omitempty"`
	Passwords    map[string]*PasswordEntry `json:"passwords"`
	Devices      map[string]*DeviceEntry   `json:"devices"`
}

type PasswordEntry struct {
	DeviceID      string   `json:"device_id,omitempty"`
	DeviceIDs     []string `json:"device_ids,omitempty"`
	MaxDevices    int      `json:"max_devices,omitempty"`
	ExpiresAt     int64   `json:"expires_at"`
	DownBytes     int64   `json:"down_bytes"`
	UpBytes       int64   `json:"up_bytes"`
	TotalBytes    int64   `json:"total_bytes,omitempty"`
	MaxDownMBps   float64 `json:"max_down_mbps,omitempty"`
	MaxUpMBps     float64 `json:"max_up_mbps,omitempty"`
	IsDeactivated bool    `json:"is_deactivated,omitempty"`
	Comment       string  `json:"comment,omitempty"`
	Ports         string  `json:"ports,omitempty"`
	VkHash        string  `json:"vk_hash,omitempty"`
	SubID         string  `json:"sub_id,omitempty"`
	LastSeenAt    int64   `json:"last_seen_at,omitempty"`
}

const oneGB = 1024 * 1024 * 1024

func gbToBytes(gb float64) int64 {
	if gb <= 0 {
		return 0
	}
	return int64(gb * float64(oneGB))
}

func bytesToGB(b int64) float64 {
	if b <= 0 {
		return 0
	}
	return float64(b) / float64(oneGB)
}

func trafficUsed(entry *PasswordEntry) int64 {
	if entry == nil {
		return 0
	}
	return entry.UpBytes + entry.DownBytes
}

func trafficExceeded(entry *PasswordEntry) bool {
	if entry == nil || entry.TotalBytes <= 0 {
		return false
	}
	return trafficUsed(entry) >= entry.TotalBytes
}

func isPasswordExpired(entry *PasswordEntry) bool {
	if entry == nil || entry.ExpiresAt == 0 {
		return false
	}
	return time.Now().Unix() > entry.ExpiresAt
}

func countActivePasswords(db *PasswordsDB) int {
	n := 0
	for _, e := range db.Passwords {
		if e != nil && !isPasswordExpired(e) {
			n++
		}
	}
	return n
}

type DeviceEntry struct {
	DeviceID string `json:"device_id"`
	IP       string `json:"ip"`
	PrivKey  string `json:"priv_key"`
	PubKey   string `json:"pub_key"`
}

type ServerStats struct {
	ActiveUsers int                      `json:"active_users"`
	Sessions    int                      `json:"sessions"`
	Total       int64                    `json:"total"`
	NAT         string                   `json:"nat"`
	Uptime      string                   `json:"uptime"`
	DownGB      string                   `json:"down_gb"`
	UpGB        string                   `json:"up_gb"`
	Online      []map[string]interface{} `json:"online"`
	Timestamp   int64                    `json:"timestamp"`
}

const passChars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"

func loadPasswords() (*PasswordsDB, error) {
	if !panelDBEnabled() {
		return &PasswordsDB{
			Passwords: map[string]*PasswordEntry{},
			Devices:   map[string]*DeviceEntry{},
		}, nil
	}
	db, err := loadPasswordsNorm()
	if err == nil {
		return db, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return &PasswordsDB{
			Passwords: map[string]*PasswordEntry{},
			Devices:   map[string]*DeviceEntry{},
		}, nil
	}
	return nil, err
}

func dedupePasswordDeviceBindings(db *PasswordsDB) {
	type pick struct {
		pass   string
		isMain bool
	}
	chosen := map[string]pick{}
	for pass, entry := range db.Passwords {
		if entry == nil {
			continue
		}
		normalizeEntryDevices(entry)
		isMain := pass == db.MainPassword
		for _, did := range allEntryDeviceIDsPanel(entry) {
			if cur, ok := chosen[did]; !ok {
				chosen[did] = pick{pass: pass, isMain: isMain}
			} else if cur.isMain && !isMain {
				chosen[did] = pick{pass: pass, isMain: isMain}
			}
		}
	}
	for pass, entry := range db.Passwords {
		if entry == nil {
			continue
		}
		keep := make([]string, 0, len(entry.DeviceIDs))
		for _, did := range allEntryDeviceIDsPanel(entry) {
			if c, ok := chosen[did]; ok && c.pass == pass {
				keep = append(keep, did)
			}
		}
		entry.DeviceIDs = keep
		if len(keep) > 0 {
			entry.DeviceID = keep[0]
		} else {
			entry.DeviceID = ""
		}
	}
}

func savePasswords(db *PasswordsDB) error {
	if !panelDBEnabled() {
		return fmt.Errorf("panel database not available")
	}
	return savePasswordsNorm(db)
}

func maskPassword(pass string) string {
	if len(pass) <= 3 {
		return pass
	}
	return pass[:3] + "****"
}

func userOnlineFromStats(pass, deviceID string, isMain bool, stats *ServerStats) bool {
	if stats == nil || len(stats.Online) == 0 {
		return false
	}
	deviceIDs := strings.Split(deviceID, ", ")
	for _, o := range stats.Online {
		onlineDevice, _ := o["device_id"].(string)
		onlineUser, _ := o["user"].(string)
		onlinePass, _ := o["password"].(string)

		if onlinePass != "" && onlinePass == pass {
			return true
		}
		if onlineDevice != "" && deviceID != "" {
			for _, did := range deviceIDs {
				if did != "" && did == onlineDevice {
					return true
				}
			}
		}
		if isMain && onlineUser == "main" {
			if deviceID == "" {
				return true
			}
			for _, did := range deviceIDs {
				if did != "" && did == onlineDevice {
					return true
				}
			}
			continue
		}
		if !isMain && onlinePass == "" && onlineUser == maskPassword(pass) {
			return true
		}
	}
	return false
}

func gbStringToBytes(s string) int64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 0
	}
	return int64(v * float64(oneGB))
}

func passwordsTrafficTotals(db *PasswordsDB) (up, down int64) {
	if db == nil {
		return 0, 0
	}
	for _, e := range db.Passwords {
		if e == nil {
			continue
		}
		up += e.UpBytes
		down += e.DownBytes
	}
	return up, down
}

func mainUserRow(db *PasswordsDB, stats *ServerStats, inbound WdttInboundConfig, serverIP, vpnTitle string) map[string]interface{} {
	upBytes := int64(0)
	downBytes := int64(0)
	deviceIDs := []string{}
	if entry, ok := db.Passwords[db.MainPassword]; ok && entry != nil {
		upBytes = entry.UpBytes
		downBytes = entry.DownBytes
	}
	if stats != nil {
		for _, o := range stats.Online {
			user, _ := o["user"].(string)
			if user != "main" {
				continue
			}
			if did, _ := o["device_id"].(string); did != "" {
				deviceIDs = append(deviceIDs, did)
			}
		}
	}
	used := upBytes + downBytes
	dtlsPort, wgPort, clientPort := resolveUserPorts(nil, inbound)
	return map[string]interface{}{
		"password":         db.MainPassword,
		"is_main":          true,
		"device_id":        strings.Join(deviceIDs, ", "),
		"comment":          "Владелец",
		"expires_at":       0,
		"expires":          "бессрочно",
		"up":               formatBytes(upBytes),
		"down":             formatBytes(downBytes),
		"up_bytes":         upBytes,
		"down_bytes":       downBytes,
		"total_bytes":      0,
		"total_gb":         0,
		"traffic_used":     used,
		"traffic_used_fmt": formatBytes(used),
		"traffic_exceeded": false,
		"active":           true,
		"online":           userOnlineFromStats(db.MainPassword, strings.Join(deviceIDs, ", "), true, stats),
		"ports":            "",
		"dtls_port":        dtlsPort,
		"wg_port":          wgPort,
		"client_port":      clientPort,
		"link":             buildWdttLink(serverIP, db.MainPassword, vpnTitle, "", &PasswordEntry{Comment: "Владелец"}, inbound, ""),
	}
}

func sortUsersList(users []map[string]interface{}) {
	sort.Slice(users, func(i, j int) bool {
		mi, _ := users[i]["is_main"].(bool)
		mj, _ := users[j]["is_main"].(bool)
		if mi != mj {
			return mi
		}
		ci := strings.ToLower(strings.TrimSpace(fmt.Sprint(users[i]["comment"])))
		cj := strings.ToLower(strings.TrimSpace(fmt.Sprint(users[j]["comment"])))
		if ci != cj {
			return ci < cj
		}
		return fmt.Sprint(users[i]["password"]) < fmt.Sprint(users[j]["password"])
	})
}

func loadServerStats() *ServerStats {
	data, err := os.ReadFile(filepath.Join(wdttConfigDir, "server.log"))
	if err != nil {
		return nil
	}
	var s ServerStats
	if json.Unmarshal(data, &s) != nil {
		return nil
	}
	return &s
}

func getWdttIface() string {
	out, _ := runCmd("ip", "-br", "addr", "show", "wdtt0")
	if out == "" {
		return ""
	}
	parts := strings.Fields(out)
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

func updateWdttPassword(pass string) error {
	db, err := loadPasswords()
	if err != nil {
		return err
	}
	db.MainPassword = pass
	if err := savePasswords(db); err != nil {
		return err
	}
	unit := "/etc/systemd/system/" + wdttServiceUnit
	data, err := os.ReadFile(unit)
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(line, "ExecStart=") && strings.Contains(line, "-password") {
				idx := strings.Index(line, "-password")
				if idx >= 0 {
					before := line[:idx+len("-password")+1]
					lines[i] = before + pass
				}
			}
		}
		os.WriteFile(unit, []byte(strings.Join(lines, "\n")), 0644)
		runCmd("systemctl", "daemon-reload")
	}
	return restartWdttWithDeps()
}

func genPassword() string {
	b := make([]byte, 16)
	rand.Read(b)
	out := make([]byte, 16)
	for i := range out {
		out[i] = passChars[int(b[i])%len(passChars)]
	}
	return string(out)
}

func validPassword(pw string) bool {
	return len(pw) >= 1 && len(pw) <= 128
}

func createUser(password string, entry *PasswordEntry) (string, error) {
	if entry == nil {
		entry = &PasswordEntry{}
	}
	normalizeEntryDevices(entry)
	if password == "" {
		password = genPassword()
	}
	if !validPassword(password) {
		return "", fmt.Errorf("пароль должен быть от 1 до 128 символов")
	}
	db, err := loadPasswords()
	if err != nil {
		return "", err
	}
	maxUsers := inboundMaxUsers()
	if countActivePasswords(db) >= maxUsers {
		return "", fmt.Errorf("максимум %d активных паролей (истёкшие не считаются)", maxUsers)
	}
	if _, exists := db.Passwords[password]; exists {
		return "", fmt.Errorf("пароль уже существует")
	}
	if strings.TrimSpace(entry.SubID) == "" {
		subID, err := genSubID()
		if err != nil {
			return "", err
		}
		entry.SubID = subID
	}
	db.Passwords[password] = entry
	if err := savePasswords(db); err != nil {
		return "", err
	}
	applyWdttConfigChange()
	return password, nil
}

func addUserPasswords(count int) ([]string, error) {
	created := make([]string, 0, count)
	for i := 0; i < count; i++ {
		pw, err := createUser("", &PasswordEntry{ExpiresAt: 0})
		if err != nil {
			return created, err
		}
		created = append(created, pw)
	}
	return created, nil
}

func updateUser(oldPassword, newPassword string, entry *PasswordEntry, manageDevices bool) error {
	if oldPassword == "" {
		return fmt.Errorf("пароль не указан")
	}
	if entry == nil {
		entry = &PasswordEntry{}
	}
	if newPassword == "" {
		newPassword = oldPassword
	}
	if !validPassword(newPassword) {
		return fmt.Errorf("пароль должен быть от 1 до 128 символов")
	}
	db, err := loadPasswords()
	if err != nil {
		return err
	}
	cur, ok := db.Passwords[oldPassword]
	if !ok {
		return fmt.Errorf("пользователь не найден")
	}
	if newPassword != oldPassword {
		if _, exists := db.Passwords[newPassword]; exists {
			return fmt.Errorf("пароль уже существует")
		}
		entry.DownBytes = cur.DownBytes
		entry.UpBytes = cur.UpBytes
		entry.SubID = cur.SubID
		entry.LastSeenAt = cur.LastSeenAt
		delete(db.Passwords, oldPassword)
		db.Passwords[newPassword] = entry
	} else {
		entry.DownBytes = cur.DownBytes
		entry.UpBytes = cur.UpBytes
		entry.SubID = cur.SubID
		entry.LastSeenAt = cur.LastSeenAt
		db.Passwords[oldPassword] = entry
	}
	if strings.TrimSpace(entry.SubID) == "" {
		subID, err := genSubID()
		if err != nil {
			return err
		}
		entry.SubID = subID
	}
	wasExpired := isPasswordExpired(cur)
	nowValid := !isPasswordExpired(entry)
	if wasExpired && nowValid && !trafficExceeded(entry) {
		entry.IsDeactivated = false
	}
	normalizeEntryDevices(cur)
	normalizeEntryDevices(entry)
	// Сохраняем текущие устройства только если запрос их не редактировал.
	// Если устройства переданы явно (в т.ч. пустой список) — применяем как есть,
	// что позволяет отвязать все привязанные устройства.
	if !manageDevices && len(entry.DeviceIDs) == 0 && entry.DeviceID == "" {
		entry.DeviceIDs = append([]string(nil), cur.DeviceIDs...)
		entry.DeviceID = cur.DeviceID
	}
	if entry.MaxDevices <= 0 {
		entry.MaxDevices = cur.MaxDevices
	}
	if len(entry.DeviceIDs) > entryMaxDevices(entry) {
		return fmt.Errorf("привязано %d устройств — лимит %d, сначала отвяжите лишние", len(entry.DeviceIDs), entryMaxDevices(entry))
	}
	for _, devID := range cur.DeviceIDs {
		if !entryHasDevice(entry, devID) {
			delete(db.Devices, devID)
		}
	}
	if err := savePasswords(db); err != nil {
		return err
	}
	return applyWdttConfigChange()
}

func resetUserTraffic(pass string) error {
	db, err := loadPasswords()
	if err != nil {
		return err
	}
	entry, ok := db.Passwords[pass]
	if !ok {
		return fmt.Errorf("пользователь не найден")
	}
	entry.UpBytes = 0
	entry.DownBytes = 0
	// Сброс квоты: сервер ставит IsDeactivated при исчерпании GB — снимаем блокировку
	if !trafficExceeded(entry) {
		entry.IsDeactivated = false
	}
	if err := savePasswords(db); err != nil {
		return err
	}
	return applyWdttConfigChange()
}

func deleteUserPassword(pass string) error {
	db, err := loadPasswords()
	if err != nil {
		return err
	}
	if entry, ok := db.Passwords[pass]; ok {
		for _, devID := range allEntryDeviceIDsPanel(entry) {
			delete(db.Devices, devID)
		}
	}
	delete(db.Passwords, pass)
	if err := savePasswords(db); err != nil {
		return err
	}
	return applyWdttConfigChange()
}

func formatBytes(b int64) string {
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	if b < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(b)/1024/1024)
	}
	return fmt.Sprintf("%.2f GB", float64(b)/1024/1024/1024)
}

func passwordExpiry(e *PasswordEntry) string {
	if e == nil || e.ExpiresAt == 0 {
		return "бессрочно"
	}
	t := time.Unix(e.ExpiresAt, 0)
	if time.Now().After(t) {
		return "истёк"
	}
	days := int(math.Ceil(time.Until(t).Hours() / 24))
	return fmt.Sprintf("%d дн.", days)
}
