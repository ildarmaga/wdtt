package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type PasswordsDB struct {
	MainPassword string                    `json:"main_password"`
	Passwords    map[string]*PasswordEntry `json:"passwords"`
	Devices      map[string]*DeviceEntry   `json:"devices"`
}

type PasswordEntry struct {
	DeviceID      string  `json:"device_id"`
	ExpiresAt     int64   `json:"expires_at"`
	DownBytes     int64   `json:"down_bytes"`
	UpBytes       int64   `json:"up_bytes"`
	TotalBytes    int64   `json:"total_bytes,omitempty"`
	MaxDownMBps   float64 `json:"max_down_mbps,omitempty"`
	MaxUpMBps     float64 `json:"max_up_mbps,omitempty"`
	IsDeactivated bool    `json:"is_deactivated,omitempty"`
	Comment       string  `json:"comment,omitempty"`
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
	p := filepath.Join(wdttConfigDir, "passwords.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return &PasswordsDB{Passwords: map[string]*PasswordEntry{}, Devices: map[string]*DeviceEntry{}}, nil
	}
	var db PasswordsDB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, err
	}
	if db.Passwords == nil {
		db.Passwords = map[string]*PasswordEntry{}
	}
	if db.Devices == nil {
		db.Devices = map[string]*DeviceEntry{}
	}
	return &db, nil
}

func savePasswords(db *PasswordsDB) error {
	p := filepath.Join(wdttConfigDir, "passwords.json")
	data, _ := json.MarshalIndent(db, "", "  ")
	return os.WriteFile(p, data, 0600)
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
		if onlineDevice != "" {
			if deviceID != "" {
				for _, did := range deviceIDs {
					if did != "" && did == onlineDevice {
						return true
					}
				}
			}
			if isMain && onlineUser == "main" && deviceID == "" {
				return true
			}
		}
		if isMain && onlineUser == "main" {
			return true
		}
		if !isMain && onlineUser == maskPassword(pass) {
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

func mainUserRow(db *PasswordsDB, stats *ServerStats) map[string]interface{} {
	upBytes := int64(0)
	downBytes := int64(0)
	deviceIDs := []string{}
	if stats != nil {
		upBytes = gbStringToBytes(stats.UpGB)
		downBytes = gbStringToBytes(stats.DownGB)
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
	}
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
	return serviceRestart(wdttServiceUnit)
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
	if len(db.Passwords) >= 10 {
		return "", fmt.Errorf("максимум 10 паролей")
	}
	if _, exists := db.Passwords[password]; exists {
		return "", fmt.Errorf("пароль уже существует")
	}
	db.Passwords[password] = entry
	if err := savePasswords(db); err != nil {
		return "", err
	}
	serviceRestart(wdttServiceUnit)
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

func updateUser(oldPassword, newPassword string, entry *PasswordEntry) error {
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
		delete(db.Passwords, oldPassword)
		db.Passwords[newPassword] = entry
	} else {
		entry.DownBytes = cur.DownBytes
		entry.UpBytes = cur.UpBytes
		db.Passwords[oldPassword] = entry
	}
	if err := savePasswords(db); err != nil {
		return err
	}
	if newPassword != oldPassword ||
		cur.DeviceID != entry.DeviceID ||
		cur.ExpiresAt != entry.ExpiresAt ||
		cur.TotalBytes != entry.TotalBytes ||
		cur.MaxDownMBps != entry.MaxDownMBps ||
		cur.MaxUpMBps != entry.MaxUpMBps ||
		cur.IsDeactivated != entry.IsDeactivated {
		serviceRestart(wdttServiceUnit)
	}
	return nil
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
	if err := savePasswords(db); err != nil {
		return err
	}
	return serviceRestart(wdttServiceUnit)
}

func deleteUserPassword(pass string) error {
	db, err := loadPasswords()
	if err != nil {
		return err
	}
	delete(db.Passwords, pass)
	if err := savePasswords(db); err != nil {
		return err
	}
	serviceRestart(wdttServiceUnit)
	return nil
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
