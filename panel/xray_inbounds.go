package panel

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

const panelXrayInboundMetaPath = "/etc/wdtt/xray-inbounds-meta.json"

const panelProtocolMixed = "mixed"

type PanelXrayInboundMeta struct {
	Remark      string `json:"remark"`
	Enable      bool   `json:"enable"`
	Total       int64  `json:"total"`
	ExpiryTime  int64  `json:"expiryTime"`
	TrafficReset string `json:"trafficReset"`
}

type PanelXrayInboundRow struct {
	Tag      string                 `json:"tag"`
	Remark   string                 `json:"remark"`
	Enable   bool                   `json:"enable"`
	Protocol string                 `json:"protocol"`
	Listen   string                 `json:"listen"`
	Port     int                    `json:"port"`
	Settings map[string]interface{} `json:"settings,omitempty"`
	Sniffing map[string]interface{} `json:"sniffing,omitempty"`
	Total    int64                  `json:"total"`
	ExpiryTime int64                `json:"expiryTime"`
	TrafficReset string             `json:"trafficReset"`
}

type PanelXrayInboundSaveRequest struct {
	Create       bool                   `json:"create"`
	Tag          string                 `json:"tag"`
	Remark       string                 `json:"remark"`
	Enable       bool                   `json:"enable"`
	Protocol     string                 `json:"protocol"`
	Listen       string                 `json:"listen"`
	Port         int                    `json:"port"`
	Settings     map[string]interface{} `json:"settings"`
	Sniffing     map[string]interface{} `json:"sniffing"`
	Total        int64                  `json:"total"`
	ExpiryTime   int64                  `json:"expiryTime"`
	TrafficReset string                 `json:"trafficReset"`
}

func isProtectedInboundTag(tag string) bool {
	tag = strings.TrimSpace(tag)
	for _, t := range protectedInboundTags {
		if tag == t {
			return true
		}
	}
	return tag == "api"
}

func loadPanelXrayInboundMeta() map[string]PanelXrayInboundMeta {
	if panelDBEnabled() {
		if out, ok, err := loadXrayInboundMetaNorm(); err != nil {
			log.Printf("panel db xray_inbound_meta: %v", err)
		} else if ok {
			return out
		}
	}
	out := map[string]PanelXrayInboundMeta{}
	data, err := os.ReadFile(panelXrayInboundMetaPath)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	if panelDBEnabled() {
		_ = saveXrayInboundMetaNorm(out)
	}
	return out
}

func savePanelXrayInboundMeta(meta map[string]PanelXrayInboundMeta) error {
	return saveXrayInboundMetaNorm(meta)
}

