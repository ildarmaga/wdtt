package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// Панель
	panelConfigPath   = "/etc/wdtt/panel.json"
	panelXrayMetaPath = "/etc/wdtt/panel-xray.json"

	// WDTT VPN-сервер
	wdttConfigDir     = "/etc/wdtt"
	wdttServerBin     = "/usr/local/bin/wdtt-server"
	wdttXrayRulesPath = "/usr/local/bin/wdtt-xray-rules.sh"
	wdttServiceUnit   = "wdtt.service"

	// WDTT xray
	xrayConfigDir  = "/etc/wdtt-xray"
	xrayConfigPath = "/etc/wdtt-xray/config.json"
	xrayBinDir     = "/usr/local/wdtt-xray/bin"
	xrayAssetDir   = "/usr/local/wdtt-xray/bin"
	xrayLogDir     = "/var/log/wdtt-xray"
	xrayServiceUnit = "wdtt-xray.service"

	panelServiceUnit = "wdtt-panel.service"
)

type PanelConfig struct {
	Username      string `json:"username"`
	PasswordHash  string `json:"password_hash"`
	Port          int    `json:"port"`
	WebBasePath   string `json:"web_base_path"`
	SessionKey    string `json:"session_key"`
	WebListen     string `json:"webListen,omitempty"`
	WebDomain     string `json:"webDomain,omitempty"`
	WebCertFile   string `json:"webCertFile,omitempty"`
	WebKeyFile    string `json:"webKeyFile,omitempty"`
	SessionMaxAge int    `json:"sessionMaxAge,omitempty"`
	PageSize      int    `json:"pageSize,omitempty"`
	RemarkModel   string `json:"remarkModel,omitempty"`
}

func loadPanelConfig() (*PanelConfig, error) {
	data, err := os.ReadFile(panelConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return createDefaultPanelConfig()
		}
		return nil, err
	}
	var cfg PanelConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Port == 0 {
		cfg.Port = 2860
	}
	if cfg.WebBasePath == "" {
		cfg.WebBasePath = "/wdtt/"
	}
	if cfg.SessionKey == "" {
		cfg.SessionKey = randomHex(32)
		savePanelConfig(&cfg)
	}
	normalizePanelConfig(&cfg)
	if cfg.WebDomain == "" && panelTLSEnabled(&cfg) {
		if d := domainFromCertPath(cfg.WebCertFile); d != "" {
			cfg.WebDomain = d
			_ = savePanelConfig(&cfg)
		}
	}
	return &cfg, nil
}

func normalizePanelConfig(cfg *PanelConfig) {
	if cfg.Port == 0 {
		cfg.Port = 2860
	}
	if cfg.WebBasePath == "" {
		cfg.WebBasePath = "/wdtt/"
	}
	if cfg.SessionMaxAge <= 0 {
		cfg.SessionMaxAge = 60
	}
	if cfg.PageSize <= 0 {
		cfg.PageSize = 50
	}
	if cfg.RemarkModel == "" {
		cfg.RemarkModel = "-ieo"
	}
	cfg.RemarkModel = normalizeRemarkModel(cfg.RemarkModel)
}

func normalizeRemarkModel(s string) string {
	if len(s) < 2 {
		return "-ieo"
	}
	sep := s[0]
	seen := make(map[byte]bool)
	parts := make([]byte, 0, len(s)-1)
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c != 'i' && c != 'e' && c != 'o' {
			continue
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		parts = append(parts, c)
	}
	if len(parts) == 0 {
		return "-ieo"
	}
	return string(append([]byte{sep}, parts...))
}

func createDefaultPanelConfig() (*PanelConfig, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte("wdtt"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	cfg := &PanelConfig{
		Username:      "admin",
		PasswordHash:  string(hash),
		Port:          2860,
		WebBasePath:   "/wdtt/",
		SessionKey:    randomHex(32),
		SessionMaxAge: 60,
		PageSize:      50,
		RemarkModel:   "-ieo",
	}
	if err := savePanelConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func savePanelConfig(cfg *PanelConfig) error {
	os.MkdirAll(filepath.Dir(panelConfigPath), 0700)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(panelConfigPath, data, 0600)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (c *PanelConfig) sessionDuration() time.Duration {
	if c.SessionMaxAge > 0 {
		return time.Duration(c.SessionMaxAge) * time.Minute
	}
	return 24 * time.Hour
}

func panelSettingsMap(cfg *PanelConfig) map[string]interface{} {
	return map[string]interface{}{
		"webListen":        cfg.WebListen,
		"webDomain":        cfg.WebDomain,
		"webPort":          cfg.Port,
		"webBasePath":      cfg.WebBasePath,
		"sessionMaxAge":    cfg.SessionMaxAge,
		"pageSize":         cfg.PageSize,
		"remarkModel":      cfg.RemarkModel,
		"webCertFile":      cfg.WebCertFile,
		"webKeyFile":       cfg.WebKeyFile,
		"twoFactorEnable":  false,
	}
}

func (c *PanelConfig) basePath() string {
	p := c.WebBasePath
	if p == "" {
		return "/wdtt/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	if p[len(p)-1] != '/' {
		p += "/"
	}
	return p
}
