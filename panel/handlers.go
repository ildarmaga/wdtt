package panel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	"github.com/ildarmaga/wdtt/pkg/vkhash"
	"golang.org/x/crypto/bcrypt"
)

var safePathSegment = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func isSafePathSegment(s string) bool {
	return s != "" && s != ".." && safePathSegment.MatchString(s)
}

const maxRequestBodySize = 1 << 20 // 1 MiB

func readJSON(r *http.Request, v interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if len(bytes.TrimSpace(body)) == 0 {
		return fmt.Errorf("empty body")
	}
	if err := json.Unmarshal(body, v); err == nil {
		return nil
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return fmt.Errorf("invalid request body")
	}
	return decodeForm(form, v)
}

func decodeForm(form url.Values, v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("expected struct pointer")
	}
	rv = rv.Elem()
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		val := form.Get(name)
		if val == "" {
			continue
		}
		fv := rv.Field(i)
		if !fv.CanSet() {
			continue
		}
		switch fv.Kind() {
		case reflect.String:
			fv.SetString(val)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid %s", name)
			}
			fv.SetInt(n)
		case reflect.Bool:
			fv.SetBool(val == "true" || val == "1")
		}
	}
	return nil
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
		"users_count":      len(db.Passwords),
		"devices_count":    len(db.Devices),
		"server_ip":          a.serverIP(),
		"default_link_host":  a.defaultLinkHost(),
		"panel_tls":          panelTLSEnabled(a.cfg),
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

func (a *App) enrichUserSub(u map[string]interface{}, entry *PasswordEntry) {
	if u == nil || entry == nil {
		return
	}
	u["sub_id"] = entry.SubID
	u["sub_url"] = a.buildSubURL(entry.SubID)
}

func (a *App) handleUsersList(w http.ResponseWriter, r *http.Request) {
	// Подтянуть live vk_calls / снять finishing; ручные хеши не затираем (#23).
	reconcileUserSessionFields()
	db, err := loadPasswords()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	inbound, _ := loadWdttInbound()
	linkHost := a.resolveLinkHost(inbound)
	stats := loadServerStats()
	mainSub := ""
	if mainEntry, ok := db.Passwords[db.MainPassword]; ok && mainEntry != nil {
		mainSub = a.buildSubURL(mainEntry.SubID)
	}
	users := []map[string]interface{}{mainUserRow(db, stats, inbound, linkHost, a.cfg.SubTitle, mainSub)}
	if mainEntry, ok := db.Passwords[db.MainPassword]; ok {
		a.enrichUserSub(users[0], mainEntry)
	}
	for pass, entry := range db.Passwords {
		if pass == db.MainPassword {
			continue
		}
		used := trafficUsed(entry)
		dtlsPort, wgPort, clientPort := resolveUserPorts(entry, inbound)
		normalizeEntryDevices(entry)
		u := map[string]interface{}{
			"password":           maskPassword(pass),
			"password_key":       pass,
			"device_id":          deviceIDsDisplay(entry),
			"device_ids":         entry.DeviceIDs,
			"devices_bound":      len(entry.DeviceIDs),
			"max_devices":        entryMaxDevices(db, pass, entry),
			"comment":            entry.Comment,
			"expires_at":         entry.ExpiresAt,
			"expires":            passwordExpiry(entry),
			"up":                 formatBytes(entry.UpBytes),
			"down":               formatBytes(entry.DownBytes),
			"up_bytes":           entry.UpBytes,
			"down_bytes":         entry.DownBytes,
			"total_bytes":        entry.TotalBytes,
			"total_gb":           bytesToGB(entry.TotalBytes),
			"max_down_mbps":      entry.MaxDownMBps,
			"max_up_mbps":        entry.MaxUpMBps,
			"traffic_used":       used,
			"traffic_used_fmt":   formatBytes(used),
			"traffic_exceeded":   trafficExceeded(entry),
			"active":             !entry.IsDeactivated && !isPasswordExpired(entry),
			"online":             userOnlineFromStats(pass, deviceIDsDisplay(entry), false, stats),
			"last_seen_at":       entry.LastSeenAt,
			"ports":              entry.Ports,
			"dtls_port":          dtlsPort,
			"wg_port":            wgPort,
			"client_port":        clientPort,
			"link":               buildWdttLink(linkHost, pass, a.cfg.SubTitle, entry.VkHash, entry, inbound, a.buildSubURL(entry.SubID)),
			"vk_hash":            entry.VkHash,
		}
		a.enrichUserSub(u, entry)
		users = append(users, u)
	}
	sortUsersList(users)
	devices := []map[string]interface{}{}
	for id, dev := range db.Devices {
		devices = append(devices, map[string]interface{}{
			"device_id": id,
			"ip": dev.IP,
		})
	}
	jsonOK(w, map[string]interface{}{
		"users":         users,
		"devices":       devices,
		"inbound": map[string]interface{}{
			"tag":               inbound.Tag,
			"sub_title":         a.cfg.SubTitle,
			"remark":            inbound.Remark,
			"listen_host":       inbound.ListenHost,
			"server_host":       inbound.ServerHost,
			"default_link_host": a.defaultLinkHost(),
			"panel_tls":         panelTLSEnabled(a.cfg),
			"dtls_port":         inbound.DtlsPort,
			"wg_port":           inbound.WgPort,
			"client_port":       inbound.ClientPort,
			"dns":               inbound.DNS,
			"max_users":         inbound.MaxUsers,
		},
	})
}

