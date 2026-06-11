package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func xrayAPIAddInbound(inbound map[string]interface{}) error {
	bin := xrayBinary()
	if bin == "" {
		return fmt.Errorf("xray binary не найден")
	}
	if !serviceActive(xrayServiceUnit) {
		return fmt.Errorf("xray не запущен")
	}
	payload := map[string]interface{}{
		"inbounds": []interface{}{inbound},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	tmp := filepath.Join(os.TempDir(), "wdtt-xray-adi.json")
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	out, err := runCmd(bin, "api", "adi", "--server="+xrayAPIAddr(), tmp)
	if err != nil {
		if out != "" {
			return fmt.Errorf("%s", strings.TrimSpace(out))
		}
		return err
	}
	return nil
}

func xrayAPIRemoveInbound(tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return fmt.Errorf("пустой тег inbound")
	}
	bin := xrayBinary()
	if bin == "" {
		return fmt.Errorf("xray binary не найден")
	}
	if !serviceActive(xrayServiceUnit) {
		return nil
	}
	out, err := runCmd(bin, "api", "rmi", "--server="+xrayAPIAddr(), tag)
	if err != nil {
		if out != "" {
			return fmt.Errorf("%s", strings.TrimSpace(out))
		}
		return err
	}
	return nil
}

// applyPanelXrayInboundRuntime hot-applies inbound like 3x-ui (HandlerService).
// Falls back to caller restarting xray when this returns an error.
func applyPanelXrayInboundRuntime(isCreate, enable bool, inbound map[string]interface{}, tag string) error {
	if !enable {
		return xrayAPIRemoveInbound(tag)
	}
	if !isCreate {
		_ = xrayAPIRemoveInbound(tag)
	}
	return xrayAPIAddInbound(inbound)
}

func applyPanelXrayInboundWithFallback(isCreate, enable bool, inbound map[string]interface{}, tag string) {
	if err := applyPanelXrayInboundRuntime(isCreate, enable, inbound, tag); err != nil {
		log.Printf("xray hot-reload (%s): %v — перезапуск сервиса", tag, err)
		if rerr := serviceRestart(xrayServiceUnit); rerr != nil {
			log.Printf("xray restart after hot-reload failure: %v", rerr)
		}
	}
}