func listPanelXrayInbounds() ([]PanelXrayInboundRow, error) {
	raw, err := loadXrayConfigRaw()
	if err != nil {
		return nil, err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	meta := loadPanelXrayInboundMeta()
	inbounds, _ := cfg["inbounds"].([]interface{})
	rows := make([]PanelXrayInboundRow, 0)
	for _, ib := range inbounds {
		m, ok := ib.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := m["tag"].(string)
		if tag == "" || isProtectedInboundTag(tag) {
			continue
		}
		protocol, _ := m["protocol"].(string)
		listen, _ := m["listen"].(string)
		port := intFromAny(m["port"])
		settings, _ := m["settings"].(map[string]interface{})
		sniffing, _ := m["sniffing"].(map[string]interface{})
		row := PanelXrayInboundRow{
			Tag:      tag,
			Remark:   tag,
			Enable:   true,
			Protocol: protocol,
			Listen:   listen,
			Port:     port,
			Settings: settings,
			Sniffing: sniffing,
		}
		if md, ok := meta[tag]; ok {
			if md.Remark != "" {
				row.Remark = md.Remark
			}
			row.Enable = md.Enable
			row.Total = md.Total
			row.ExpiryTime = md.ExpiryTime
			row.TrafficReset = md.TrafficReset
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func intFromAny(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func savePanelXrayInbound(req PanelXrayInboundSaveRequest) error {
	tag := strings.TrimSpace(req.Tag)
	if tag == "" {
		return fmt.Errorf("укажите тег inbound")
	}
	if isProtectedInboundTag(tag) {
		return fmt.Errorf("зарезервированный тег: %s", tag)
	}
	protocol := strings.ToLower(strings.TrimSpace(req.Protocol))
	if protocol == "" || protocol == "wdtt" {
		return fmt.Errorf("протокол wdtt настраивается отдельно")
	}
	if req.Port < 1 || req.Port > 65535 {
		return fmt.Errorf("порт должен быть от 1 до 65535")
	}
	listen := strings.TrimSpace(req.Listen)
	if !req.Enable {
		listen = "127.0.0.1"
	} else if listen == "" {
		listen = "0.0.0.0"
	}

	raw, err := loadXrayConfigRaw()
	if err != nil {
		return err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return err
	}
	inbounds, _ := cfg["inbounds"].([]interface{})
	filtered := make([]interface{}, 0, len(inbounds))
	found := false
	for _, ib := range inbounds {
		m, ok := ib.(map[string]interface{})
		if !ok {
			filtered = append(filtered, ib)
			continue
		}
		if ibTag, _ := m["tag"].(string); ibTag == tag {
			found = true
			continue
		}
		filtered = append(filtered, ib)
	}
	if req.Create && found {
		return fmt.Errorf("inbound с тегом %s уже существует", tag)
	}

	settings := req.Settings
	if settings == nil {
		settings = map[string]interface{}{}
	}
	if protocol == panelProtocolMixed {
		if _, ok := settings["auth"]; !ok {
			settings["auth"] = "noauth"
		}
		if _, ok := settings["udp"]; !ok {
			settings["udp"] = true
		}
		if _, ok := settings["ip"]; !ok {
			settings["ip"] = "127.0.0.1"
		}
	}

	sniffing := req.Sniffing
	if sniffing == nil {
		sniffing = map[string]interface{}{
			"enabled":      false,
			"destOverride": []interface{}{"http", "tls", "quic", "fakedns"},
		}
	}

	inboundEntry := map[string]interface{}{
		"listen":   listen,
		"port":     req.Port,
		"protocol": protocol,
		"tag":      tag,
		"settings": settings,
		"sniffing": sniffing,
	}

	filtered = append(filtered, inboundEntry)

	cfg["inbounds"] = filtered
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := writeXrayConfig(string(out)); err != nil {
		return err
	}

	meta := loadPanelXrayInboundMeta()
	meta[tag] = PanelXrayInboundMeta{
		Remark:       strings.TrimSpace(req.Remark),
		Enable:       req.Enable,
		Total:        req.Total,
		ExpiryTime:   req.ExpiryTime,
		TrafficReset: req.TrafficReset,
	}
	if err := savePanelXrayInboundMeta(meta); err != nil {
		return err
	}

	isCreate := req.Create && !found
	applyPanelXrayInboundWithFallback(isCreate, req.Enable, inboundEntry, tag)
	return nil
}

func deletePanelXrayInbound(tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return fmt.Errorf("укажите тег")
	}
	if isProtectedInboundTag(tag) {
		return fmt.Errorf("нельзя удалить системный inbound")
	}
	raw, err := loadXrayConfigRaw()
	if err != nil {
		return err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return err
	}
	inbounds, _ := cfg["inbounds"].([]interface{})
	filtered := make([]interface{}, 0, len(inbounds))
	removed := false
	for _, ib := range inbounds {
		m, ok := ib.(map[string]interface{})
		if ok {
			if ibTag, _ := m["tag"].(string); ibTag == tag {
				removed = true
				continue
			}
		}
		filtered = append(filtered, ib)
	}
	if !removed {
		return fmt.Errorf("inbound не найден")
	}
	cfg["inbounds"] = filtered
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := writeXrayConfig(string(out)); err != nil {
		return err
	}
	meta := loadPanelXrayInboundMeta()
	delete(meta, tag)
	if err := savePanelXrayInboundMeta(meta); err != nil {
		return err
	}
	if err := xrayAPIRemoveInbound(tag); err != nil {
		log.Printf("xray hot-remove (%s): %v — перезапуск сервиса", tag, err)
		if rerr := serviceRestart(xrayServiceUnit); rerr != nil {
			log.Printf("xray restart after hot-remove failure: %v", rerr)
		}
	}
	return nil
}
