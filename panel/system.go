package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

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
