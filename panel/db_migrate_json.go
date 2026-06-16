package panel

import (
	"encoding/json"
	"log"
	"os"
	"strings"
)

var legacyPasswordsPath = "/etc/wdtt/passwords.json"

// migrateLegacyJSONFiles импортирует старые JSON-конфиги в SQLite и удаляет их.
// /etc/wdtt-xray/config.json не трогаем — его читает процесс Xray.
func migrateLegacyJSONFiles() {
	if !panelDBEnabled() {
		return
	}
	migrateLegacyPanelJSON()
	migrateLegacyPasswordsJSON()
	migrateLegacyInboundJSON()
	migrateLegacyXrayMetaJSON()
	migrateLegacyXrayInboundMetaJSON()
	migrateLegacyXrayTrafficJSON()
}

func migrateLegacyPanelJSON() {
	if _, err := os.Stat(panelConfigPath); err != nil {
		return
	}
	defer removeLegacyJSONFile(panelConfigPath)
	if !tableHasRows(`SELECT COUNT(*) FROM panel_config`) {
		data, err := os.ReadFile(panelConfigPath)
		if err == nil {
			var cfg PanelConfig
			if json.Unmarshal(data, &cfg) == nil {
				normalizePanelConfig(&cfg)
				if err := savePanelConfigNorm(&cfg); err != nil {
					log.Printf("panel db: migrate panel.json: %v", err)
				} else {
					log.Printf("panel db: migrated %s → panel_config", panelConfigPath)
				}
			}
		}
	}
}

func migrateLegacyPasswordsJSON() {
	if _, err := os.Stat(legacyPasswordsPath); err != nil {
		return
	}
	if !tableHasRows(`SELECT COUNT(*) FROM wdtt_users`) {
		db, err := parsePasswordsJSONFile(legacyPasswordsPath)
		if err != nil {
			log.Printf("panel db: migrate passwords.json: %v", err)
			return
		}
		if err := savePasswordsNorm(db); err != nil {
			log.Printf("panel db: migrate passwords.json save: %v", err)
			return
		}
		log.Printf("panel db: migrated %s → wdtt_users (%d)", legacyPasswordsPath, len(db.Passwords))
	}
	removeLegacyJSONFile(legacyPasswordsPath)
}

func migrateLegacyInboundJSON() {
	if _, err := os.Stat(wdttInboundPath); err != nil {
		return
	}
	if !tableHasRows(`SELECT COUNT(*) FROM wdtt_inbound`) {
		data, err := os.ReadFile(wdttInboundPath)
		if err == nil {
			var cfg WdttInboundConfig
			if json.Unmarshal(data, &cfg) == nil {
				cfg.normalize()
				if err := saveInboundNorm(cfg); err != nil {
					log.Printf("panel db: migrate inbound.json: %v", err)
					return
				}
				log.Printf("panel db: migrated %s → wdtt_inbound", wdttInboundPath)
			}
		}
	}
	removeLegacyJSONFile(wdttInboundPath)
}

func migrateLegacyXrayMetaJSON() {
	if _, err := os.Stat(panelXrayMetaPath); err != nil {
		return
	}
	if !tableHasRows(`SELECT COUNT(*) FROM xray_panel_meta`) {
		data, err := os.ReadFile(panelXrayMetaPath)
		if err == nil {
			var meta panelXrayMeta
			if json.Unmarshal(data, &meta) == nil {
				if meta.OutboundTestURL == "" {
					meta.OutboundTestURL = "https://www.google.com/generate_204"
				}
				if err := saveXrayMetaNorm(meta); err != nil {
					log.Printf("panel db: migrate panel-xray.json: %v", err)
					return
				}
				log.Printf("panel db: migrated %s → xray_panel_meta", panelXrayMetaPath)
			}
		}
	}
	removeLegacyJSONFile(panelXrayMetaPath)
}

