package main

import (
	"net/http"
	"strings"

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

func (a *App) handleSettingUpdate(w http.ResponseWriter, r *http.Request) {
	var req panelSettingsReq
	if err := readJSON(r, &req); err != nil {
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
	var req panelUserReq
	if err := readJSON(r, &req); err != nil {
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
	if err := serviceRestart(panelServiceUnit); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, nil)
}
