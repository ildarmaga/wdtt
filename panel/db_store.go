package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	dbKeyXrayMeta        = "xray_meta"
	dbKeyXrayInboundMeta = "xray_inbound_meta"
	dbKeyPasswords       = "passwords"
	dbKeyInbound         = "inbound"
	dbKeyXrayTraffic     = "xray_traffic"
)

func dbSaveJSON(key string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return dbSettingSet(key, string(data))
}

func dbLoadJSON(key string, dest interface{}) (bool, error) {
	raw, ok, err := dbSettingGet(key)
	if err != nil {
		return false, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return false, nil
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return false, err
	}
	return true, nil
}

func writeJSONFile(path string, v interface{}, perm os.FileMode) error {
	if err := os.MkdirAll(filepathDir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func filepathDir(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return "."
}

func saveToDB(key string, v interface{}) error {
	if !panelDBEnabled() {
		return fmt.Errorf("panel database not available")
	}
	return saveToNormalized(key, v)
}

func saveToNormalized(key string, v interface{}) error {
	switch key {
	case panelDBSetting:
		cfg, ok := v.(*PanelConfig)
		if !ok {
			return fmt.Errorf("bad type for panel")
		}
		return savePanelConfigNorm(cfg)
	case dbKeyPasswords:
		db, ok := v.(*PasswordsDB)
		if !ok {
			return fmt.Errorf("bad type for passwords")
		}
		return savePasswordsNorm(db)
	case dbKeyInbound:
		cfg, ok := v.(WdttInboundConfig)
		if !ok {
			return fmt.Errorf("bad type for inbound")
		}
		return saveInboundNorm(cfg)
	case dbKeyXrayMeta:
		meta, ok := v.(panelXrayMeta)
		if !ok {
			return fmt.Errorf("bad type for xray meta")
		}
		return saveXrayMetaNorm(meta)
	case dbKeyXrayInboundMeta:
		meta, ok := v.(map[string]PanelXrayInboundMeta)
		if !ok {
			return fmt.Errorf("bad type for xray inbound meta")
		}
		return saveXrayInboundMetaNorm(meta)
	case dbKeyXrayTraffic:
		p, ok := v.(xrayTrafficPersist)
		if !ok {
			return fmt.Errorf("bad type for xray traffic")
		}
		return saveXrayTrafficNorm(p)
	default:
		return dbSaveJSON(key, v)
	}
}

func migrateJSONStoresToDB() {
	migrateLegacyJSONFiles()
}

func syncJSONFileToDB(key, path string, dest interface{}) {
	if !panelDBEnabled() {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return
	}
	if err := dbSaveJSON(key, dest); err != nil {
		log.Printf("panel db sync %s: %v", key, err)
		return
	}
	log.Printf("panel db: migrated %s from %s", key, path)
}
