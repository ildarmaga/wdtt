package panel

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (a *App) handleVKCreatorStatus(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, vkCreatorStatus())
}

func (a *App) handleVKCreatorSaveCookies(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err == nil && r.MultipartForm != nil {
		if files := r.MultipartForm.File["file"]; len(files) > 0 {
			f, err := files[0].Open()
			if err != nil {
				jsonMsg(w, err.Error(), false)
				return
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				jsonMsg(w, err.Error(), false)
				return
			}
			if err := saveVKCookies(data); err != nil {
				jsonMsg(w, err.Error(), false)
				return
			}
			jsonOK(w, vkCreatorStatus())
			return
		}
	}
	var req struct {
		CookiesJSON   string `json:"cookies_json"`
		CookieString  string `json:"cookie_string"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonMsg(w, "неверный JSON", false)
		return
	}
	raw := strings.TrimSpace(req.CookiesJSON)
	if raw == "" {
		raw = strings.TrimSpace(req.CookieString)
	}
	if raw == "" {
		jsonMsg(w, "передайте cookies_json или cookie_string", false)
		return
	}
	if err := saveVKCookies([]byte(raw)); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonOK(w, vkCreatorStatus())
}

func (a *App) handleVKCreatorClearCookies(w http.ResponseWriter, r *http.Request) {
	if err := clearVKCookies(); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonOK(w, vkCreatorStatus())
}

func (a *App) handleVKCreatorInstallBinary(w http.ResponseWriter, r *http.Request) {
	if err := ensureVKCreatorBinary(); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonOK(w, vkCreatorStatus())
}

func (a *App) handleVKCreatorCreateCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		Apply    bool   `json:"apply"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonMsg(w, "неверный JSON", false)
		return
	}
	joinLink, hash, sess, err := createVKCall(strings.TrimSpace(req.Password))
	if err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	if req.Apply && req.Password != "" {
		if err := applyVKHashToUser(req.Password, hash); err != nil {
			jsonMsg(w, "hash получен, но не сохранён: "+err.Error(), false)
			return
		}
	}
	jsonOK(w, map[string]interface{}{
		"join_link": joinLink,
		"vk_hash":   hash,
		"session":   sess,
	})
}

func (a *App) handleVKCreatorStopCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonMsg(w, "неверный JSON", false)
		return
	}
	if err := stopVKCreatorSession(strings.TrimSpace(req.Password)); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonOK(w, vkCreatorStatus())
}

func (a *App) handleVKCreatorExportCookiesTemplate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="cookies-vk.example.json"`)
	_ = json.NewEncoder(w).Encode([]vkCookieEntry{
		{Name: "remixsid", Value: "PASTE_FROM_WHITELISTBYPASS_CREATOR"},
		{Name: "remixlang", Value: "0"},
	})
}
