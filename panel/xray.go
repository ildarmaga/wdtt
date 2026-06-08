package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type XrayRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
}

var geoFiles = map[string]string{
	"geoip.dat":     "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat",
	"geosite.dat":   "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat",
	"geoip_IR.dat":  "https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geoip.dat",
	"geosite_IR.dat": "https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geosite.dat",
	"geoip_RU.dat":  "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat",
	"geosite_RU.dat": "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat",
}

func xrayBinary() string {
	p := filepath.Join(xrayBinDir, "xray-linux-amd64")
	if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
		return p
	}
	return ""
}

func xrayVersion() string {
	bin := xrayBinary()
	if bin == "" {
		return "не установлен"
	}
	out, _ := runCmd(bin, "version")
	if out == "" {
		return "unknown"
	}
	return strings.Split(out, "\n")[0]
}

func xrayVersionShort() string {
	v := strings.TrimPrefix(xrayVersion(), "Xray ")
	if i := strings.IndexByte(v, ' '); i > 0 {
		v = v[:i]
	}
	if v == "не установлен" || v == "unknown" {
		return "Unknown"
	}
	return v
}

func fetchXrayReleases() ([]XrayRelease, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/XTLS/Xray-core/releases?per_page=15")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var releases []XrayRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func xrayArchZip() string {
	switch runtime.GOARCH {
	case "arm64":
		return "Xray-linux-arm64-v8a.zip"
	case "arm":
		return "Xray-linux-arm32-v7a.zip"
	default:
		return "Xray-linux-64.zip"
	}
}

func installXrayVersion(tag string) error {
	os.MkdirAll(xrayBinDir, 0755)
	zipName := xrayArchZip()
	url := fmt.Sprintf("https://github.com/XTLS/Xray-core/releases/download/%s/%s", tag, zipName)
	tmpZip := filepath.Join(os.TempDir(), "xray-dl.zip")
	if err := downloadFile(url, tmpZip); err != nil {
		return err
	}
	defer os.Remove(tmpZip)

	serviceStop(xrayServiceUnit)

	zr, err := zip.OpenReader(tmpZip)
	if err != nil {
		return err
	}
	defer zr.Close()

	dest := filepath.Join(xrayBinDir, "xray-linux-amd64")
	for _, f := range zr.File {
		if f.Name == "xray" || strings.HasSuffix(f.Name, "/xray") {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				rc.Close()
				return err
			}
			_, err = io.Copy(out, rc)
			out.Close()
			rc.Close()
			if err != nil {
				return err
			}
			break
		}
	}
	serviceStart(xrayServiceUnit)
	return nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func updateGeofile(name string) error {
	url, ok := geoFiles[name]
	if !ok {
		return fmt.Errorf("неизвестный файл: %s", name)
	}
	os.MkdirAll(xrayAssetDir, 0755)
	dest := filepath.Join(xrayAssetDir, name)
	return downloadFile(url, dest)
}

func updateAllGeofiles() ([]string, error) {
	updated := []string{}
	for name := range geoFiles {
		if err := updateGeofile(name); err != nil {
			return updated, err
		}
		updated = append(updated, name)
	}
	serviceRestart(xrayServiceUnit)
	return updated, nil
}

func copyRegularFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func materializeAssetFile(dst string) {
	if fi, err := os.Lstat(dst); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(dst)
			if err != nil {
				return
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(dst), target)
			}
			os.Remove(dst)
			_ = copyRegularFile(target, dst, 0644)
		}
		return
	}
}

func ensureWdttXrayDirs() {
	os.MkdirAll(xrayBinDir, 0755)
	os.MkdirAll(xrayConfigDir, 0755)
	os.MkdirAll(xrayLogDir, 0755)

	for name := range geoFiles {
		dst := filepath.Join(xrayAssetDir, name)
		materializeAssetFile(dst)
		if _, err := os.Stat(dst); err != nil {
			_ = updateGeofile(name)
		}
	}

	ensureXrayLogPaths()
}

func defaultXrayAccessLog() string {
	return filepath.Join(xrayLogDir, "access.log")
}

func defaultXrayErrorLog() string {
	return filepath.Join(xrayLogDir, "error.log")
}

func normalizeXrayLogPath(key, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "none" {
		return value
	}
	if !filepath.IsAbs(value) {
		return filepath.Join(xrayLogDir, filepath.Base(value))
	}
	return value
}

func ensureXrayLogDefaults(cfg map[string]interface{}) {
	logCfg, ok := cfg["log"].(map[string]interface{})
	if !ok {
		logCfg = map[string]interface{}{}
		cfg["log"] = logCfg
	}
	if _, ok := logCfg["loglevel"]; !ok {
		logCfg["loglevel"] = "warning"
	}
	access, _ := logCfg["access"].(string)
	switch access {
	case "", "./access.log":
		logCfg["access"] = defaultXrayAccessLog()
	default:
		if norm := normalizeXrayLogPath("access", access); norm != access {
			logCfg["access"] = norm
		}
	}
	errLog, _ := logCfg["error"].(string)
	switch errLog {
	case "", "./error.log":
		logCfg["error"] = defaultXrayErrorLog()
	default:
		if norm := normalizeXrayLogPath("error", errLog); norm != errLog {
			logCfg["error"] = norm
		}
	}
}

func touchXrayLogFile(path string) {
	if path == "" || path == "none" {
		return
	}
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		f.Close()
	}
}

func ensureXrayLogFiles(cfg map[string]interface{}) {
	ensureXrayLogDefaults(cfg)
	logCfg, _ := cfg["log"].(map[string]interface{})
	if logCfg == nil {
		return
	}
	_ = os.MkdirAll(xrayLogDir, 0755)
	if p, _ := logCfg["access"].(string); p != "" {
		touchXrayLogFile(p)
	}
	if p, _ := logCfg["error"].(string); p != "" {
		touchXrayLogFile(p)
	}
}

func ensureXrayLogPaths() {
	data, err := os.ReadFile(xrayConfigPath)
	if err != nil {
		return
	}
	var cfg map[string]interface{}
	if json.Unmarshal(data, &cfg) != nil {
		return
	}
	before, _ := json.Marshal(cfg["log"])
	ensureXrayLogFiles(cfg)
	after, _ := json.Marshal(cfg["log"])
	if string(before) != string(after) {
		out, _ := json.MarshalIndent(cfg, "", "  ")
		_ = os.WriteFile(xrayConfigPath, out, 0644)
	}
}

func patchWdttXrayService() {
	unit := "/etc/systemd/system/" + xrayServiceUnit
	data, err := os.ReadFile(unit)
	if err != nil {
		return
	}
	content := string(data)
	newContent := content
	if !strings.Contains(newContent, "WorkingDirectory="+xrayLogDir) {
		newContent = strings.Replace(newContent,
			"LimitNOFILE=65535\n",
			"LimitNOFILE=65535\nWorkingDirectory="+xrayLogDir+"\n", 1)
	}
	if newContent != content {
		os.WriteFile(unit, []byte(newContent), 0644)
		runCmd("systemctl", "daemon-reload")
	}
}
