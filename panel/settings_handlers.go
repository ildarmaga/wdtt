package main

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
}

type panelUserReq struct {
	OldUsername string `json:"oldUsername"`
	OldPassword string `json:"oldPassword"`
	NewUsername string `json:"newUsername"`
	NewPassword string `json:"newPassword"`
}

func (a *App) handleSettingAll(w http.ResponseWriter, r *http.Request) {
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
	if err := validatePanelTLS(req.WebCertFile, req.WebKeyFile); err != nil {
		jsonError(w, err.Error(), 400)
		return
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
	normalizePanelConfig(a.cfg)

	if err := savePanelConfig(a.cfg); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
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

func (a *App) handleSettingRestartPanel(w http.ResponseWriter, r *http.Request) {
	// Ответ до restart: иначе systemctl убивает процесс и клиент видит 503.
	jsonOK(w, nil)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		if err := serviceRestart(panelServiceUnit); err != nil {
			log.Printf("restart panel: %v", err)
		}
	}()
}
