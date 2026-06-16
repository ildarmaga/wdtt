package panel

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type panelSettingsReq struct {
	WebListen     string `json:"webListen"`
	WebDomain     string `json:"webDomain"`
	WebPort       int    `json:"webPort"`
	WebBasePath   string `json:"webBasePath"`
	SessionMaxAge int    `json:"sessionMaxAge"`
	PageSize      int    `json:"pageSize"`
	RemarkModel   string `json:"remarkModel"`
	WebCertFile   string `json:"webCertFile"`
	WebKeyFile    string `json:"webKeyFile"`
	BlockPing     bool   `json:"blockPing"`
	SubEnable     bool   `json:"subEnable"`
	SubListen     string `json:"subListen"`
	SubPort       int    `json:"subPort"`
	SubPath       string `json:"subPath"`
	SubDomain     string `json:"subDomain"`
	SubCertFile   string `json:"subCertFile"`
	SubKeyFile    string `json:"subKeyFile"`
	SubEncrypt    bool   `json:"subEncrypt"`
	SubTitle      string `json:"subTitle"`
	SubSupportURL string `json:"subSupportUrl"`
	SubProfileURL string `json:"subProfileUrl"`
	SubAnnounce   string `json:"subAnnounce"`
	SubURI        string `json:"subURI"`
	SubShowInfo   bool   `json:"subShowInfo"`
	DashboardPollSec   int    `json:"dashboardPollSec"`
	UsersPollSec       int    `json:"usersPollSec"`
	ConnectionsPollSec int    `json:"connectionsPollSec"`
}

type panelUserReq struct {
	OldUsername string `json:"oldUsername"`
	OldPassword string `json:"oldPassword"`
	NewUsername string `json:"newUsername"`
	NewPassword string `json:"newPassword"`
}

func (a *App) handleSettingAll(w http.ResponseWriter, r *http.Request) {
	changed := sanitizePanelCertPaths(a.cfg)
	if syncSubCertFromPanel(a.cfg) {
		changed = true
	}
	if changed {
		_ = savePanelConfig(a.cfg)
		go a.restartSubscriptionServer()
	}
	jsonOK(w, panelSettingsMap(a.cfg))
}

func parsePanelSettingsReq(r *http.Request) (panelSettingsReq, error) {
	var req panelSettingsReq
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return req, err
	}
	defer r.Body.Close()
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		return req, json.Unmarshal(body, &req)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return req, err
	}
	req.WebListen = values.Get("webListen")
	req.WebDomain = values.Get("webDomain")
	req.WebBasePath = values.Get("webBasePath")
	req.RemarkModel = values.Get("remarkModel")
	req.WebCertFile = values.Get("webCertFile")
	req.WebKeyFile = values.Get("webKeyFile")
	if p := values.Get("webPort"); p != "" {
		req.WebPort, _ = strconv.Atoi(p)
	}
	if p := values.Get("sessionMaxAge"); p != "" {
		req.SessionMaxAge, _ = strconv.Atoi(p)
	}
	if p := values.Get("pageSize"); p != "" {
		req.PageSize, _ = strconv.Atoi(p)
	}
	if p := values.Get("dashboardPollSec"); p != "" {
		req.DashboardPollSec, _ = strconv.Atoi(p)
	}
	if p := values.Get("usersPollSec"); p != "" {
		req.UsersPollSec, _ = strconv.Atoi(p)
	}
	if p := values.Get("connectionsPollSec"); p != "" {
		req.ConnectionsPollSec, _ = strconv.Atoi(p)
	}
	if p := values.Get("blockPing"); p != "" {
		req.BlockPing = p == "true" || p == "1"
	}
	req.SubEnable = values.Get("subEnable") == "true" || values.Get("subEnable") == "1"
	req.SubListen = values.Get("subListen")
	req.SubPath = values.Get("subPath")
	req.SubDomain = values.Get("subDomain")
	req.SubCertFile = values.Get("subCertFile")
	req.SubKeyFile = values.Get("subKeyFile")
	req.SubTitle = values.Get("subTitle")
	req.SubSupportURL = values.Get("subSupportUrl")
	req.SubProfileURL = values.Get("subProfileUrl")
	req.SubAnnounce = values.Get("subAnnounce")
	req.SubURI = values.Get("subURI")
	if p := values.Get("subPort"); p != "" {
		req.SubPort, _ = strconv.Atoi(p)
	}
	req.SubEncrypt = values.Get("subEncrypt") == "true" || values.Get("subEncrypt") == "1"
	req.SubShowInfo = values.Get("subShowInfo") == "true" || values.Get("subShowInfo") == "1"
	return req, nil
}

func parsePanelUserReq(r *http.Request) (panelUserReq, error) {
	var req panelUserReq
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return req, err
	}
	defer r.Body.Close()
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		return req, json.Unmarshal(body, &req)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return req, err
	}
	req.OldUsername = values.Get("oldUsername")
	req.OldPassword = values.Get("oldPassword")
	req.NewUsername = values.Get("newUsername")
	req.NewPassword = values.Get("newPassword")
	return req, nil
}

