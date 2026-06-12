package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	protectedInboundDNS = "dns-in"
	protectedInboundTCP = "redirect-in"
)

var protectedInboundTags = []string{protectedInboundTCP, protectedInboundDNS}

type panelXrayMeta struct {
	OutboundTestURL string `json:"outbound_test_url"`
	Warp            string `json:"warp"`
}

func loadPanelXrayMeta() panelXrayMeta {
	if panelDBEnabled() {
		if meta, ok, err := loadXrayMetaNorm(); err != nil {
			log.Printf("panel db xray_meta: %v", err)
		} else if ok {
			return meta
		}
	}
	meta := panelXrayMeta{OutboundTestURL: "https://www.google.com/generate_204"}
	data, err := os.ReadFile(panelXrayMetaPath)
	if err != nil {
		return meta
	}
	json.Unmarshal(data, &meta)
	if meta.OutboundTestURL == "" {
		meta.OutboundTestURL = "https://www.google.com/generate_204"
	}
	if panelDBEnabled() {
		_ = saveXrayMetaNorm(meta)
	}
	return meta
}

func savePanelXrayMeta(meta panelXrayMeta) error {
	return saveToDB(dbKeyXrayMeta, meta)
}

func loadXrayConfigRaw() (string, error) {
	if panelDBEnabled() {
		if raw, ok, err := loadXrayConfigNorm(); err != nil {
			log.Printf("panel db xray config load: %v, fallback file", err)
		} else if ok {
			return raw, nil
		}
	}
	data, err := os.ReadFile(xrayConfigPath)
	if err != nil {
		return "", err
	}
	raw := string(data)
	if panelDBEnabled() {
		_ = saveXrayConfigNorm(raw)
	}
	return raw, nil
}

