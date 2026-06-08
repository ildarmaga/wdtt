package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var warpHTTPClient = &http.Client{Timeout: 30 * time.Second}

func getWarpData() (string, error) {
	meta := loadPanelXrayMeta()
	return meta.Warp, nil
}

func setWarpData(data string) error {
	meta := loadPanelXrayMeta()
	meta.Warp = data
	return savePanelXrayMeta(meta)
}

func delWarpData() error {
	return setWarpData("")
}

func getWarpConfig() (string, error) {
	warp, err := getWarpData()
	if err != nil || warp == "" {
		return "", fmt.Errorf("WARP не настроен")
	}
	var warpData map[string]string
	if err := json.Unmarshal([]byte(warp), &warpData); err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://api.cloudflareclient.com/v0a2158/reg/%s", warpData["device_id"])
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+warpData["access_token"])
	resp, err := warpHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Cloudflare API: %s", string(body))
	}
	return string(body), nil
}

func regWarp(secretKey, publicKey string) (string, error) {
	if secretKey == "" || publicKey == "" {
		return "", fmt.Errorf("privateKey и publicKey обязательны")
	}
	tos := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	hostName, _ := os.Hostname()
	payload := fmt.Sprintf(`{"key":"%s","tos":"%s","type":"PC","model":"wdtt-panel","name":"%s"}`, publicKey, tos, hostName)

	req, err := http.NewRequest(http.MethodPost, "https://api.cloudflareclient.com/v0a2158/reg", bytes.NewBufferString(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("CF-Client-Version", "a-7.21-0721")
	req.Header.Set("Content-Type", "application/json")

	resp, err := warpHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Cloudflare API: %s", string(body))
	}

	var rspData map[string]interface{}
	if err := json.Unmarshal(body, &rspData); err != nil {
		return "", err
	}
	deviceID, _ := rspData["id"].(string)
	token, _ := rspData["token"].(string)
	license := ""
	if account, ok := rspData["account"].(map[string]interface{}); ok {
		license, _ = account["license"].(string)
	}
	if deviceID == "" || token == "" {
		return "", fmt.Errorf("некорректный ответ Cloudflare WARP")
	}

	warpData := fmt.Sprintf("{\n  \"access_token\": \"%s\",\n  \"device_id\": \"%s\",\n  \"license_key\": \"%s\",\n  \"private_key\": \"%s\"\n}",
		token, deviceID, license, secretKey)
	if err := setWarpData(warpData); err != nil {
		return "", err
	}
	return fmt.Sprintf("{\n  \"data\": %s,\n  \"config\": %s\n}", warpData, string(body)), nil
}

func setWarpLicense(license string) (string, error) {
	warp, err := getWarpData()
	if err != nil || warp == "" {
		return "", fmt.Errorf("WARP не настроен")
	}
	var warpData map[string]string
	if err := json.Unmarshal([]byte(warp), &warpData); err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://api.cloudflareclient.com/v0a2158/reg/%s/account", warpData["device_id"])
	payload := fmt.Sprintf(`{"license": "%s"}`, license)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBufferString(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+warpData["access_token"])
	req.Header.Set("Content-Type", "application/json")

	resp, err := warpHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	if success, _ := response["success"].(bool); success == false {
		if errs, ok := response["errors"].([]interface{}); ok && len(errs) > 0 {
			if errObj, ok := errs[0].(map[string]interface{}); ok {
				code, _ := errObj["code"].(float64)
				msg, _ := errObj["message"].(string)
				return "", fmt.Errorf("%v: %s", code, msg)
			}
		}
		return "", fmt.Errorf("ошибка обновления лицензии WARP")
	}

	warpData["license_key"] = license
	newWarpData, err := json.MarshalIndent(warpData, "", "  ")
	if err != nil {
		return "", err
	}
	if err := setWarpData(string(newWarpData)); err != nil {
		return "", err
	}
	return string(newWarpData), nil
}

func handleWarpAPI(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	form, err := parsePostForm(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusOK)
		return
	}
	var resp string
	switch action {
	case "data":
		resp, err = getWarpData()
	case "del":
		err = delWarpData()
	case "config":
		resp, err = getWarpConfig()
	case "reg":
		resp, err = regWarp(form.Get("privateKey"), form.Get("publicKey"))
	case "license":
		resp, err = setWarpLicense(form.Get("license"))
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		jsonError(w, err.Error(), http.StatusOK)
		return
	}
	jsonOK(w, resp)
}