func (a *App) handleSettingUpdate(w http.ResponseWriter, r *http.Request) {
	req, err := parsePanelSettingsReq(r)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if req.WebPort < 1 || req.WebPort > 65535 {
		jsonError(w, "неверный порт панели", 400)
		return
	}
	if req.WebBasePath != "" {
		if !strings.HasPrefix(req.WebBasePath, "/") {
			req.WebBasePath = "/" + req.WebBasePath
		}
		if !strings.HasSuffix(req.WebBasePath, "/") {
			req.WebBasePath += "/"
		}
	}
	if req.SessionMaxAge < 60 {
		req.SessionMaxAge = 60
	}
	if req.PageSize < 0 {
		req.PageSize = 0
	}

	a.cfg.WebListen = strings.TrimSpace(req.WebListen)
	a.cfg.WebDomain = strings.TrimSpace(req.WebDomain)
	if a.cfg.WebDomain == "" && req.WebCertFile != "" {
		if d := domainFromCertPath(req.WebCertFile); d != "" {
			a.cfg.WebDomain = d
		}
	}
	a.cfg.Port = req.WebPort
	if req.WebBasePath != "" {
		a.cfg.WebBasePath = req.WebBasePath
	}
	a.cfg.SessionMaxAge = req.SessionMaxAge
	a.cfg.PageSize = req.PageSize
	if req.RemarkModel != "" {
		a.cfg.RemarkModel = req.RemarkModel
	}
	a.cfg.WebCertFile = strings.TrimSpace(req.WebCertFile)
	a.cfg.WebKeyFile = strings.TrimSpace(req.WebKeyFile)
	a.cfg.BlockPing = req.BlockPing
	a.cfg.SubEnable = req.SubEnable
	a.cfg.SubListen = strings.TrimSpace(req.SubListen)
	a.cfg.SubPort = req.SubPort
	a.cfg.SubPath = normalizeSubPath(req.SubPath)
	a.cfg.SubDomain = strings.TrimSpace(req.SubDomain)
	a.cfg.SubCertFile = strings.TrimSpace(req.SubCertFile)
	a.cfg.SubKeyFile = strings.TrimSpace(req.SubKeyFile)
	a.cfg.SubEncrypt = req.SubEncrypt
	a.cfg.SubTitle = strings.TrimSpace(req.SubTitle)
	a.cfg.SubSupportURL = strings.TrimSpace(req.SubSupportURL)
	a.cfg.SubProfileURL = strings.TrimSpace(req.SubProfileURL)
	a.cfg.SubAnnounce = strings.TrimSpace(req.SubAnnounce)
	a.cfg.SubURI = strings.TrimSpace(req.SubURI)
	a.cfg.SubShowInfo = req.SubShowInfo
	a.cfg.DashboardPollSec = clampDashboardPollSec(req.DashboardPollSec)
	a.cfg.UsersPollSec = clampPagePollSec(req.UsersPollSec)
	a.cfg.ConnectionsPollSec = clampPagePollSec(req.ConnectionsPollSec)
	normalizePanelConfig(a.cfg)
	syncSubCertFromPanel(a.cfg)
	syncPanelPollDefaults(a.cfg)
	sanitizePanelCertPaths(a.cfg)

	if a.cfg.SubPort < 1 || a.cfg.SubPort > 65535 {
		jsonError(w, "неверный порт подписки", 400)
		return
	}
	if err := validatePanelTLS(a.cfg.SubCertFile, a.cfg.SubKeyFile); err != nil {
		jsonError(w, "подписка TLS: "+err.Error(), 400)
		return
	}

	if err := validatePanelTLS(a.cfg.WebCertFile, a.cfg.WebKeyFile); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	if ufwInstalled() {
		current := ufwPingBlocked()
		if req.BlockPing != current {
			if err := setUFWBlockPing(req.BlockPing); err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
		}
	} else if req.BlockPing {
		jsonError(w, "ufw не установлен — блокировка ping недоступна", 400)
		return
	}

	if err := savePanelConfig(a.cfg); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	go refreshTunnelLocalServiceRules()
	a.restartSubscriptionServer()
	jsonOK(w, panelSettingsMap(a.cfg))
}

func (a *App) handleSettingUpdateUser(w http.ResponseWriter, r *http.Request) {
	req, err := parsePanelUserReq(r)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	oldUser := strings.TrimSpace(req.OldUsername)
	if oldUser == "" {
		oldUser = a.cfg.Username
	}
	if oldUser != a.cfg.Username {
		jsonError(w, i18nWeb("pages.settings.toasts.originalUserPassIncorrect"), 400)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(a.cfg.PasswordHash), []byte(req.OldPassword)) != nil {
		jsonError(w, i18nWeb("pages.settings.toasts.originalUserPassIncorrect"), 401)
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
	if strings.TrimSpace(req.NewUsername) != "" {
		a.cfg.Username = strings.TrimSpace(req.NewUsername)
	}
	if err := savePanelConfig(a.cfg); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, nil)
}

type panelSSHPortReq struct {
	Port int `json:"port"`
}

func (a *App) handleSettingSSHPort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req panelSSHPortReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	result, err := setSSHPort(req.Port)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	jsonOK(w, result)
}

func (a *App) handleSettingRestartPanel(w http.ResponseWriter, r *http.Request) {
	// Ответ до restart: иначе systemctl убивает процесс и клиент видит 503.
	jsonOK(w, nil)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		if err := restartPanelService(); err != nil {
			log.Printf("restart panel: %v", err)
		}
	}()
}
