package panel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
)

const (
	vkAndroidClientID     = "2274003"
	vkAndroidClientSecret = "hHbZxrka2uZ6jB1inYsH"
	vkOAuthAPIVersion     = "5.199"
)

var (
	vkLoginIPRe       = regexp.MustCompile(`name="ip_h"\s+value="([^"]+)"`)
	vkLoginDomainRe   = regexp.MustCompile(`name="lg_domain_h"\s+value="([^"]+)"`)
	vkLoginToRe       = regexp.MustCompile(`name="to"\s+value="([^"]*)"`)
	vkLoginCaptchaRe  = regexp.MustCompile(`onLoginCaptcha\(\s*(\d+)`)
	vkLoginCaptchaImg = regexp.MustCompile(`src="(https?://[^"]+captcha[^"]+)"`)
)

type vkPasswordLoginInput struct {
	Login         string `json:"login"`
	Password      string `json:"password"`
	CaptchaSid    string `json:"captcha_sid,omitempty"`
	CaptchaKey    string `json:"captcha_key,omitempty"`
	Code          string `json:"code,omitempty"`
	ValidationSid string `json:"validation_sid,omitempty"`
}

type vkPasswordLoginOutcome struct {
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
	CaptchaSid    string `json:"captcha_sid,omitempty"`
	CaptchaImg    string `json:"captcha_img,omitempty"`
	ValidationSid string `json:"validation_sid,omitempty"`
	PhoneMask     string `json:"phone_mask,omitempty"`
	ValidationTyp string `json:"validation_type,omitempty"`
	LoggedIn      bool   `json:"logged_in"`
}

func vkPasswordLogin(st *vkLoginState, in vkPasswordLoginInput) (*vkPasswordLoginOutcome, error) {
	login := strings.TrimSpace(in.Login)
	pass := strings.TrimSpace(in.Password)
	if login == "" || pass == "" {
		return &vkPasswordLoginOutcome{
			Status:  "error",
			Message: "Укажите логин (телефон или email) и пароль",
		}, nil
	}

	client := vkLoginHTTPClient(st.jar)

	// Warm up only on the initial attempt. On a 2FA/captcha retry we must NOT
	// re-fetch vk.com/login.vk.com: it adds extra bot-signal requests and VK
	// counts the resend as another password attempt, which trips flood control
	// right at the code-submission step.
	isRetry := strings.TrimSpace(in.Code) != "" ||
		strings.TrimSpace(in.ValidationSid) != "" ||
		strings.TrimSpace(in.CaptchaSid) != ""
	if !isRetry {
		if err := vkWarmupLoginSession(client); err != nil {
			return nil, err
		}
	}

	oauthOut := vkOAuthPasswordLogin(client, st.jar, login, pass, in)
	if oauthOut.Status == "ok" || oauthOut.Status == "need_captcha" || oauthOut.Status == "need_2fa" {
		return oauthOut, nil
	}

	legacyOut, err := vkLegacyPasswordLogin(client, st.jar, login, pass, in.CaptchaSid, in.CaptchaKey, in.Code)
	if err != nil {
		return nil, err
	}
	if legacyOut.Status == "ok" || legacyOut.Status == "need_captcha" || legacyOut.Status == "need_2fa" {
		return legacyOut, nil
	}
	if oauthOut.Message != "" {
		return oauthOut, nil
	}
	return legacyOut, nil
}

