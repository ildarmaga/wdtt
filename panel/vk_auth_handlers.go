package panel

import (
	"log"
	"net/http"
)

func (a *App) handleVKAuthStatus(w http.ResponseWriter, r *http.Request) {
	sess := a.parseSession(r)
	if sess == nil {
		jsonMsg(w, "unauthorized", false)
		return
	}
	jsonOK(w, vkAuthStatus(sess.User))
}

func (a *App) handleVKAuthLogin(w http.ResponseWriter, r *http.Request) {
	sess := a.parseSession(r)
	if sess == nil {
		jsonMsg(w, "unauthorized", false)
		return
	}
	var in vkPasswordLoginInput
	if err := readJSON(r, &in); err != nil {
		log.Printf("[VKLOGIN] user=%s readJSON error: %v", sess.User, err)
		jsonMsg(w, "неверный запрос", false)
		return
	}
	log.Printf("[VKLOGIN] user=%s attempt login=%q hasPass=%v hasCode=%v hasCaptcha=%v",
		sess.User, in.Login, in.Password != "", in.Code != "", in.CaptchaKey != "")
	st, err := getVKLoginState(sess.User)
	if err != nil {
		log.Printf("[VKLOGIN] user=%s state error: %v", sess.User, err)
		jsonMsg(w, err.Error(), false)
		return
	}
	out, err := vkPasswordLogin(st, in)
	if err != nil {
		log.Printf("[VKLOGIN] user=%s vkPasswordLogin error: %v", sess.User, err)
		jsonMsg(w, err.Error(), false)
		return
	}
	log.Printf("[VKLOGIN] user=%s result status=%q loggedIn=%v msg=%q captcha=%v 2fa=%v",
		sess.User, out.Status, out.LoggedIn, out.Message, out.CaptchaImg != "", out.ValidationSid != "")
	if out.Status == "ok" && out.LoggedIn {
		if err := saveVKCookiesFromLogin(sess.User); err != nil {
			jsonMsg(w, err.Error(), false)
			return
		}
		st := vkCreatorStatus()
		st["auth_logged_in"] = true
		st["login_status"] = out.Status
		st["login_message"] = out.Message
		jsonOK(w, st)
		return
	}
	jsonOK(w, map[string]interface{}{
		"login_status":     out.Status,
		"login_message":    out.Message,
		"captcha_sid":      out.CaptchaSid,
		"captcha_img":      out.CaptchaImg,
		"validation_sid":   out.ValidationSid,
		"phone_mask":       out.PhoneMask,
		"validation_type":  out.ValidationTyp,
		"logged_in":        out.LoggedIn,
		"auth_logged_in":   vkAuthStatus(sess.User)["logged_in"],
	})
}

func (a *App) handleVKAuthSave(w http.ResponseWriter, r *http.Request) {
	sess := a.parseSession(r)
	if sess == nil {
		jsonMsg(w, "unauthorized", false)
		return
	}
	if err := saveVKCookiesFromLogin(sess.User); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonOK(w, vkCreatorStatus())
}

func (a *App) handleVKAuthClear(w http.ResponseWriter, r *http.Request) {
	sess := a.parseSession(r)
	if sess == nil {
		jsonMsg(w, "unauthorized", false)
		return
	}
	clearVKLoginState(sess.User)
	jsonOK(w, vkAuthStatus(sess.User))
}

func (a *App) handleVKLoginProxy(w http.ResponseWriter, r *http.Request) {
	a.proxyVKLogin(w, r)
}
