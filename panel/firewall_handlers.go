package main

import (
	"net/http"
)

type firewallPortReq struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Comment  string `json:"comment"`
}

type firewallPortUpdateReq struct {
	OldProtocol string `json:"old_protocol"`
	OldPort     int    `json:"old_port"`
	Protocol    string `json:"protocol"`
	Port        int    `json:"port"`
	Comment     string `json:"comment"`
}

func (a *App) handleFirewallPortsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	ports, err := listFirewallPorts()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if ports == nil {
		ports = []FirewallPort{}
	}
	jsonOK(w, map[string]interface{}{"ports": ports, "backend": firewallBackend(), "ufw_active": ufwActive()})
}

func (a *App) handleFirewallPortOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req firewallPortReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := firewallPortOpen(req.Protocol, req.Port, req.Comment); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	ports, _ := listFirewallPorts()
	if ports == nil {
		ports = []FirewallPort{}
	}
	jsonOK(w, map[string]interface{}{"ports": ports, "backend": firewallBackend(), "ufw_active": ufwActive()})
}

func (a *App) handleFirewallPortUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req firewallPortUpdateReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := firewallPortUpdate(req.OldProtocol, req.OldPort, req.Protocol, req.Port, req.Comment); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	ports, _ := listFirewallPorts()
	if ports == nil {
		ports = []FirewallPort{}
	}
	jsonOK(w, map[string]interface{}{"ports": ports, "backend": firewallBackend(), "ufw_active": ufwActive()})
}

func (a *App) handleFirewallPortClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req firewallPortReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := firewallPortClose(req.Protocol, req.Port); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	ports, _ := listFirewallPorts()
	if ports == nil {
		ports = []FirewallPort{}
	}
	jsonOK(w, map[string]interface{}{"ports": ports, "backend": firewallBackend(), "ufw_active": ufwActive()})
}