func vkLoginHTTPClient(jar http.CookieJar) *http.Client {
	return &http.Client{
		Jar:     jar,
		Timeout: 0,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

func vkWarmupLoginSession(client *http.Client) error {
	for _, u := range []string{"https://vk.com/", "https://login.vk.com/?act=login"} {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		vkSetBrowserHeaders(req, "https://vk.com/")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return nil
}

func vkSetBrowserHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", vkProxyUserAgent)
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

type vkOAuthTokenResponse struct {
	AccessToken      string `json:"access_token"`
	UserID           int64  `json:"user_id"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	ErrorType        string `json:"error_type"`
	CaptchaSid       json.RawMessage `json:"captcha_sid"`
	CaptchaImg       string          `json:"captcha_img"`
	ValidationSid    string          `json:"validation_sid"`
	PhoneMask        string          `json:"phone_mask"`
	ValidationType   string          `json:"validation_type"`
	RedirectURI      string          `json:"redirect_uri"`
}

func vkOAuthPasswordLogin(client *http.Client, jar http.CookieJar, login, pass string, in vkPasswordLoginInput) *vkPasswordLoginOutcome {
	form := url.Values{
		"grant_type":    {"password"},
		"client_id":     {vkAndroidClientID},
		"client_secret": {vkAndroidClientSecret},
		"username":      {login},
		"password":      {pass},
		"scope":         {"all"},
		"2fa_supported": {"1"},
		"v":             {vkOAuthAPIVersion},
		"lang":          {"ru"},
	}
	if sid := strings.TrimSpace(in.CaptchaSid); sid != "" {
		form.Set("captcha_sid", sid)
		form.Set("captcha_key", strings.TrimSpace(in.CaptchaKey))
	}
	if code := strings.TrimSpace(in.Code); code != "" {
		form.Set("code", code)
	}
	if vs := strings.TrimSpace(in.ValidationSid); vs != "" {
		form.Set("validation_sid", vs)
	}

	req, err := http.NewRequest(http.MethodPost, "https://oauth.vk.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return &vkPasswordLoginOutcome{Status: "error", Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	vkSetBrowserHeaders(req, "https://vk.com/")

	resp, err := client.Do(req)
	if err != nil {
		return &vkPasswordLoginOutcome{Status: "error", Message: err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &vkPasswordLoginOutcome{Status: "error", Message: err.Error()}
	}

	var tok vkOAuthTokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return &vkPasswordLoginOutcome{Status: "error", Message: "неожиданный ответ VK OAuth"}
	}

	switch tok.Error {
	case "":
		if tok.AccessToken == "" {
			return &vkPasswordLoginOutcome{Status: "error", Message: "пустой access_token от VK"}
		}
		if err := vkExchangeAccessTokenToWebSession(client, tok.AccessToken); err != nil {
			return &vkPasswordLoginOutcome{Status: "error", Message: err.Error()}
		}
		if !vkJarHasRemixsidHTTP(jar) {
			return &vkPasswordLoginOutcome{
				Status:  "error",
				Message: "вход выполнен, но remixsid не получен — используйте ручную загрузку cookies или вход через vk.com в окне",
			}
		}
		return &vkPasswordLoginOutcome{Status: "ok", LoggedIn: true, Message: "Вход выполнен"}
	case "need_captcha":
		sid := vkRawJSONString(tok.CaptchaSid)
		return &vkPasswordLoginOutcome{
			Status:     "need_captcha",
			Message:    "Требуется капча",
			CaptchaSid: sid,
			CaptchaImg: tok.CaptchaImg,
		}
	case "need_validation":
		msg := tok.ErrorDescription
		if msg == "" {
			msg = "Требуется код подтверждения"
		}
		return &vkPasswordLoginOutcome{
			Status:        "need_2fa",
			Message:       msg,
			ValidationSid: tok.ValidationSid,
			PhoneMask:     tok.PhoneMask,
			ValidationTyp: tok.ValidationType,
		}
	default:
		msg := strings.TrimSpace(tok.ErrorDescription)
		if msg == "" {
			msg = tok.Error
		}
		if msg == "" {
			msg = string(body)
		}
		return &vkPasswordLoginOutcome{Status: "error", Message: msg}
	}
}

func vkRawJSONString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return strings.Trim(string(raw), `"`)
}

func vkExchangeAccessTokenToWebSession(client *http.Client, accessToken string) error {
	authURL := "https://oauth.vk.com/authorize?" + url.Values{
		"client_id":     {"6287487"},
		"redirect_uri":  {"https://oauth.vk.com/blank.html"},
		"response_type": {"token"},
		"access_token":  {accessToken},
		"scope":         {"1073737727"},
		"display":       {"mobile"},
		"v":             {vkOAuthAPIVersion},
	}.Encode()

	req, err := http.NewRequest(http.MethodGet, authURL, nil)
	if err != nil {
		return err
	}
	vkSetBrowserHeaders(req, "https://vk.com/")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if vkJarHasRemixsidHTTP(client.Jar) {
		return nil
	}

	req2, err := http.NewRequest(http.MethodGet, "https://vk.com/feed", nil)
	if err != nil {
		return err
	}
	vkSetBrowserHeaders(req2, "https://vk.com/")
	resp2, err := client.Do(req2)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	return nil
}

func vkJarHasRemixsidHTTP(jar http.CookieJar) bool {
	cj, ok := jar.(*cookiejar.Jar)
	if !ok || cj == nil {
		return false
	}
	return vkJarHasRemixsid(cj)
}

func vkLegacyPasswordLogin(client *http.Client, jar http.CookieJar, login, pass, captchaSid, captchaKey, code string) (*vkPasswordLoginOutcome, error) {
	page, err := vkFetchText(client, "https://vk.com/", "https://vk.com/")
	if err != nil {
		return nil, err
	}

	ipH := vkLoginIPRe.FindStringSubmatch(page)
	lgH := vkLoginDomainRe.FindStringSubmatch(page)
	if len(ipH) < 2 || len(lgH) < 2 {
		return &vkPasswordLoginOutcome{
			Status:  "error",
			Message: "не удалось получить форму входа VK — попробуйте позже или загрузите cookies вручную",
		}, nil
	}
	toVal := ""
	if m := vkLoginToRe.FindStringSubmatch(page); len(m) >= 2 {
		toVal = m[1]
	}

	form := url.Values{
		"act":         {"login"},
		"role":        {"al_frame"},
		"expire":      {""},
		"to":          {toVal},
		"recaptcha":   {""},
		"captcha_sid": {captchaSid},
		"captcha_key": {captchaKey},
		"_origin":     {"https://vk.com"},
		"utf8":        {"1"},
		"ip_h":        {ipH[1]},
		"lg_domain_h": {lgH[1]},
		"ul":          {""},
		"email":       {login},
		"pass":        {pass},
	}
	if code != "" {
		form.Set("code", code)
	}

	req, err := http.NewRequest(http.MethodPost, "https://login.vk.com/?act=login", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://login.vk.com")
	vkSetBrowserHeaders(req, "https://login.vk.com/?act=login")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	text := string(body)

	if strings.Contains(text, "onLoginDone(") || vkJarHasRemixsidHTTP(jar) {
		return &vkPasswordLoginOutcome{Status: "ok", LoggedIn: true, Message: "Вход выполнен"}, nil
	}
	if strings.Contains(text, "onLoginFailed(4,") {
		return &vkPasswordLoginOutcome{Status: "error", Message: "Неверный логин или пароль"}, nil
	}
	if strings.Contains(text, "onLoginCaptcha(") {
		sid := ""
		if m := vkLoginCaptchaRe.FindStringSubmatch(text); len(m) >= 2 {
			sid = m[1]
		}
		img := ""
		if m := vkLoginCaptchaImg.FindStringSubmatch(text); len(m) >= 2 {
			img = m[1]
		}
		if img == "" && sid != "" {
			img = "https://api.vk.com/captcha.php?sid=" + url.QueryEscape(sid)
		}
		return &vkPasswordLoginOutcome{
			Status:     "need_captcha",
			Message:    "Требуется капча",
			CaptchaSid: sid,
			CaptchaImg: img,
		}, nil
	}
	if strings.Contains(text, "act=authcheck") || strings.Contains(text, "authcheck") {
		return &vkPasswordLoginOutcome{
			Status:  "need_2fa",
			Message: "Требуется код двухфакторной аутентификации — отправьте SMS-код в поле «Код 2FA»",
		}, nil
	}
	if strings.Contains(text, "onLoginFailed") {
		return &vkPasswordLoginOutcome{
			Status:  "error",
			Message: "VK отклонил вход — попробуйте OAuth-форму ниже или загрузите cookies вручную",
		}, nil
	}
	return &vkPasswordLoginOutcome{
		Status:  "error",
		Message: "не удалось завершить вход через vk.com",
	}, nil
}

func vkFetchText(client *http.Client, target, referer string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	vkSetBrowserHeaders(req, referer)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	return string(body), nil
}
