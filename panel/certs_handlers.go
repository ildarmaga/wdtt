package main

import (
	"net/http"
	"strings"
)

type certIssueReq struct {
	Domain       string `json:"domain"`
	IP           string `json:"ip"`
	IPv6         string `json:"ipv6"`
	HTTPPort     int    `json:"httpPort"`
	ApplyToPanel bool   `json:"applyToPanel"`
	RestartPanel bool   `json:"restartPanel"`
}

type certDomainReq struct {
	Domain string `json:"domain"`
}

type certApplyReq struct {
	CertFile     string `json:"certFile"`
	KeyFile      string `json:"keyFile"`
	RestartPanel bool   `json:"restartPanel"`
}

type acmeCronReq struct {
	Enabled bool `json:"enabled"`
	Hour    int  `json:"hour"`
	Minute  int  `json:"minute"`
}

func (a *App) handleCertsList(w http.ResponseWriter, r *http.Request) {
	certs, err := listCertificates(a.cfg)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]interface{}{
		"certs": certs,
		"acme":  acmeStatus(),
		"panel": map[string]interface{}{
			"webCertFile": a.cfg.WebCertFile,
			"webKeyFile":  a.cfg.WebKeyFile,
			"webDomain":   a.cfg.WebDomain,
			"tlsActive":   panelTLSEnabled(a.cfg),
		},
	})
}

func (a *App) handleCertsIssue(w http.ResponseWriter, r *http.Request) {
	var req certIssueReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	domain := strings.TrimSpace(req.Domain)
	title := "Выпуск SSL для " + domain
	if !acmeJobTryStart(title) {
		jsonMsg(w, "Уже выполняется операция ACME", false)
		return
	}
	cfg := a.cfg
	go func() {
		result, err := issueAcmeDomain(domain, req.HTTPPort, req.ApplyToPanel, cfg)
		if err != nil {
			acmeJobFail(err)
			return
		}
		if req.ApplyToPanel {
			cfg.WebCertFile = result["certFile"].(string)
			cfg.WebKeyFile = result["keyFile"].(string)
			if req.RestartPanel {
				go func() { _ = serviceRestart(panelServiceUnit) }()
			}
		}
		acmeJobDone(result)
	}()
	jsonOK(w, map[string]interface{}{"started": true})
}

func (a *App) handleCertsIssueIP(w http.ResponseWriter, r *http.Request) {
	var req certIssueReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	title := "Выпуск SSL для IP"
	if req.IP != "" {
		title += " " + strings.TrimSpace(req.IP)
	}
	if !acmeJobTryStart(title) {
		jsonMsg(w, "Уже выполняется операция ACME", false)
		return
	}
	cfg := a.cfg
	go func() {
		result, err := issueAcmeIP(req.IP, req.IPv6, req.HTTPPort, req.ApplyToPanel, cfg)
		if err != nil {
			acmeJobFail(err)
			return
		}
		if req.ApplyToPanel {
			cfg.WebCertFile = result["certFile"].(string)
			cfg.WebKeyFile = result["keyFile"].(string)
			if req.RestartPanel {
				go func() { _ = serviceRestart(panelServiceUnit) }()
			}
		}
		acmeJobDone(result)
	}()
	jsonOK(w, map[string]interface{}{"started": true})
}

func (a *App) handleCertsRenew(w http.ResponseWriter, r *http.Request) {
	var req certDomainReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	domain := strings.TrimSpace(req.Domain)
	title := "Обновление сертификата " + domain
	if !acmeJobTryStart(title) {
		jsonMsg(w, "Уже выполняется операция ACME", false)
		return
	}
	go func() {
		result, err := renewAcmeCert(domain)
		if err != nil {
			acmeJobFail(err)
			return
		}
		acmeJobDone(result)
	}()
	jsonOK(w, map[string]interface{}{"started": true})
}

func (a *App) handleCertsAcmeLog(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, acmeJobPollJSON())
}

func (a *App) handleCertsRenewDtls(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Restart bool `json:"restart"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	restart := true
	if r.ContentLength > 0 {
		restart = req.Restart
	}
	result, err := renewDTLSCert(restart, a.cfg)
	if err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonOK(w, result)
}

func (a *App) handleCertsRevoke(w http.ResponseWriter, r *http.Request) {
	var req certDomainReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	result, err := revokeAcmeCert(req.Domain, a.cfg)
	if err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonOK(w, result)
}

func (a *App) handleCertsDeleteCertbot(w http.ResponseWriter, r *http.Request) {
	var req certDomainReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	result, err := deleteLetsEncryptCert(req.Domain, a.cfg)
	if err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonOK(w, result)
}

func (a *App) handleCertsApply(w http.ResponseWriter, r *http.Request) {
	var req certApplyReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := applyPanelCert(a.cfg, req.CertFile, req.KeyFile, req.RestartPanel); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	a.cfg.WebCertFile = strings.TrimSpace(req.CertFile)
	a.cfg.WebKeyFile = strings.TrimSpace(req.KeyFile)
	certs, _ := listCertificates(a.cfg)
	jsonOK(w, map[string]interface{}{
		"message": "Сертификат панели обновлён",
		"certs":   certs,
		"panel": map[string]interface{}{
			"webCertFile": a.cfg.WebCertFile,
			"webKeyFile":  a.cfg.WebKeyFile,
			"webDomain":   a.cfg.WebDomain,
			"tlsActive":   panelTLSEnabled(a.cfg),
		},
	})
}

func (a *App) handleCertsInstallAcme(w http.ResponseWriter, r *http.Request) {
	if err := installAcme(); err != nil {
		jsonMsg(w, "Не удалось установить acme.sh: "+err.Error(), false)
		return
	}
	jsonOK(w, map[string]interface{}{
		"message": "acme.sh установлен, cron автообновления настроен",
		"acme":    acmeStatus(),
	})
}

func (a *App) handleCertsAcmeCron(w http.ResponseWriter, r *http.Request) {
	var req acmeCronReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if req.Enabled && !acmeInstalled() {
		jsonMsg(w, "Сначала установите acme.sh", false)
		return
	}
	if err := setAcmeCron(req.Enabled, req.Hour, req.Minute); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	msg := "Автообновление отключено"
	if req.Enabled {
		h, m := normalizeAcmeCronTime(req.Hour, req.Minute)
		msg = "Автообновление настроено на " + acmeCronScheduleText(h, m)
	}
	jsonOK(w, map[string]interface{}{
		"message": msg,
		"acme":    acmeStatus(),
	})
}
