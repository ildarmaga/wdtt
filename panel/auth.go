package panel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "wdtt-panel"

type sessionData struct {
	User      string
	ExpiresAt int64
}

func (a *App) createSession(w http.ResponseWriter, username string) {
	dur := a.cfg.sessionDuration()
	exp := time.Now().Add(dur).Unix()
	payload := username + "|" + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionKey))
	mac.Write([]byte(payload))
	token := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	sess := &sessionData{User: username, ExpiresAt: exp}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     a.cfg.basePath(),
		HttpOnly: true,
		Secure:   a.sessionCookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(dur.Seconds()),
	})
	a.setCSRFCookie(w, sess)
}

func (a *App) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     a.cfg.basePath(),
		HttpOnly: true,
		MaxAge:   -1,
	})
	a.clearCSRFCookie(w)
}

func (a *App) parseSession(r *http.Request) *sessionData {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionKey))
	mac.Write(raw)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return nil
	}
	chunks := strings.SplitN(string(raw), "|", 2)
	if len(chunks) != 2 {
		return nil
	}
	exp, _ := strconv.ParseInt(chunks[1], 10, 64)
	if time.Now().Unix() > exp {
		return nil
	}
	return &sessionData{User: chunks[0], ExpiresAt: exp}
}

func isAjax(r *http.Request) bool {
	return r.Header.Get("X-Requested-With") == "XMLHttpRequest"
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.parseSession(r) == nil {
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
		next(w, r)
	}
}

type loginForm struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	TwoFactorCode string `json:"twoFactorCode"`
}

func parseLoginForm(r *http.Request) (loginForm, error) {
	var form loginForm
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		defer r.Body.Close()
		err := json.NewDecoder(r.Body).Decode(&form)
		return form, err
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return form, err
	}
	defer r.Body.Close()
	for _, pair := range strings.Split(string(body), "&") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := kv[0]
		val := strings.ReplaceAll(kv[1], "+", " ")
		switch key {
		case "username":
			form.Username = val
		case "password":
			form.Password = val
		case "twoFactorCode":
			form.TwoFactorCode = val
		}
	}
	return form, nil
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	form, err := parseLoginForm(r)
	if err != nil {
		jsonError(w, i18nWeb("pages.login.toasts.invalidFormData"), http.StatusOK)
		return
	}
	ip := clientIP(r)
	if !panelLoginLimiter.allow(ip) {
		jsonError(w, i18nWeb("pages.login.toasts.wrongUsernameOrPassword"), http.StatusTooManyRequests)
		return
	}
	if form.Username == "" || form.Password == "" {
		jsonError(w, i18nWeb("pages.login.toasts.emptyUsername"), http.StatusOK)
		return
	}
	if form.Username != a.cfg.Username ||
		bcrypt.CompareHashAndPassword([]byte(a.cfg.PasswordHash), []byte(form.Password)) != nil {
		panelLoginLimiter.recordFail(ip)
		jsonError(w, i18nWeb("pages.login.toasts.wrongUsernameOrPassword"), http.StatusOK)
		return
	}
	panelLoginLimiter.reset(ip)
	a.createSession(w, form.Username)
	jsonOK(w, nil)
}

func (a *App) handleTwoFactorEnable(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, false)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.clearSession(w)
	if r.Method == http.MethodPost || isAjax(r) {
		jsonOK(w, nil)
		return
	}
	http.Redirect(w, r, a.cfg.basePath(), http.StatusFound)
}
