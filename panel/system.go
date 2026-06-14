package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

const wdttAdminReloadPath = "/admin/reload"

func wdttAdminReloadURL() string {
	cfg, err := loadWdttInbound()
	addr := "127.0.0.1:2861"
	if err == nil && strings.TrimSpace(cfg.AdminAddr) != "" {
		addr = strings.TrimSpace(cfg.AdminAddr)
	}
	return "http://" + addr + wdttAdminReloadPath
}

var xrayManuallyStopped atomic.Bool

func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func serviceActive(name string) bool {
	out, _ := runCmd("systemctl", "is-active", name)
	return out == "active"
}

func serviceEnabled(name string) bool {
	out, _ := runCmd("systemctl", "is-enabled", name)
	return out == "enabled"
}

func serviceUnitFailed(name string) bool {
	active, _ := runCmd("systemctl", "show", name, "-p", "ActiveState", "--value")
	if active == "failed" {
		return true
	}
	result, _ := runCmd("systemctl", "show", name, "-p", "Result", "--value")
	switch result {
	case "exit-code", "signal", "core-dump", "timeout", "resources":
		return true
	}
	return false
}

func serviceRestart(name string) error {
	_, err := runCmd("systemctl", "restart", name)
	return err
}

func serviceStop(name string) error {
	_, err := runCmd("systemctl", "stop", name)
	return err
}

func serviceStart(name string) error {
	_, err := runCmd("systemctl", "start", name)
	return err
}

func markXrayManuallyStopped() {
	xrayManuallyStopped.Store(true)
}

func markXrayAutoManaged() {
	xrayManuallyStopped.Store(false)
}

func waitServiceActive(name string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if serviceActive(name) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// ensureXrayFollowsWdtt поднимает Xray, если WDTT работает, а Xray случайно остался выключенным.
func ensureXrayFollowsWdtt() {
	if xrayManuallyStopped.Load() {
		return
	}
	if !serviceActive(wdttServiceUnit) || !serviceEnabled(xrayServiceUnit) {
		return
	}
	if serviceActive(xrayServiceUnit) || serviceUnitFailed(xrayServiceUnit) {
		return
	}
	log.Printf("[watchdog] WDTT активен, Xray выключен — запускаем")
	if err := serviceStart(xrayServiceUnit); err != nil {
		log.Printf("[watchdog] не удалось запустить Xray: %v", err)
	}
}

// refreshTunnelLocalServiceRules обновляет iptables для панели/подписки через VPN (порты из panel.db).
func refreshTunnelLocalServiceRules() {
	if _, err := os.Stat(wdttXrayRulesPath); err == nil {
		if _, err := runCmd("bash", wdttXrayRulesPath, "up"); err != nil {
			log.Printf("[panel] xray rules refresh: %v", err)
		}
	}
	if err := wdttHotReload(); err != nil {
		log.Printf("[panel] wdtt hot-reload after port change: %v", err)
	}
}

// wdttHotReload просит wdtt-server перечитать users/inbound из panel.db (fallback JSON) без перезапуска.
func wdttHotReload() error {
	if !serviceActive(wdttServiceUnit) {
		return fmt.Errorf("WDTT не запущен")
	}
	req, err := http.NewRequest(http.MethodPost, wdttAdminReloadURL(), strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg, err := loadPanelConfig(); err == nil && cfg != nil {
		if key := strings.TrimSpace(cfg.SessionKey); key != "" {
			req.Header.Set("X-WDTT-Admin-Token", key)
		}
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("admin reload HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// applyWdttConfigChange сохраняет конфиг и применяет hot-reload; при ошибке — полный restart.
func applyWdttConfigChange() error {
	if err := wdttHotReload(); err == nil {
		return nil
	} else {
		log.Printf("[panel] hot-reload не удался (%v), перезапуск WDTT", err)
	}
	return restartWdttWithDeps()
}

// restartWdttWithDeps перезапускает WDTT и гарантированно поднимает Xray, если он включён в systemd.
func restartWdttWithDeps() error {
	xrayWanted := serviceActive(xrayServiceUnit) || serviceEnabled(xrayServiceUnit)
	if xrayWanted {
		markXrayAutoManaged()
	}
	if err := serviceRestart(wdttServiceUnit); err != nil {
		return err
	}
	if !xrayWanted {
		return nil
	}
	if !waitServiceActive(wdttServiceUnit, 20*time.Second) {
		return fmt.Errorf("WDTT не поднялся после перезапуска")
	}
	if serviceActive(xrayServiceUnit) {
		return nil
	}
	if err := serviceStart(xrayServiceUnit); err != nil {
		return err
	}
	if !waitServiceActive(xrayServiceUnit, 20*time.Second) {
		return fmt.Errorf("Xray не поднялся после перезапуска WDTT")
	}
	return nil
}
