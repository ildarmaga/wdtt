package paneldb

import (
	"database/sql"
	"fmt"
	"strings"
)

const DefaultXrayOutboundTestURL = "https://www.google.com/generate_204"

// XrayMeta — строка xray_panel_meta (id=1).
type XrayMeta struct {
	OutboundTestURL string
	Warp            string
}

// XrayInboundMeta — метаданные inbound (таблица xray_inbound_meta, ключ — tag).
type XrayInboundMeta struct {
	Remark       string
	Enable       bool
	Total        int64
	ExpiryTime   int64
	TrafficReset string
}

// XrayTrafficTotals — строка xray_traffic_totals (id=1).
type XrayTrafficTotals struct {
	Up   int64
	Down int64
}

func normalizeXrayMeta(meta *XrayMeta) {
	if meta == nil {
		return
	}
	if strings.TrimSpace(meta.OutboundTestURL) == "" {
		meta.OutboundTestURL = DefaultXrayOutboundTestURL
	}
}

// HasXrayMeta — есть ли строка в xray_panel_meta.
func HasXrayMeta(db *sql.DB) (bool, error) {
	n, err := tableCount(db, "xray_panel_meta")
	return n > 0, err
}

// LoadXrayMeta читает xray_panel_meta WHERE id = 1.
func LoadXrayMeta(db *sql.DB) (XrayMeta, bool, error) {
	if db == nil {
		return XrayMeta{}, false, fmt.Errorf("nil db")
	}
	has, err := HasXrayMeta(db)
	if err != nil || !has {
		return XrayMeta{OutboundTestURL: DefaultXrayOutboundTestURL}, false, err
	}
	var meta XrayMeta
	err = db.QueryRow(`SELECT outbound_test_url, warp FROM xray_panel_meta WHERE id = 1`).Scan(
		&meta.OutboundTestURL, &meta.Warp,
	)
	if err == sql.ErrNoRows {
		return XrayMeta{OutboundTestURL: DefaultXrayOutboundTestURL}, false, nil
	}
	if err != nil {
		return XrayMeta{}, false, err
	}
	normalizeXrayMeta(&meta)
	return meta, true, nil
}

// SaveXrayMeta upsert xray_panel_meta (id=1).
func SaveXrayMeta(db *sql.DB, meta XrayMeta) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	normalizeXrayMeta(&meta)
	_, err := db.Exec(`INSERT INTO xray_panel_meta (id, outbound_test_url, warp) VALUES (1,?,?)
		ON CONFLICT(id) DO UPDATE SET outbound_test_url=excluded.outbound_test_url, warp=excluded.warp`,
		meta.OutboundTestURL, meta.Warp)
	return err
}

// HasXrayInboundMeta — есть ли строки в xray_inbound_meta.
func HasXrayInboundMeta(db *sql.DB) (bool, error) {
	n, err := tableCount(db, "xray_inbound_meta")
	return n > 0, err
}

// LoadXrayInboundMeta читает все строки xray_inbound_meta.
func LoadXrayInboundMeta(db *sql.DB) (map[string]XrayInboundMeta, bool, error) {
	if db == nil {
		return nil, false, fmt.Errorf("nil db")
	}
	has, err := HasXrayInboundMeta(db)
	if err != nil || !has {
		return map[string]XrayInboundMeta{}, false, err
	}
	rows, err := db.Query(`SELECT tag, remark, enable, total, expiry_time, traffic_reset FROM xray_inbound_meta`)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := map[string]XrayInboundMeta{}
	for rows.Next() {
		var tag string
		var m XrayInboundMeta
		var en int
		if err := rows.Scan(&tag, &m.Remark, &en, &m.Total, &m.ExpiryTime, &m.TrafficReset); err != nil {
			return nil, false, err
		}
		m.Enable = en != 0
		out[tag] = m
	}
	return out, true, rows.Err()
}

// SaveXrayInboundMeta полностью перезаписывает xray_inbound_meta.
func SaveXrayInboundMeta(db *sql.DB, meta map[string]XrayInboundMeta) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM xray_inbound_meta`); err != nil {
		return err
	}
	for tag, m := range meta {
		en := 0
		if m.Enable {
			en = 1
		}
		if _, err := tx.Exec(`INSERT INTO xray_inbound_meta (tag, remark, enable, total, expiry_time, traffic_reset)
			VALUES (?,?,?,?,?,?)`, tag, m.Remark, en, m.Total, m.ExpiryTime, m.TrafficReset); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// HasXrayTraffic — есть ли строка в xray_traffic_totals.
func HasXrayTraffic(db *sql.DB) (bool, error) {
	n, err := tableCount(db, "xray_traffic_totals")
	return n > 0, err
}

// LoadXrayTraffic читает xray_traffic_totals WHERE id = 1.
func LoadXrayTraffic(db *sql.DB) (XrayTrafficTotals, bool, error) {
	if db == nil {
		return XrayTrafficTotals{}, false, fmt.Errorf("nil db")
	}
	has, err := HasXrayTraffic(db)
	if err != nil || !has {
		return XrayTrafficTotals{}, false, err
	}
	var p XrayTrafficTotals
	err = db.QueryRow(`SELECT up, down FROM xray_traffic_totals WHERE id = 1`).Scan(&p.Up, &p.Down)
	if err == sql.ErrNoRows {
		return XrayTrafficTotals{}, false, nil
	}
	if err != nil {
		return XrayTrafficTotals{}, false, err
	}
	return p, true, nil
}

// SaveXrayTraffic upsert xray_traffic_totals (id=1).
func SaveXrayTraffic(db *sql.DB, p XrayTrafficTotals) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	_, err := db.Exec(`INSERT INTO xray_traffic_totals (id, up, down) VALUES (1,?,?)
		ON CONFLICT(id) DO UPDATE SET up=excluded.up, down=excluded.down`, p.Up, p.Down)
	return err
}

// LoadXrayConfig читает raw_json из xray_config WHERE id = 1.
func LoadXrayConfig(db *sql.DB) (string, bool, error) {
	if db == nil {
		return "", false, fmt.Errorf("nil db")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM xray_config WHERE length(trim(raw_json)) > 0`).Scan(&n); err != nil {
		return "", false, err
	}
	if n == 0 {
		return "", false, nil
	}
	var raw string
	err := db.QueryRow(`SELECT raw_json FROM xray_config WHERE id = 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(raw) == "" {
		return "", false, nil
	}
	return raw, true, nil
}

// SaveXrayConfig upsert xray_config (id=1).
func SaveXrayConfig(db *sql.DB, raw string) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	_, err := db.Exec(`INSERT INTO xray_config (id, raw_json) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET raw_json = excluded.raw_json`, raw)
	return err
}
