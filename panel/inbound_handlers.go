package main

import (
	"net/http"
)

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
	db, _ := loadPasswords()
	mainPass := ""
	if db != nil {
		mainPass = db.MainPassword
	}
	st := collectWdttInboundStatus(cfg)
	jsonOK(w, map[string]interface{}{
		"tag":              cfg.Tag,
		"listen_host":      cfg.ListenHost,
		"dtls_port":        cfg.DtlsPort,
		"wg_port":          cfg.WgPort,
		"server_ip":          a.serverIP(),
		"default_link_host":  a.defaultLinkHost(),
		"panel_tls":          panelTLSEnabled(a.cfg),
		"link_main":          buildWdttLink(a.resolveLinkHost(cfg), mainPass, "", &PasswordEntry{Comment: "Владелец"}, cfg),
		"main_password":    mainPass,
		"remark":           cfg.Remark,
		"server_host":      cfg.ServerHost,
		"client_port":      cfg.ClientPort,
		"dns":              cfg.DNS,
		"max_users":        cfg.MaxUsers,
		"service_active":   st.ServiceActive,
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
	})
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
	if err := applyWdttInbound(req); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	cfg, _ := loadWdttInbound()
	db, _ := loadPasswords()
	mainPass := ""
	if db != nil {
		mainPass = db.MainPassword
	}
	st := collectWdttInboundStatus(cfg)
	jsonOK(w, map[string]interface{}{
		"tag":            cfg.Tag,
		"remark":         cfg.Remark,
		"listen_host":    cfg.ListenHost,
		"server_host":    cfg.ServerHost,
		"dtls_port":      cfg.DtlsPort,
		"wg_port":        cfg.WgPort,
		"client_port":    cfg.ClientPort,
		"dns":            cfg.DNS,
		"max_users":      cfg.MaxUsers,
		"link_main":         buildWdttLink(a.resolveLinkHost(cfg), mainPass, "", &PasswordEntry{Comment: "Владелец"}, cfg),
		"main_password":     mainPass,
		"server_ip":         a.serverIP(),
		"default_link_host": a.defaultLinkHost(),
		"panel_tls":         panelTLSEnabled(a.cfg),
		"service_active": st.ServiceActive,
		"iface_up":       st.IfaceUp,
		"iface_addr":     st.IfaceAddr,
		"dtls_firewall":  st.DtlsFirewall,
		"wg_firewall":    st.WgFirewall,
		"dtls_listening": st.DtlsListening,
		"wg_listening":   st.WgListening,
		"active_users":   st.ActiveUsers,
		"total_users":    st.TotalUsers,
		"online_users":   st.OnlineUsers,
		"xray_active":    st.XrayActive,
	})
}