type userAPIReq struct {
	Password    string    `json:"password"`
	DeviceID    string    `json:"device_id"`
	DeviceIDs   *[]string `json:"device_ids"`
	MaxDevices  int       `json:"max_devices"`
	Comment     string  `json:"comment"`
	ExpiresAt   int64   `json:"expires_at"`
	TotalGB     float64 `json:"total_gb"`
	MaxDownMBps float64 `json:"max_down_mbps"`
	MaxUpMBps   float64 `json:"max_up_mbps"`
	Active      *bool   `json:"active"`
	Ports       string  `json:"ports"`
	DtlsPort    int     `json:"dtls_port"`
	WgPort      int     `json:"wg_port"`
	ClientPort  int     `json:"client_port"`
	UseCustomPorts bool `json:"use_custom_ports"`
	VkHash      string  `json:"vk_hash"`
	Count       int     `json:"count"`
}

func passwordEntryFromReq(req userAPIReq) *PasswordEntry {
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	entry := &PasswordEntry{
		Comment:       strings.TrimSpace(req.Comment),
		ExpiresAt:     req.ExpiresAt,
		TotalBytes:    gbToBytes(req.TotalGB),
		MaxDownMBps:   req.MaxDownMBps,
		MaxUpMBps:     req.MaxUpMBps,
		IsDeactivated: !active,
		Ports:         portsFromReq(req),
		MaxDevices:    req.MaxDevices,
		VkHash:        vkhash.Normalize(req.VkHash),
	}
	if req.DeviceIDs != nil {
		// Поле передано явно (в т.ч. пустой список = отвязать все устройства).
		entry.DeviceIDs = append([]string(nil), (*req.DeviceIDs)...)
	} else if id := strings.TrimSpace(req.DeviceID); id != "" {
		entry.DeviceIDs = []string{id}
	}
	normalizeEntryDevices(entry)
	return entry
}

func portsFromReq(req userAPIReq) string {
	if req.Ports != "" {
		return strings.TrimSpace(req.Ports)
	}
	if !req.UseCustomPorts {
		return ""
	}
	if req.DtlsPort > 0 && req.WgPort > 0 && req.ClientPort > 0 {
		return strconv.Itoa(req.DtlsPort) + "," + strconv.Itoa(req.WgPort) + "," + strconv.Itoa(req.ClientPort)
	}
	return ""
}

func (a *App) handleUserAdd(w http.ResponseWriter, r *http.Request) {
	var req userAPIReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
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
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	manageDevices := req.DeviceIDs != nil || strings.TrimSpace(req.DeviceID) != ""
	if err := updateUser(req.OldPassword, strings.TrimSpace(req.Password), req.userAPIReq, manageDevices); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	jsonOK(w, nil)
}

func (a *App) handleUserResetTraffic(w http.ResponseWriter, r *http.Request) {
	var req struct{ Password string `json:"password"` }
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
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
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
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
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if !validPassword(req.Password) {
		jsonError(w, "пароль должен быть от 1 до 128 символов", 400)
		return
	}
	if err := updateWdttPassword(req.Password); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, nil)
}

func (a *App) handleXrayVersions(w http.ResponseWriter, r *http.Request) {
	tags, err := xrayVersionTagList()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]interface{}{
		"current":  xrayVersion(),
		"versions": tags,
	})
}

func (a *App) handleXrayInstall(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/xray/install/")
	if !isSafePathSegment(tag) {
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
	if !isSafePathSegment(name) {
		jsonError(w, "имя файла не указано", 400)
		return
	}
	updated, restartXray, err := updateGeofilesOp(name)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if restartXray {
		serviceRestart(xrayServiceUnit)
	}
	if len(updated) == 1 {
		jsonOK(w, map[string]string{"updated": updated[0]})
		return
	}
	jsonOK(w, map[string]interface{}{"updated": updated})
}

func (a *App) handleXrayConfig(w http.ResponseWriter, r *http.Request) {
	raw, err := loadXrayConfigRaw()
	if err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(raw))
}

func (a *App) handlePanelPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
		NewUsername string `json:"new_username"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, "неверный формат запроса", 400)
		return
	}
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
	level := r.URL.Query().Get("level")
	syslog := r.URL.Query().Get("syslog") == "true"
	lines := fetchServiceLogLines(n, svc, level, syslog)
	jsonOK(w, map[string]interface{}{"logs": lines})
}

func writeStatic(w http.ResponseWriter, data []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Write(data)
}

// ==================== API Tokens ====================

func (a *App) handleTokensList(w http.ResponseWriter, r *http.Request) {
	if !panelDBEnabled() {
		jsonError(w, "database unavailable", 500)
		return
	}
	tokens, err := paneldb.ListAPITokens(panelDB)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, tokens)
}

func (a *App) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	if !panelDBEnabled() {
		jsonError(w, "database unavailable", 500)
		return
	}
	var req struct {
		Name  string `json:"name"`
		Scope string `json:"scope"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if req.Scope != paneldb.APITokenScopeAdmin && req.Scope != paneldb.APITokenScopeReadonly {
		req.Scope = paneldb.APITokenScopeAdmin
	}
	tok, err := paneldb.CreateAPIToken(panelDB, strings.TrimSpace(req.Name), req.Scope)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, tok)
}

func (a *App) handleTokenDelete(w http.ResponseWriter, r *http.Request) {
	if !panelDBEnabled() {
		jsonError(w, "database unavailable", 500)
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if req.ID == 0 {
		jsonError(w, "id required", 400)
		return
	}
	if err := paneldb.DeleteAPIToken(panelDB, req.ID); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, nil)
}

func (a *App) handleTokenToggle(w http.ResponseWriter, r *http.Request) {
	if !panelDBEnabled() {
		jsonError(w, "database unavailable", 500)
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if req.ID == 0 {
		jsonError(w, "id required", 400)
		return
	}
	if err := paneldb.ToggleAPIToken(panelDB, req.ID); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, nil)
}
