package panel

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (a *App) handleVKCreatorStatus(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, vkCreatorStatus())
}

func (a *App) handleVKCreatorSaveCookies(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
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
	}
	if err := r.ParseForm(); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	if raw := strings.TrimSpace(r.Form.Get("cookies_json")); raw != "" {
		if err := saveVKCookies([]byte(raw)); err != nil {
			jsonMsg(w, err.Error(), false)
			return
		}
		jsonOK(w, vkCreatorStatus())
		return
	}
	if raw := strings.TrimSpace(r.Form.Get("cookie_string")); raw != "" {
		if err := saveVKCookies([]byte(raw)); err != nil {
			jsonMsg(w, err.Error(), false)
			return
		}
		jsonOK(w, vkCreatorStatus())
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	raw := extractCookiesPayload(body)
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

// extractCookiesPayload pulls the cookies content out of a request body that
// may arrive as: (1) JSON {"cookies_json":"..."} / {"cookie_string":"..."},
// (2) form-encoded cookies_json=...&..., or (3) a raw cookies array / cookie
// string pasted directly. It never errors — returns "" only if nothing usable
// is found, so a stray encoding quirk can't surface as "неверный JSON".
func extractCookiesPayload(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return ""
	}
	// (3) Raw cookies array or remixsid string pasted directly as the body.
	if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "remixsid=") {
		return s
	}
	// (1) JSON object body.
	if strings.HasPrefix(s, "{") {
		var req struct {
			CookiesJSON  string `json:"cookies_json"`
			CookieString string `json:"cookie_string"`
		}
		if json.Unmarshal(body, &req) == nil {
			if v := strings.TrimSpace(req.CookiesJSON); v != "" {
				return v
			}
			if v := strings.TrimSpace(req.CookieString); v != "" {
				return v
			}
		}
	}
	// (2) Form-encoded body. Parse leniently: split on & and unescape each
	// pair ourselves, ignoring any pair that fails to unescape.
	for _, pair := range strings.Split(s, "&") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		if key != "cookies_json" && key != "cookie_string" {
			continue
		}
		val, err := url.QueryUnescape(kv[1])
		if err != nil {
			val = kv[1]
		}
		if v := strings.TrimSpace(val); v != "" {
			return v
		}
	}
	return ""
}

func (a *App) handleVKCreatorClearCookies(w http.ResponseWriter, r *http.Request) {
	if err := clearVKCookies(); err != nil {
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
	st := vkCreatorStatus()
	jsonOK(w, map[string]interface{}{
		"join_link":  joinLink,
		"vk_hash":    hash,
		"session":    sess,
		"sessions":   st["sessions"],
		"cookies_ok": st["cookies_ok"],
	})
}

func (a *App) handleVKCreatorStopCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		CallID   string `json:"call_id"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonMsg(w, "неверный JSON", false)
		return
	}
	if err := stopVKCreatorSession(strings.TrimSpace(req.Password), strings.TrimSpace(req.CallID)); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonOK(w, vkCreatorStatus())
}

func (a *App) handleVKCreatorExportCookiesTemplate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="cookies-vk.example.json"`)
	_ = json.NewEncoder(w).Encode([]vkCookieEntry{
		{Name: "remixsid", Value: "PASTE_OR_USE_PANEL_LOGIN"},
		{Name: "remixlang", Value: "0"},
	})
}