func migrateLegacyXrayInboundMetaJSON() {
	if _, err := os.Stat(panelXrayInboundMetaPath); err != nil {
		return
	}
	if !tableHasRows(`SELECT COUNT(*) FROM xray_inbound_meta`) {
		data, err := os.ReadFile(panelXrayInboundMetaPath)
		if err == nil {
			meta := map[string]PanelXrayInboundMeta{}
			if json.Unmarshal(data, &meta) == nil {
				if err := saveXrayInboundMetaNorm(meta); err != nil {
					log.Printf("panel db: migrate xray-inbounds-meta.json: %v", err)
					return
				}
				log.Printf("panel db: migrated %s → xray_inbound_meta (%d)", panelXrayInboundMetaPath, len(meta))
			}
		}
	}
	removeLegacyJSONFile(panelXrayInboundMetaPath)
}

func migrateLegacyXrayTrafficJSON() {
	path := xrayTrafficFile()
	if _, err := os.Stat(path); err != nil {
		return
	}
	if !tableHasRows(`SELECT COUNT(*) FROM xray_traffic_totals`) {
		data, err := os.ReadFile(path)
		if err == nil {
			var p xrayTrafficPersist
			if json.Unmarshal(data, &p) == nil {
				if err := saveXrayTrafficNorm(p); err != nil {
					log.Printf("panel db: migrate xray_traffic.json: %v", err)
					return
				}
				log.Printf("panel db: migrated %s → xray_traffic_totals", path)
			}
		}
	}
	removeLegacyJSONFile(path)
}

func removeLegacyJSONFile(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("panel db: remove legacy %s: %v", path, err)
		}
		return
	}
	log.Printf("panel db: removed legacy config %s", path)
}

func parsePasswordsJSONFile(path string) (*PasswordsDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePasswordsJSON(data)
}

func parsePasswordsJSON(data []byte) (*PasswordsDB, error) {
	var db PasswordsDB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, err
	}
	if db.Passwords == nil {
		db.Passwords = map[string]*PasswordEntry{}
	}
	if db.Devices == nil {
		db.Devices = map[string]*DeviceEntry{}
	}
	for _, entry := range db.Passwords {
		normalizeEntryDevices(entry)
	}
	dedupePasswordDeviceBindings(&db)
	return &db, nil
}

// ensureDefaultWdttData создаёт inbound и пустую БД пользователей при первой установке.
func ensureDefaultWdttData() {
	if !panelDBEnabled() {
		return
	}
	if !tableHasRows(`SELECT COUNT(*) FROM wdtt_inbound`) {
		cfg := defaultWdttInbound()
		applyDeployInboundDefaults(&cfg)
		if svc, err := parseWdttInboundFromService(); err == nil {
			svc.normalize()
			if svc.DtlsPort > 0 {
				cfg.DtlsPort = svc.DtlsPort
			}
			if svc.WgPort > 0 {
				cfg.WgPort = svc.WgPort
			}
			if strings.TrimSpace(svc.AdminAddr) != "" {
				cfg.AdminAddr = svc.AdminAddr
			}
		}
		if err := saveInboundNorm(cfg); err != nil {
			log.Printf("panel db: default inbound: %v", err)
		} else {
			log.Printf("panel db: seeded default wdtt_inbound (DTLS %d, WG %d)", cfg.DtlsPort, cfg.WgPort)
		}
	}
	if tableHasRows(`SELECT COUNT(*) FROM wdtt_users`) {
		return
	}
	db := &PasswordsDB{
		Passwords: map[string]*PasswordEntry{},
		Devices:   map[string]*DeviceEntry{},
	}
	if pass := passwordFromWdttService(); pass != "" {
		db.MainPassword = pass
	}
	if err := savePasswordsNorm(db); err != nil {
		log.Printf("panel db: seed users: %v", err)
		return
	}
	if db.MainPassword != "" {
		log.Printf("panel db: seeded wdtt_users (main password from wdtt.service)")
	}
}

func passwordFromWdttService() string {
	data, err := os.ReadFile(wdttServicePath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "ExecStart=") || strings.Contains(line, "ExecStartPre") {
			continue
		}
		if m := rePassword.FindStringSubmatch(line); len(m) == 2 {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}
