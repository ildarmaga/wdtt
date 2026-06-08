package main

import (
	"crypto/tls"
	"path/filepath"
	"strings"
)

func panelTLSEnabled(cfg *PanelConfig) bool {
	if cfg == nil {
		return false
	}
	cert := strings.TrimSpace(cfg.WebCertFile)
	key := strings.TrimSpace(cfg.WebKeyFile)
	if cert == "" || key == "" {
		return false
	}
	_, err := tls.LoadX509KeyPair(cert, key)
	return err == nil
}

func domainFromCertPath(certFile string) string {
	certFile = strings.TrimSpace(certFile)
	if certFile == "" {
		return ""
	}
	name := filepath.Base(filepath.Dir(certFile))
	if strings.Contains(name, ".") && !strings.ContainsAny(name, `/\ `) {
		return name
	}
	return ""
}

func (cfg *PanelConfig) panelDomain() string {
	if cfg == nil {
		return ""
	}
	if d := strings.TrimSpace(cfg.WebDomain); d != "" {
		return d
	}
	if panelTLSEnabled(cfg) {
		return domainFromCertPath(cfg.WebCertFile)
	}
	return ""
}

func (cfg *PanelConfig) defaultLinkHost(publicIP string) string {
	if d := cfg.panelDomain(); d != "" {
		return d
	}
	return strings.TrimSpace(publicIP)
}

func (a *App) defaultLinkHost() string {
	return a.cfg.defaultLinkHost(a.serverIP())
}

func (a *App) resolveLinkHost(inbound WdttInboundConfig) string {
	if h := strings.TrimSpace(inbound.ServerHost); h != "" {
		return h
	}
	return a.defaultLinkHost()
}
