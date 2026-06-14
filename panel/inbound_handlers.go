package main

import (
	"net/http"
)

func inboundAPIPayload(a *App, cfg WdttInboundConfig) map[string]interface{} {
	db, _ := loadPasswords()
	mainPass := ""
	if db != nil {
		mainPass = db.MainPassword
	}
	st := collectWdttInboundStatus(cfg)
	stats := loadServerStats()
	upBytes, downBytes := inboundTrafficTotals(db, stats)
	trafficUsed := upBytes + downBytes
	serviceActive := st.ServiceActive
	return map[string]interface{}{
		"configured":        isWdttInboundConfigured(),
		"tag":               cfg.Tag,
		"enable":            cfg.Enable,
		"listen_host":      cfg.ListenHost,
		"dtls_port":        cfg.DtlsPort,
		"wg_port":           cfg.WgPort,
		"server_ip":        a.serverIP(),
		"default_link_host": a.defaultLinkHost(),
		"panel_tls":        panelTLSEnabled(a.cfg),
		"link_main":        buildWdttLink(a.resolveLinkHost(cfg), mainPass, a.cfg.SubTitle, "", &PasswordEntry{Comment: "Владелец"}, cfg, ""),
		"remark":           cfg.Remark,
		"server_host":      cfg.ServerHost,
		"client_port":      cfg.ClientPort,
		"dns":              cfg.DNS,
		"mtu":              cfg.MTU,
		"max_users":        cfg.MaxUsers,
		"handshake_timeout_sec": cfg.HandshakeTimeoutSec,
		"max_dtls_per_device":   cfg.MaxDtlsPerDevice,
		"online_timeout_sec":    cfg.OnlineTimeoutSec,
		"admin_addr":            cfg.AdminAddr,
		"service_active":   serviceActive,
		"iface_up":         st.IfaceUp,
		"iface_addr":       st.IfaceAddr,
		"dtls_firewall":    st.DtlsFirewall,
		"wg_firewall":      st.WgFirewall,
		"dtls_listening":   st.DtlsListening,
		"wg_listening":     st.WgListening,
		"active_users":     st.ActiveUsers,
		"total_users":      st.TotalUsers,
		"online_users":     st.OnlineUsers,
		"xray_active":      st.XrayActive,
		"up_bytes":         upBytes,
		"down_bytes":       downBytes,
		"traffic_used":     trafficUsed,
		"traffic_used_fmt": formatBytes(trafficUsed),
		"up_fmt":           formatBytes(upBytes),
		"down_fmt":         formatBytes(downBytes),
	}
}

func (a *App) serveConnectionsPage(w http.ResponseWriter, r *http.Request) {
	if a.parseSession(r) == nil {
		http.Redirect(w, r, a.cfg.basePath(), http.StatusFound)
		return
	}
	a.renderHTML(w, r, "connections.html", "Подключения", nil)
}

func (a *App) handleInboundGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", 405)
		return
	}
	cfg, err := loadWdttInbound()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, inboundAPIPayload(a, cfg))
}

func (a *App) handleInboundSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	var req WdttInboundConfig
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	restarted, err := applyWdttInbound(req)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	cfg, _ := loadWdttInbound()
	payload := inboundAPIPayload(a, cfg)
	payload["restarted"] = restarted
	payload["hot_reload"] = !restarted && req.Enable
	jsonOK(w, payload)
}
