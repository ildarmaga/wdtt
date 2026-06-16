package panel

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

const (
	legacySettingPanel          = "panel"
	legacySettingPasswords      = "passwords"
	legacySettingInbound        = "inbound"
	legacySettingXrayMeta       = "xray_meta"
	legacySettingXrayInboundMeta = "xray_inbound_meta"
	legacySettingXrayTraffic    = "xray_traffic"
)

var legacySettingKeys = []string{
	legacySettingPanel,
	legacySettingPasswords,
	legacySettingInbound,
	legacySettingXrayMeta,
	legacySettingXrayInboundMeta,
	legacySettingXrayTraffic,
}

func settingBlobGet(key string) (string, bool, error) {
	if panelDB == nil {
		return "", false, fmt.Errorf("database not initialized")
	}
	var value string
	err := panelDB.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func loadSettingJSON(key string, dest interface{}) (bool, error) {
	raw, ok, err := settingBlobGet(key)
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

func loadPanelConfigFromSettingsBlob() (*PanelConfig, error) {
	raw, ok, err := settingBlobGet(legacySettingPanel)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, os.ErrNotExist
	}
	var cfg PanelConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// migrateLegacySettingsBlobs импортирует JSON из settings.* в нормализованные таблицы,
// если целевая таблица ещё пуста.
func migrateLegacySettingsBlobs() error {
	has, err := paneldb.HasPanelConfig(panelDB)
	if err != nil {
		return err
	}
	if !has {
		if cfg, err := loadPanelConfigFromSettingsBlob(); err == nil {
			if err := savePanelConfigNorm(cfg); err != nil {
				return err
			}
			log.Printf("panel db: normalized panel_config from settings blob")
		}
	}

	hasUsers, err := paneldb.HasUsers(panelDB)
	if err != nil {
		return err
	}
	if !hasUsers {
		var pdb PasswordsDB
		if ok, _ := loadSettingJSON(legacySettingPasswords, &pdb); ok {
			if err := savePasswordsNorm(&pdb); err != nil {
				return err
			}
			log.Printf("panel db: normalized wdtt_users (%d) from settings blob", len(pdb.Passwords))
		}
	}

	hasInbound, err := paneldb.HasInbound(panelDB)
	if err != nil {
		return err
	}
	if !hasInbound {
		var inb WdttInboundConfig
		if ok, _ := loadSettingJSON(legacySettingInbound, &inb); ok {
			inb.normalize()
			if err := saveInboundNorm(inb); err != nil {
				return err
			}
			log.Printf("panel db: normalized wdtt_inbound from settings blob")
		}
	}

	hasXrayMeta, err := paneldb.HasXrayMeta(panelDB)
	if err != nil {
		return err
	}
	if !hasXrayMeta {
		var xm panelXrayMeta
		if ok, _ := loadSettingJSON(legacySettingXrayMeta, &xm); ok {
			if err := saveXrayMetaNorm(xm); err != nil {
				return err
			}
			log.Printf("panel db: normalized xray_panel_meta from settings blob")
		}
	}

	hasInboundMeta, err := paneldb.HasXrayInboundMeta(panelDB)
	if err != nil {
		return err
	}
	if !hasInboundMeta {
		meta := map[string]PanelXrayInboundMeta{}
		if ok, _ := loadSettingJSON(legacySettingXrayInboundMeta, &meta); ok {
			if err := saveXrayInboundMetaNorm(meta); err != nil {
				return err
			}
			log.Printf("panel db: normalized xray_inbound_meta (%d) from settings blob", len(meta))
		}
	}

	hasTraffic, err := paneldb.HasXrayTraffic(panelDB)
	if err != nil {
		return err
	}
	if !hasTraffic {
		var tr xrayTrafficPersist
		if ok, _ := loadSettingJSON(legacySettingXrayTraffic, &tr); ok {
			if err := saveXrayTrafficNorm(tr); err != nil {
				return err
			}
			log.Printf("panel db: normalized xray_traffic_totals from settings blob")
		}
	}
	return nil
}

func migratePanelDBV9() error {
	if err := migrateLegacySettingsBlobs(); err != nil {
		return err
	}
	return purgeLegacySettingsBlobs()
}

func migratePanelDBV10() error {
	if _, err := panelDB.Exec(`ALTER TABLE wdtt_global ADD COLUMN users_rev INTEGER NOT NULL DEFAULT 0`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return err
	}
	return nil
}

func migratePanelDBV11() error {
	for _, stmt := range []string{
		`ALTER TABLE wdtt_inbound ADD COLUMN wg_keepalive_sec INTEGER NOT NULL DEFAULT 25`,
		`ALTER TABLE wdtt_inbound ADD COLUMN stats_interval_sec INTEGER NOT NULL DEFAULT 2`,
		`ALTER TABLE panel_config ADD COLUMN dashboard_poll_sec INTEGER NOT NULL DEFAULT 2`,
		`ALTER TABLE panel_config ADD COLUMN users_poll_sec INTEGER NOT NULL DEFAULT 5`,
		`ALTER TABLE panel_config ADD COLUMN connections_poll_sec INTEGER NOT NULL DEFAULT 5`,
	} {
		if _, err := panelDB.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}

func purgeLegacySettingsBlobs() error {
	var removed int
	for _, key := range legacySettingKeys {
		res, err := panelDB.Exec(`DELETE FROM settings WHERE key = ?`, key)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			removed += int(n)
		}
	}
	if removed > 0 {
		log.Printf("panel db: removed %d legacy settings blob(s)", removed)
	}
	return nil
}
