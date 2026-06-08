package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func parsePostForm(r *http.Request) (url.Values, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return url.ParseQuery(string(body))
}

func jsonMsg(w http.ResponseWriter, msg string, ok bool) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": ok,
		"msg":     msg,
	})
}

func (a *App) serveXrayPage(w http.ResponseWriter, r *http.Request) {
	if a.parseSession(r) == nil {
		http.Redirect(w, r, a.cfg.basePath(), http.StatusFound)
		return
	}
	a.renderHTML(w, r, "xray.html", "pages.xray.title", nil)
}

func (a *App) handleXrayAPI(w http.ResponseWriter, r *http.Request) {
	prefix := a.cfg.basePath() + "panel/xray/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	sub := strings.TrimPrefix(r.URL.Path, prefix)
	sub = strings.Trim(sub, "/")

	switch {
	case sub == "" && r.Method == http.MethodPost:
		a.handleGetXraySetting(w, r)
	case sub == "update" && r.Method == http.MethodPost:
		a.handleUpdateXraySetting(w, r)
	case sub == "getOutboundsTraffic":
		traffic, err := getOutboundsTraffic()
		if err != nil {
			jsonError(w, err.Error(), http.StatusOK)
			return
		}
		jsonOK(w, traffic)
	case sub == "getXrayResult":
		jsonOK(w, getXrayRestartResult())
	case sub == "getDefaultJsonConfig":
		cfg, err := defaultXrayConfig()
		if err != nil {
			jsonError(w, err.Error(), http.StatusOK)
			return
		}
		jsonOK(w, cfg)
	case sub == "testOutbound" && r.Method == http.MethodPost:
		a.handleTestOutbound(w, r)
	case sub == "resetOutboundsTraffic" && r.Method == http.MethodPost:
		var req struct {
			Tag string `json:"tag"`
		}
		tag := ""
		if err := readJSON(r, &req); err == nil {
			tag = req.Tag
		} else if form, ferr := parsePostForm(r); ferr == nil {
			tag = form.Get("tag")
		}
		if err := resetOutboundStats(tag); err != nil {
			jsonError(w, err.Error(), http.StatusOK)
			return
		}
		jsonOK(w, "")
	case strings.HasPrefix(sub, "warp/"):
		handleWarpAPI(w, r, strings.TrimPrefix(sub, "warp/"))
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleGetXraySetting(w http.ResponseWriter, r *http.Request) {
	raw, err := loadXrayConfigRaw()
	if err != nil {
		jsonError(w, i18nWeb("pages.settings.toasts.getSettings")+": "+err.Error(), http.StatusOK)
		return
	}
	raw = unwrapXrayTemplateConfig(raw)
	raw, err = mergeXrayConfigDefaults(raw)
	if err != nil {
		jsonError(w, err.Error(), http.StatusOK)
		return
	}
	tags, err := getInboundTagsFromConfig(raw)
	if err != nil {
		jsonError(w, err.Error(), http.StatusOK)
		return
	}
	meta := loadPanelXrayMeta()
	tagsJSON, _ := json.Marshal(tags)
	resp := map[string]interface{}{
		"xraySetting":     json.RawMessage(raw),
		"inboundTags":     json.RawMessage(tagsJSON),
		"outboundTestUrl": meta.OutboundTestURL,
	}
	result, _ := json.Marshal(resp)
	jsonOK(w, string(result))
}

func (a *App) handleUpdateXraySetting(w http.ResponseWriter, r *http.Request) {
	form, err := parsePostForm(r)
	if err != nil {
		jsonError(w, i18nWeb("pages.login.toasts.invalidFormData"), http.StatusOK)
		return
	}
	xraySetting := form.Get("xraySetting")
	if xraySetting == "" {
		jsonError(w, i18nWeb("pages.settings.toasts.modifySettings"), http.StatusOK)
		return
	}
	xraySetting = unwrapXrayTemplateConfig(xraySetting)
	if err := saveXrayConfig(xraySetting); err != nil {
		jsonError(w, i18nWeb("pages.settings.toasts.modifySettings")+": "+err.Error(), http.StatusOK)
		return
	}
	meta := loadPanelXrayMeta()
	testURL := form.Get("outboundTestUrl")
	if testURL == "" {
		testURL = "https://www.google.com/generate_204"
	}
	meta.OutboundTestURL = testURL
	savePanelXrayMeta(meta)
	jsonMsg(w, i18nWeb("pages.settings.toasts.modifySettings"), true)
}

func (a *App) handleRestartXrayService(w http.ResponseWriter, r *http.Request) {
	if err := serviceRestart(xrayServiceUnit); err != nil {
		jsonMsg(w, i18nWeb("pages.xray.restartError")+": "+err.Error(), false)
		return
	}
	jsonMsg(w, i18nWeb("pages.xray.restartSuccess"), true)
}

func (a *App) handleDefaultJsonConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := defaultXrayConfig()
	if err != nil {
		jsonError(w, err.Error(), http.StatusOK)
		return
	}
	jsonOK(w, cfg)
}

func (a *App) handleCustomGeoAliases(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, []interface{}{})
}