func persistXrayConfigRaw(raw string) error {
	if panelDBEnabled() {
		if err := saveXrayConfigNorm(raw); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(xrayConfigPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(xrayConfigPath, []byte(raw), 0644)
}

func unwrapXrayTemplateConfig(raw string) string {
	const maxDepth = 8
	for i := 0; i < maxDepth; i++ {
		var top map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &top); err != nil {
			return raw
		}
		inner, ok := top["xraySetting"]
		if !ok {
			return raw
		}
		for _, k := range []string{"inbounds", "outbounds", "routing", "api", "dns", "log", "policy", "stats"} {
			if _, hit := top[k]; hit {
				return raw
			}
		}
		unwrapped := string(inner)
		var asStr string
		if err := json.Unmarshal(inner, &asStr); err == nil {
			unwrapped = asStr
		}
		raw = unwrapped
	}
	return raw
}

func getInboundTagsFromConfig(raw string) ([]string, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	inbounds, _ := cfg["inbounds"].([]interface{})
	tags := make([]string, 0, len(inbounds))
	for _, ib := range inbounds {
		m, _ := ib.(map[string]interface{})
		if tag, ok := m["tag"].(string); ok && tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

func preserveProtectedInbounds(oldRaw, newRaw string) (string, error) {
	var oldCfg, newCfg map[string]interface{}
	if err := json.Unmarshal([]byte(oldRaw), &oldCfg); err != nil {
		return newRaw, nil
	}
	if err := json.Unmarshal([]byte(newRaw), &newCfg); err != nil {
		return "", err
	}

	oldInbounds, _ := oldCfg["inbounds"].([]interface{})
	newInbounds, _ := newCfg["inbounds"].([]interface{})
	byTag := map[string]interface{}{}
	for _, ib := range newInbounds {
		m, _ := ib.(map[string]interface{})
		if tag, ok := m["tag"].(string); ok {
			byTag[tag] = m
		}
	}
	for _, ib := range oldInbounds {
		m, _ := ib.(map[string]interface{})
		tag, _ := m["tag"].(string)
		for _, pt := range protectedInboundTags {
			if tag == pt {
				byTag[tag] = m
			}
		}
	}
	merged := make([]interface{}, 0, len(byTag))
	// keep order: protected first, then rest
	for _, pt := range protectedInboundTags {
		if v, ok := byTag[pt]; ok {
			merged = append(merged, v)
			delete(byTag, pt)
		}
	}
	for _, v := range byTag {
		merged = append(merged, v)
	}
	newCfg["inbounds"] = merged
	out, err := json.MarshalIndent(newCfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func validateXrayConfig(raw string) error {
	bin := xrayBinary()
	if bin == "" {
		return fmt.Errorf("xray binary не найден")
	}
	tmp := filepath.Join(os.TempDir(), "wdtt-xray-test.json")
	if err := os.WriteFile(tmp, []byte(raw), 0600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	out, err := runCmd(bin, "run", "-test", "-c", tmp)
	if err != nil {
		if out != "" {
			return fmt.Errorf("%s", strings.TrimSpace(out))
		}
		return err
	}
	return nil
}

func writeXrayConfig(raw string) error {
	oldRaw, _ := loadXrayConfigRaw()
	merged, err := preserveProtectedInbounds(oldRaw, raw)
	if err != nil {
		return err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(merged), &cfg); err != nil {
		return err
	}
	ensureXrayLogFiles(cfg)
	mergedBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	merged = string(mergedBytes)
	if err := validateXrayConfig(merged); err != nil {
		return err
	}
	return persistXrayConfigRaw(merged)
}

func saveXrayConfig(raw string) error {
	if err := writeXrayConfig(raw); err != nil {
		return err
	}
	return serviceRestart(xrayServiceUnit)
}

func defaultXrayPolicy() map[string]interface{} {
	return map[string]interface{}{
		"levels": map[string]interface{}{
			"0": map[string]interface{}{
				"statsUserDownlink": true,
				"statsUserUplink":   true,
			},
		},
		"system": map[string]interface{}{
			"statsInboundDownlink":  true,
			"statsInboundUplink":    true,
			"statsOutboundDownlink": false,
			"statsOutboundUplink":   false,
		},
	}
}

func ensureXrayStatsAPI(cfg map[string]interface{}) {
	if cfg["stats"] == nil {
		cfg["stats"] = map[string]interface{}{}
	}
	if cfg["api"] == nil {
		cfg["api"] = map[string]interface{}{
			"tag": "api",
			"services": []interface{}{
				"HandlerService",
				"LoggerService",
				"StatsService",
			},
		}
	}

	inbounds, _ := cfg["inbounds"].([]interface{})
	hasAPIInbound := false
	for _, ib := range inbounds {
		m, _ := ib.(map[string]interface{})
		if tag, _ := m["tag"].(string); tag == "api" {
			hasAPIInbound = true
			break
		}
	}
	if !hasAPIInbound {
		cfg["inbounds"] = append(inbounds, map[string]interface{}{
			"listen":   "127.0.0.1",
			"port":     defaultXrayAPIPort,
			"protocol": "tunnel",
			"settings": map[string]interface{}{
				"rewriteAddress": "127.0.0.1",
			},
			"tag": "api",
		})
	}

	routing, _ := cfg["routing"].(map[string]interface{})
	if routing == nil {
		routing = map[string]interface{}{}
		cfg["routing"] = routing
	}
	rules, _ := routing["rules"].([]interface{})
	hasAPIRule := false
	for _, r := range rules {
		m, _ := r.(map[string]interface{})
		tags, _ := m["inboundTag"].([]interface{})
		for _, t := range tags {
			if s, _ := t.(string); s == "api" {
				hasAPIRule = true
				break
			}
		}
	}
	if !hasAPIRule {
		apiRule := map[string]interface{}{
			"type":        "field",
			"inboundTag":  []interface{}{"api"},
			"outboundTag": "api",
		}
		routing["rules"] = append([]interface{}{apiRule}, rules...)
	}
}

func mergeXrayConfigDefaults(raw string) (string, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, err
	}
	policy, _ := cfg["policy"].(map[string]interface{})
	if policy == nil {
		cfg["policy"] = defaultXrayPolicy()
	} else if _, ok := policy["system"]; !ok {
		def := defaultXrayPolicy()
		policy["system"] = def["system"]
	}
	ensureXrayStatsAPI(cfg)
	ensureXrayLogFiles(cfg)
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return raw, err
	}
	return string(out), nil
}

func patchXrayStatsAPIOnDisk() error {
	raw, err := loadXrayConfigRaw()
	if err != nil {
		return err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return err
	}
	before, _ := json.Marshal(cfg)
	ensureXrayStatsAPI(cfg)
	ensureXrayLogFiles(cfg)
	after, _ := json.Marshal(cfg)
	if string(before) == string(after) {
		return nil
	}
	merged, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := validateXrayConfig(string(merged)); err != nil {
		return err
	}
	if err := persistXrayConfigRaw(string(merged)); err != nil {
		return err
	}
	return serviceRestart(xrayServiceUnit)
}

func getXrayRestartResult() string {
	if serviceActive(xrayServiceUnit) {
		return ""
	}
	if data, err := os.ReadFile(filepath.Join(xrayLogDir, "error.log")); err == nil {
		lines := strings.Split(string(data), "\n")
		errLines := make([]string, 0, 8)
		for i := len(lines) - 1; i >= 0 && len(errLines) < 8; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			lower := strings.ToLower(line)
			if strings.Contains(lower, "[error]") || strings.Contains(lower, "failed") || strings.Contains(lower, "fatal") {
				errLines = append([]string{line}, errLines...)
			}
		}
		if len(errLines) > 0 {
			return strings.Join(errLines, "\n")
		}
	}
	out, _ := runCmd("journalctl", "-u", xrayServiceUnit, "-n", "20", "--no-pager", "-o", "cat", "-p", "err")
	if msg := strings.TrimSpace(out); msg != "" {
		return msg
	}
	out, _ = runCmd("journalctl", "-u", xrayServiceUnit, "-n", "15", "--no-pager", "-o", "cat")
	return strings.TrimSpace(out)
}

func defaultXrayConfig() (map[string]interface{}, error) {
	raw, err := loadXrayConfigRaw()
	if err != nil {
		cfg := map[string]interface{}{
			"log":       map[string]interface{}{"loglevel": "warning"},
			"inbounds":  []interface{}{},
			"outbounds": []interface{}{},
		}
		ensureXrayLogFiles(cfg)
		return cfg, nil
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
