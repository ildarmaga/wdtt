package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"golang.zx2c4.com/wireguard/device"
)

type panelUserUpdateReq struct {
	OldPassword string    `json:"old_password"`
	Password    string    `json:"password"`
	DeviceID    string    `json:"device_id"`
	DeviceIDs   *[]string `json:"device_ids"`
	MaxDevices  int       `json:"max_devices"`
	Comment     string    `json:"comment"`
	ExpiresAt   int64     `json:"expires_at"`
	TotalGB     float64   `json:"total_gb"`
	MaxDownMBps float64   `json:"max_down_mbps"`
	MaxUpMBps   float64   `json:"max_up_mbps"`
	Active      *bool     `json:"active"`
	Ports       string    `json:"ports"`
	DtlsPort    int       `json:"dtls_port"`
	WgPort      int       `json:"wg_port"`
	ClientPort  int       `json:"client_port"`
	UseCustomPorts bool   `json:"use_custom_ports"`
	VkHash         string    `json:"vk_hash"`
}

func readAdminJSON(r *http.Request, v interface{}) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return fmt.Errorf("empty body")
	}
	return json.Unmarshal(body, v)
}

func writeAdminJSON(w http.ResponseWriter, ok bool, msg string) {
	w.Header().Set("Content-Type", "application/json")
	if ok {
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}
	_, _ = w.Write([]byte(`{"ok":false,"msg":` + jsonString(msg) + `}`))
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func gbToBytesPanel(gb float64) int64 {
	if gb <= 0 {
		return 0
	}
	return int64(gb * 1024 * 1024 * 1024)
}

func portsFromPanelReq(req panelUserUpdateReq) string {
	if req.Ports != "" {
		return strings.TrimSpace(req.Ports)
	}
	if !req.UseCustomPorts {
		return ""
	}
	if req.DtlsPort > 0 && req.WgPort > 0 && req.ClientPort > 0 {
		return fmt.Sprintf("%d,%d,%d", req.DtlsPort, req.WgPort, req.ClientPort)
	}
	return ""
}

func applyPanelUserUpdateLocked(wgDev *device.Device, req panelUserUpdateReq, manageDevices bool) error {
	oldPassword := strings.TrimSpace(req.OldPassword)
	newPassword := strings.TrimSpace(req.Password)
	if oldPassword == "" {
		return fmt.Errorf("пароль не указан")
	}
	if newPassword == "" {
		newPassword = oldPassword
	}
	if len(newPassword) == 0 || len(newPassword) > 128 {
		return fmt.Errorf("пароль должен быть от 1 до 128 символов")
	}
	cur, ok := db.Passwords[oldPassword]
	if !ok || cur == nil {
		return fmt.Errorf("пользователь не найден")
	}
	if oldPassword == db.MainPassword {
		if newPassword != oldPassword {
			return fmt.Errorf("смену главного пароля выполняйте в поле «Пароль»")
		}
		if !manageDevices {
			return fmt.Errorf("для главного пароля доступно только управление устройствами")
		}
		entry := *cur
		if req.DeviceIDs != nil {
			entry.DeviceIDs = append([]string(nil), (*req.DeviceIDs)...)
			entry.DeviceID = ""
		} else if id := strings.TrimSpace(req.DeviceID); id != "" {
			entry.DeviceIDs = []string{id}
		} else {
			entry.DeviceIDs = nil
			entry.DeviceID = ""
		}
		normalizeEntryDevices(&entry)
		entry.MaxDevices = 0
		return finalizeDeviceChangeLocked(wgDev, oldPassword, cur, &entry)
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}
	entry := *cur
	entry.Comment = strings.TrimSpace(req.Comment)
	entry.ExpiresAt = req.ExpiresAt
	entry.TotalBytes = gbToBytesPanel(req.TotalGB)
	entry.MaxDownMBps = req.MaxDownMBps
	entry.MaxUpMBps = req.MaxUpMBps
	entry.IsDeactivated = !active
	if req.Ports != "" || req.UseCustomPorts {
		entry.Ports = portsFromPanelReq(req)
	}
	if strings.TrimSpace(req.VkHash) != "" {
		entry.VkHash = strings.TrimSpace(req.VkHash)
	}
	if req.MaxDevices > 0 {
		entry.MaxDevices = req.MaxDevices
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
		db.Passwords[newPassword] = &entry
	} else {
		entry.DownBytes = cur.DownBytes
		entry.UpBytes = cur.UpBytes
		entry.SubID = cur.SubID
		entry.LastSeenAt = cur.LastSeenAt
		db.Passwords[oldPassword] = &entry
	}
	if strings.TrimSpace(entry.SubID) == "" {
		subID, err := genSubID()
		if err != nil {
			return err
		}
		entry.SubID = subID
	}
	if isPasswordExpired(cur) && !isPasswordExpired(&entry) && !isTrafficExceeded(&entry) {
		entry.IsDeactivated = false
	}
	normalizeEntryDevices(&entry)
	if manageDevices {
		if req.DeviceIDs != nil {
			entry.DeviceIDs = append([]string(nil), (*req.DeviceIDs)...)
			entry.DeviceID = ""
		} else if id := strings.TrimSpace(req.DeviceID); id != "" {
			entry.DeviceIDs = []string{id}
		} else {
			entry.DeviceIDs = nil
			entry.DeviceID = ""
		}
		normalizeEntryDevices(&entry)
	} else if len(entry.DeviceIDs) == 0 && entry.DeviceID == "" {
		entry.DeviceIDs = append([]string(nil), cur.DeviceIDs...)
		entry.DeviceID = cur.DeviceID
	}
	if entry.MaxDevices <= 0 {
		entry.MaxDevices = cur.MaxDevices
	}
	savePass := oldPassword
	if newPassword != oldPassword {
		savePass = newPassword
	}
	db.Passwords[savePass] = &entry
	if len(entry.DeviceIDs) > entryMaxDevices(&entry) && savePass != db.MainPassword {
		return fmt.Errorf("привязано %d устройств — лимит %d, сначала отвяжите лишние", len(entry.DeviceIDs), entryMaxDevices(&entry))
	}
	if manageDevices {
		return finalizeDeviceChangeLocked(wgDev, savePass, cur, &entry)
	}
	if err := refreshWrapKeysFromDBLocked(); err != nil {
		return err
	}
	if newPassword != oldPassword {
		return persistUserRenameSQLiteLocked(oldPassword, newPassword, &entry)
	}
	return persistUserEntrySQLiteLocked(savePass, &entry)
}

func finalizeDeviceChangeLocked(wgDev *device.Device, pass string, cur, entry *PasswordEntry) error {
	removed := make([]string, 0)
	for _, devID := range cur.DeviceIDs {
		if entryHasDevice(entry, devID) {
			continue
		}
		removed = append(removed, devID)
		delete(db.Devices, devID)
		removeWGDeviceLocked(wgDev, devID)
	}
	db.Passwords[pass] = entry
	if err := persistUserDevicesSQLiteLocked(pass, entry, removed); err != nil {
		return err
	}
	return persistUserEntrySQLiteLocked(pass, entry)
}

func deletePanelUserLocked(wgDev *device.Device, pass string) error {
	pass = strings.TrimSpace(pass)
	if pass == "" {
		return fmt.Errorf("пароль не указан")
	}
	if pass == db.MainPassword {
		return fmt.Errorf("нельзя удалить главный пароль")
	}
	entry, ok := db.Passwords[pass]
	if !ok {
		return fmt.Errorf("пользователь не найден")
	}
	devIDs := allEntryDeviceIDs(entry)
	for _, devID := range devIDs {
		removeWGDeviceLocked(wgDev, devID)
		delete(db.Devices, devID)
	}
	delete(db.Passwords, pass)
	if err := persistDeleteUserSQLiteLocked(pass, devIDs); err != nil {
		return err
	}
	if err := refreshWrapKeysFromDBLocked(); err != nil {
		return err
	}
	return nil
}

func registerStoreAdminRoutes(mux *http.ServeMux, wgDev *device.Device) {
	mux.HandleFunc("/admin/users/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if !adminReloadAuthorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req panelUserUpdateReq
		if err := readAdminJSON(r, &req); err != nil {
			writeAdminJSON(w, false, err.Error())
			return
		}
		manageDevices := req.DeviceIDs != nil || strings.TrimSpace(req.DeviceID) != ""
		dbMutex.Lock()
		err := applyPanelUserUpdateLocked(wgDev, req, manageDevices)
		dbMutex.Unlock()
		if err != nil {
			writeAdminJSON(w, false, err.Error())
			return
		}
		log.Printf("[ADMIN] пользователь обновлён: %s", maskPassword(req.OldPassword))
		writeAdminJSON(w, true, "")
	})
	mux.HandleFunc("/admin/users/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if !adminReloadAuthorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Password string `json:"password"`
		}
		if err := readAdminJSON(r, &req); err != nil {
			writeAdminJSON(w, false, err.Error())
			return
		}
		dbMutex.Lock()
		err := deletePanelUserLocked(wgDev, req.Password)
		dbMutex.Unlock()
		if err != nil {
			writeAdminJSON(w, false, err.Error())
			return
		}
		log.Printf("[ADMIN] пользователь удалён: %s", maskPassword(req.Password))
		writeAdminJSON(w, true, "")
	})
}
