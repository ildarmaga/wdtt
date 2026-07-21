package panel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	vkCallAppID      = "6287487"
	vkCallAPIVersion = "5.280"
	// Cookie/creator API host — flipped vk.com→vk.ru (anton48 build 171).
	vkCallWebHost    = "vk.ru"
	vkCallUserAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
)

var vkCallHTTPClient = &http.Client{Timeout: 60 * time.Second}

type vkAPIError struct {
	Code    int    `json:"error_code"`
	Message string `json:"error_msg"`
}

func vkCookieHeaderFromFile() (string, error) {
	return vkCookieHeaderFromStore()
}

func vkCookieHeaderFromStore() (string, error) {
	data, err := loadVKCookiesFromStore()
	if err != nil {
		return "", fmt.Errorf("cookies не найдены: %w", err)
	}
	var cookies []vkCookieEntry
	if err := json.Unmarshal(data, &cookies); err != nil {
		return "", fmt.Errorf("неверный JSON cookies: %w", err)
	}
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("cookies пусты")
	}
	return strings.Join(parts, "; "), nil
}

func vkHTTPPost(endpoint string, form url.Values, headers map[string]string) ([]byte, error) {
	return vkHTTPPostDo(endpoint, form, headers)
}

var vkHTTPPostDo = func(endpoint string, form url.Values, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", vkCallUserAgent)
	req.Header.Set("Origin", "https://"+vkCallWebHost)
	req.Header.Set("Referer", "https://"+vkCallWebHost+"/")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := vkCallHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func vkParseAPIError(body []byte) error {
	var wrap struct {
		Error vkAPIError `json:"error"`
	}
	if json.Unmarshal(body, &wrap) != nil || wrap.Error.Code == 0 {
		return nil
	}
	return fmt.Errorf("VK API %d: %s", wrap.Error.Code, wrap.Error.Message)
}

func vkWebToken(cookieHeader string) (string, error) {
	body, err := vkHTTPPost("https://login."+vkCallWebHost+"/?act=web_token",
		url.Values{"version": {"1"}, "app_id": {vkCallAppID}},
		map[string]string{"Cookie": cookieHeader})
	if err != nil {
		return "", fmt.Errorf("web_token: %w", err)
	}
	var tok struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("web_token parse: %w", err)
	}
	if tok.Data.AccessToken == "" {
		return "", fmt.Errorf("пустой VK token, ответ: %s", truncateBody(body, 300))
	}
	return tok.Data.AccessToken, nil
}

func vkCurrentUserID(token string) (string, error) {
	body, err := vkHTTPPost("https://api."+vkCallWebHost+"/method/users.get",
		url.Values{"v": {vkCallAPIVersion}},
		map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return "", fmt.Errorf("users.get: %w", err)
	}
	if err := vkParseAPIError(body); err != nil {
		return "", err
	}
	var resp struct {
		Response []struct {
			ID int64 `json:"id"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("users.get parse: %w", err)
	}
	if len(resp.Response) == 0 || resp.Response[0].ID == 0 {
		return "", fmt.Errorf("users.get: пустой id, ответ: %s", truncateBody(body, 300))
	}
	return fmt.Sprint(resp.Response[0].ID), nil
}

type vkCallCreateResult struct {
	CallID   string
	JoinLink string
}

func vkCreateCallLink(cookieHeader string) (vkCallCreateResult, error) {
	var out vkCallCreateResult
	token, err := vkWebToken(cookieHeader)
	if err != nil {
		return out, err
	}
	peerID, err := vkCurrentUserID(token)
	if err != nil {
		return out, err
	}
	body, err := vkHTTPPost("https://api."+vkCallWebHost+"/method/calls.start",
		url.Values{"v": {vkCallAPIVersion}, "peer_id": {peerID}},
		map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return out, fmt.Errorf("calls.start: %w", err)
	}
	if err := vkParseAPIError(body); err != nil {
		return out, err
	}
	var call struct {
		Response struct {
			CallID   string `json:"call_id"`
			JoinLink string `json:"join_link"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &call); err != nil {
		return out, fmt.Errorf("calls.start parse: %w", err)
	}
	if call.Response.CallID == "" {
		return out, fmt.Errorf("calls.start: пустой call_id, ответ: %s", truncateBody(body, 300))
	}
	if call.Response.JoinLink == "" {
		return out, fmt.Errorf("calls.start: пустой join_link, ответ: %s", truncateBody(body, 300))
	}
	out.CallID = call.Response.CallID
	out.JoinLink = call.Response.JoinLink
	return out, nil
}

func vkForceFinishCall(cookieHeader, callID string) error {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return fmt.Errorf("call_id пуст")
	}
	token, err := vkWebToken(cookieHeader)
	if err != nil {
		return err
	}
	body, err := vkHTTPPost("https://api."+vkCallWebHost+"/method/calls.forceFinish",
		url.Values{"v": {vkCallAPIVersion}, "call_id": {callID}},
		map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return fmt.Errorf("calls.forceFinish: %w", err)
	}
	if err := vkParseAPIError(body); err != nil {
		return err
	}
	var resp struct {
		Response int `json:"response"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("calls.forceFinish parse: %w", err)
	}
	if resp.Response != 1 {
		return fmt.Errorf("calls.forceFinish: неожиданный ответ %s", truncateBody(body, 300))
	}
	return nil
}

func vkAnonToken() (string, error) {
	body, err := vkHTTPPost("https://login."+vkCallWebHost+"/?act=get_anonym_token",
		url.Values{"client_id": {vkCallAppID}}, nil)
	if err != nil {
		return "", err
	}
	var tok struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", err
	}
	if tok.Data.AccessToken == "" {
		return "", fmt.Errorf("пустой anonym token")
	}
	return tok.Data.AccessToken, nil
}

func vkCallAlive(joinLink string) bool {
	joinLink = strings.TrimSpace(joinLink)
	if joinLink == "" {
		return false
	}
	token, err := vkAnonToken()
	if err != nil {
		return false
	}
	body, err := vkHTTPPost("https://api."+vkCallWebHost+"/method/calls.getCallPreview",
		url.Values{"v": {vkCallAPIVersion}, "vk_join_link": {joinLink}},
		map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return false
	}
	if vkParseAPIError(body) != nil {
		return false
	}
	var resp struct {
		Response struct {
			OKJoinLink string `json:"ok_join_link"`
		} `json:"response"`
	}
	if json.Unmarshal(body, &resp) != nil {
		return false
	}
	return resp.Response.OKJoinLink != ""
}

func truncateBody(b []byte, max int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