func (a *App) handleTestOutbound(w http.ResponseWriter, r *http.Request) {
	form, err := parsePostForm(r)
	if err != nil {
		jsonOK(w, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	outboundJSON := form.Get("outbound")
	if outboundJSON == "" {
		jsonOK(w, map[string]interface{}{"success": false, "error": "outbound parameter is required"})
		return
	}
	var outbound map[string]interface{}
	if err := json.Unmarshal([]byte(outboundJSON), &outbound); err != nil {
		jsonOK(w, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	tag, _ := outbound["tag"].(string)
	if tag == "" {
		jsonOK(w, map[string]interface{}{"success": false, "error": "outbound tag is required"})
		return
	}
	allOutbounds := form.Get("allOutbounds")
	meta := loadPanelXrayMeta()
	result, err := testOutboundDelay(outbound, allOutbounds, meta.OutboundTestURL)
	if err != nil {
		jsonOK(w, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	jsonOK(w, result)
}

func processOutboundsForTest(testOutbound map[string]interface{}, allOutboundsJSON string) (string, error) {
	var allOutbounds []map[string]interface{}
	if allOutboundsJSON != "" {
		if err := json.Unmarshal([]byte(allOutboundsJSON), &allOutbounds); err != nil {
			return "", err
		}
	}
	if len(allOutbounds) == 0 {
		allOutbounds = []map[string]interface{}{testOutbound}
	}
	processed := make([]map[string]interface{}, len(allOutbounds))
	for i, ob := range allOutbounds {
		outbound := make(map[string]interface{}, len(ob))
		for k, v := range ob {
			outbound[k] = v
		}
		protocol, _ := outbound["protocol"].(string)
		if protocol == "wireguard" {
			var settings map[string]interface{}
			if existing, ok := outbound["settings"].(map[string]interface{}); ok {
				settings = make(map[string]interface{}, len(existing))
				for k, v := range existing {
					settings[k] = v
				}
			} else {
				settings = map[string]interface{}{}
			}
			settings["noKernelTun"] = true
			outbound["settings"] = settings
		}
		processed[i] = outbound
	}
	data, err := json.Marshal(processed)
	return string(data), err
}

func findAvailableTestPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func waitForTestPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("port %d not ready after %v", port, timeout)
}

func testProxyConnection(proxyPort int, testURL string) (int64, int, error) {
	proxyURL, err := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", proxyPort))
	if err != nil {
		return 0, 0, err
	}
	client := &http.Client{
		Timeout: 12 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:       1,
			IdleConnTimeout:    10 * time.Second,
			DisableCompression: true,
		},
	}
	warmup, err := client.Get(testURL)
	if err != nil {
		return 0, 0, fmt.Errorf("request failed: %v", err)
	}
	io.Copy(io.Discard, warmup.Body)
	warmup.Body.Close()

	start := time.Now()
	resp, err := client.Get(testURL)
	delay := time.Since(start).Milliseconds()
	if err != nil {
		return 0, 0, fmt.Errorf("request failed: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return delay, resp.StatusCode, nil
}

func testOutboundDelay(testOutbound map[string]interface{}, allOutboundsJSON, testURL string) (map[string]interface{}, error) {
	if testURL == "" {
		testURL = "https://www.google.com/generate_204"
	}
	bin := xrayBinary()
	if bin == "" {
		return nil, fmt.Errorf("xray binary не найден")
	}
	outboundTag, _ := testOutbound["tag"].(string)
	if outboundTag == "" {
		return map[string]interface{}{"success": false, "error": "outbound tag is required"}, nil
	}
	outboundsJSON, err := processOutboundsForTest(testOutbound, allOutboundsJSON)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}
	testPort, err := findAvailableTestPort()
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}
	cfg := fmt.Sprintf(`{
  "log": {"loglevel": "warning", "access": "none", "error": "none"},
  "inbounds": [{
    "tag": "test-in", "listen": "127.0.0.1", "port": %d,
    "protocol": "socks", "settings": {"auth": "noauth", "udp": true}
  }],
  "outbounds": %s,
  "routing": {
    "domainStrategy": "AsIs",
    "rules": [{"type": "field", "outboundTag": "%s", "network": "tcp,udp"}]
  }
}`, testPort, outboundsJSON, outboundTag)

	tmp, err := os.CreateTemp("", "wdtt-outbound-test-*.json")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	tmp.WriteString(cfg)
	tmp.Close()
	defer os.Remove(tmpPath)

	if out, err := runCmd(bin, "run", "-test", "-c", tmpPath); err != nil {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = err.Error()
		}
		return map[string]interface{}{"success": false, "error": "config test: " + msg}, nil
	}

	cmd := exec.Command(bin, "run", "-c", tmpPath)
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+xrayAssetDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stderrPipe, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		cmd.Wait()
	}()

	if err := waitForTestPort(testPort, 3*time.Second); err != nil {
		errOut, _ := io.ReadAll(stderrPipe)
		msg := strings.TrimSpace(string(errOut))
		if msg != "" {
			return map[string]interface{}{"success": false, "error": msg}, nil
		}
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}

	delay, code, err := testProxyConnection(testPort, testURL)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}
	return map[string]interface{}{
		"success":    true,
		"delay":      delay,
		"statusCode": code,
	}, nil
}
