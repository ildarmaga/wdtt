package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func readJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "obj": v})
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "msg": msg})
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	stats := loadServerStats()
	db, _ := loadPasswords()
	jsonOK(w, map[string]interface{}{
		"wdtt_active":      serviceActive(wdttServiceUnit),
		"xray_active":      serviceActive(xrayServiceUnit),
		"wdtt_iface":       getWdttIface(),
		"xray_version":     xrayVersion(),
		"xray_binary":      xrayBinary(),
		"stats":            stats,
		"main_password":    db.MainPassword,
		"users_count":      len(db.Passwords),
		"devices_count":    len(db.Devices),
		"server_ip":        a.serverIP(),
	})
}

func (a *App) serverIP() string {
	out, _ := runCmd("curl", "-4", "-s", "--max-time", "3", "ifconfig.me")
	if out != "" {
		return strings.TrimSpace(out)
	}
	out, _ = runCmd("hostname", "-I")
	return strings.Fields(out)[0]
}

func (a *App) handleUsersList(w http.ResponseWriter, r *http.Request) {
	db, err := loadPasswords()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	stats := loadServerStats()
	users := []map[string]interface{}{mainUserRow(db, stats)}
	for pass, entry := range db.Passwords {
		used := trafficUsed(entry)
		u := map[string]interface{}{
			"password":           pass,
			"device_id":          entry.DeviceID,
			"comment":            entry.Comment,
			"expires_at":         entry.ExpiresAt,
			"expires":            passwordExpiry(entry),
			"up":                 formatBytes(entry.UpBytes),
			"down":               formatBytes(entry.DownBytes),
			"up_bytes":           entry.UpBytes,
			"down_bytes":         entry.DownBytes,
			"total_bytes":        entry.TotalBytes,
			"total_gb":           bytesToGB(entry.TotalBytes),
			"traffic_used":       used,
			"traffic_used_fmt":   formatBytes(used),
			"traffic_exceeded":   trafficExceeded(entry),
			"active":             !entry.IsDeactivated,
			"online":             userOnlineFromStats(pass, entry.DeviceID, false, stats),
		}
		users = append(users, u)
	}
	devices := []map[string]interface{}{}
	for id, dev := range db.Devices {
		devices = append(devices, map[string]interface{}{
			"device_id": id,
			"ip": dev.IP,
		})
	}
	jsonOK(w, map[string]interface{}{
		"main_password": db.MainPassword,
		"users": users,
		"devices": devices,
	})
}

type userAPIReq struct {
	Password  string  `json:"password"`
	DeviceID  string  `json:"device_id"`
	Comment   string  `json:"comment"`
	ExpiresAt int64   `json:"expires_at"`
	TotalGB   float64 `json:"total_gb"`
	Active    *bool   `json:"active"`
	Count     int    `json:"count"`
}

func passwordEntryFromReq(req userAPIReq) *PasswordEntry {
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	return &PasswordEntry{
		DeviceID:      strings.TrimSpace(req.DeviceID),
		Comment:       strings.TrimSpace(req.Comment),
		ExpiresAt:     req.ExpiresAt,
		TotalBytes:    gbToBytes(req.TotalGB),
		IsDeactivated: !active,
	}
}

func (a *App) handleUserAdd(w http.ResponseWriter, r *http.Request) {
	var req userAPIReq
	readJSON(r, &req)
	if req.Password == "" && req.DeviceID == "" && req.Comment == "" && req.ExpiresAt == 0 && req.TotalGB == 0 && req.Active == nil {
		if req.Count < 1 {
			req.Count = 1
		}
		created, err := addUserPasswords(req.Count)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		jsonOK(w, map[string]interface{}{"passwords": created})
		return
	}
	entry := passwordEntryFromReq(req)
	pw, err := createUser(strings.TrimSpace(req.Password), entry)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	jsonOK(w, map[string]interface{}{"password": pw})
}

