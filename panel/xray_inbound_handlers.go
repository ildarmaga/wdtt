package main

import (
	"net/http"
)

func (a *App) handleXrayInboundsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", 405)
		return
	}
	rows, err := listPanelXrayInbounds()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if rows == nil {
		rows = []PanelXrayInboundRow{}
	}
	jsonOK(w, map[string]interface{}{"inbounds": rows})
}

func (a *App) handleXrayInboundSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	var req PanelXrayInboundSaveRequest
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := savePanelXrayInbound(req); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	rows, _ := listPanelXrayInbounds()
	jsonOK(w, map[string]interface{}{"inbounds": rows})
}

func (a *App) handleXrayInboundDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	var req struct {
		Tag string `json:"tag"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := deletePanelXrayInbound(req.Tag); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]interface{}{"tag": req.Tag})
}
