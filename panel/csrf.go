package panel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const csrfCookie = "wdtt-csrf"

func (a *App) csrfTokenForSession(sess *sessionData) string {
	if sess == nil || a.cfg == nil {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionKey))
	mac.Write([]byte(fmt.Sprintf("csrf|%s|%d", sess.User, sess.ExpiresAt)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:18])
}

func (a *App) validCSRF(sess *sessionData, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" || sess == nil {
		return false
	}
	expected := a.csrfTokenForSession(sess)
	return expected != "" && hmac.Equal([]byte(token), []byte(expected))
}

func (a *App) setCSRFCookie(w http.ResponseWriter, sess *sessionData) {
	token := a.csrfTokenForSession(sess)
	if token == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    token,
		Path:     a.cfg.basePath(),
		Secure:   a.sessionCookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(a.cfg.sessionDuration().Seconds()),
	})
}

func (a *App) clearCSRFCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    "",
		Path:     a.cfg.basePath(),
		MaxAge:   -1,
	})
}

func (a *App) requireAuthCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Bearer token authentication (API key).
		if token := parseBearerToken(r); token != "" {
			apiTok, err := lookupAPITokenCached(token)
			if err != nil || apiTok == nil {
				jsonError(w, "invalid api token", http.StatusUnauthorized)
				return
			}
			if apiTok.Scope == "readonly" {
				switch r.Method {
				case http.MethodGet, http.MethodHead, http.MethodOptions:
				default:
					jsonError(w, "readonly token cannot modify", http.StatusForbidden)
					return
				}
			}
			go touchAPITokenAsync(apiTok.ID)
			next(w, r)
			return
		}
		// Cookie + CSRF authentication (web UI).
		sess := a.parseSession(r)
		if sess == nil {
			if isAjax(r) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"msg":     i18nWeb("pages.login.loginAgain"),
				})
				return
			}
			http.Redirect(w, r, a.cfg.basePath(), http.StatusFound)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !a.validCSRF(sess, r.Header.Get("X-CSRF-Token")) {
				jsonError(w, "invalid csrf token", http.StatusForbidden)
				return
			}
		}
		a.setCSRFCookie(w, sess)
		next(w, r)
	}
}