func (a *App) handleUserUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		userAPIReq
	}
	readJSON(r, &req)
	entry := passwordEntryFromReq(req.userAPIReq)
	if err := updateUser(req.OldPassword, strings.TrimSpace(req.Password), entry); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	jsonOK(w, nil)
}

func (a *App) handleUserResetTraffic(w http.ResponseWriter, r *http.Request) {
	var req struct{ Password string `json:"password"` }
	readJSON(r, &req)
	if req.Password == "" {
		jsonError(w, "пароль не указан", 400)
		return
	}
	if err := resetUserTraffic(req.Password); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	jsonOK(w, nil)
}

func (a *App) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	var req struct{ Password string `json:"password"` }
	readJSON(r, &req)
	if req.Password == "" {
		jsonError(w, "пароль не указан", 400)
		return
	}
	if err := deleteUserPassword(req.Password); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, nil)
}

func (a *App) handleMainPassword(w http.ResponseWriter, r *http.Request) {
	var req struct{ Password string `json:"password"` }
	readJSON(r, &req)
	if len(req.Password) < 1 {
		jsonError(w, "пустой пароль", 400)
		return
	}
	if err := updateWdttPassword(req.Password); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, nil)
}

func (a *App) handleService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", 405)
		return
	}
	// /panel/api/service/wdtt/restart
	path := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/service/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		jsonError(w, "неверный запрос", 400)
		return
	}
	svcMap := map[string]string{"wdtt": wdttServiceUnit, "xray": xrayServiceUnit}
	svc, ok := svcMap[parts[0]]
	if !ok {
		jsonError(w, "неизвестный сервис", 400)
		return
	}
	var err error
	switch parts[1] {
	case "restart":
		err = serviceRestart(svc)
	case "stop":
		err = serviceStop(svc)
	case "start":
		err = serviceStart(svc)
	default:
		jsonError(w, "неизвестное действие", 400)
		return
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"service": svc, "action": parts[1]})
}

func (a *App) handleXrayVersions(w http.ResponseWriter, r *http.Request) {
	releases, err := fetchXrayReleases()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	tags := make([]string, 0, len(releases))
	for _, rel := range releases {
		tags = append(tags, rel.TagName)
	}
	jsonOK(w, map[string]interface{}{
		"current": xrayVersion(),
		"versions": tags,
	})
}

func (a *App) handleXrayInstall(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/xray/install/")
	if tag == "" {
		jsonError(w, "версия не указана", 400)
		return
	}
	if err := installXrayVersion(tag); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"version": tag})
}

func (a *App) handleXrayGeofiles(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/xray/geofiles/")
	if name == "" || name == "all" {
		updated, err := updateAllGeofiles()
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		jsonOK(w, map[string]interface{}{"updated": updated})
		return
	}
	if err := updateGeofile(name); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	serviceRestart(xrayServiceUnit)
	jsonOK(w, map[string]string{"updated": name})
}

func (a *App) handleXrayConfig(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(xrayConfigPath)
	if err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (a *App) handlePanelPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
		NewUsername string `json:"new_username"`
	}
	readJSON(r, &req)
	if bcrypt.CompareHashAndPassword([]byte(a.cfg.PasswordHash), []byte(req.OldPassword)) != nil {
		jsonError(w, "неверный текущий пароль", 401)
		return
	}
	if req.NewPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		a.cfg.PasswordHash = string(hash)
	}
	if req.NewUsername != "" {
		a.cfg.Username = req.NewUsername
	}
	savePanelConfig(a.cfg)
	jsonOK(w, nil)
}

func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	n := 50
	if v := r.URL.Query().Get("n"); v != "" {
		n, _ = strconv.Atoi(v)
	}
	svc := r.URL.Query().Get("service")
	if svc == "" {
		svc = "wdtt"
	}
	out, _ := runCmd("journalctl", "-u", svc+".service", "-n", strconv.Itoa(n), "--no-pager", "-o", "cat")
	jsonOK(w, map[string]string{"logs": out})
}

func writeStatic(w http.ResponseWriter, data []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Write(data)
}
