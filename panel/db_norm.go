package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

const dbSchemaVersion = 4

const schemaV2DDL = `
CREATE TABLE IF NOT EXISTS panel_config (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	username TEXT NOT NULL DEFAULT 'admin',
	password_hash TEXT NOT NULL DEFAULT '',
	port INTEGER NOT NULL DEFAULT 2860,
	web_base_path TEXT NOT NULL DEFAULT '/wdtt/',
	session_key TEXT NOT NULL DEFAULT '',
	web_listen TEXT NOT NULL DEFAULT '',
	web_domain TEXT NOT NULL DEFAULT '',
	web_cert_file TEXT NOT NULL DEFAULT '',
	web_key_file TEXT NOT NULL DEFAULT '',
	session_max_age INTEGER NOT NULL DEFAULT 60,
	page_size INTEGER NOT NULL DEFAULT 50,
	remark_model TEXT NOT NULL DEFAULT '-ieo',
	block_ping INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS wdtt_global (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	main_password TEXT NOT NULL DEFAULT '',
	admin_id TEXT NOT NULL DEFAULT '',
	bot_token TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS wdtt_users (
	password TEXT PRIMARY KEY,
	device_id TEXT NOT NULL DEFAULT '',
	max_devices INTEGER NOT NULL DEFAULT 0,
	expires_at INTEGER NOT NULL DEFAULT 0,
	down_bytes INTEGER NOT NULL DEFAULT 0,
	up_bytes INTEGER NOT NULL DEFAULT 0,
	total_bytes INTEGER NOT NULL DEFAULT 0,
	max_down_mbps REAL NOT NULL DEFAULT 0,
	max_up_mbps REAL NOT NULL DEFAULT 0,
	is_deactivated INTEGER NOT NULL DEFAULT 0,
	comment TEXT NOT NULL DEFAULT '',
	ports TEXT NOT NULL DEFAULT '',
	vk_hash TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS wdtt_user_devices (
	password TEXT NOT NULL,
	device_id TEXT NOT NULL,
	sort_order INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (password, device_id),
	FOREIGN KEY (password) REFERENCES wdtt_users(password) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS wdtt_devices (
	device_id TEXT PRIMARY KEY,
	ip TEXT NOT NULL DEFAULT '',
	priv_key TEXT NOT NULL DEFAULT '',
	pub_key TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS wdtt_inbound (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	tag TEXT NOT NULL DEFAULT 'wdtt-in',
	remark TEXT NOT NULL DEFAULT 'WDTT',
	enable INTEGER NOT NULL DEFAULT 1,
	listen_host TEXT NOT NULL DEFAULT '0.0.0.0',
	server_host TEXT NOT NULL DEFAULT '',
	dtls_port INTEGER NOT NULL DEFAULT 56000,
	wg_port INTEGER NOT NULL DEFAULT 56001,
	client_port INTEGER NOT NULL DEFAULT 9000,
	dns TEXT NOT NULL DEFAULT '1.1.1.1',
	mtu INTEGER NOT NULL DEFAULT 1280,
	max_users INTEGER NOT NULL DEFAULT 10,
	handshake_timeout_sec INTEGER NOT NULL DEFAULT 30,
	max_dtls_per_device INTEGER NOT NULL DEFAULT 0,
	admin_addr TEXT NOT NULL DEFAULT '127.0.0.1:2861'
);
CREATE TABLE IF NOT EXISTS xray_panel_meta (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	outbound_test_url TEXT NOT NULL DEFAULT 'https://www.google.com/generate_204',
	warp TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS xray_inbound_meta (
	tag TEXT PRIMARY KEY,
	remark TEXT NOT NULL DEFAULT '',
	enable INTEGER NOT NULL DEFAULT 1,
	total INTEGER NOT NULL DEFAULT 0,
	expiry_time INTEGER NOT NULL DEFAULT 0,
	traffic_reset TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS xray_traffic_totals (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	up INTEGER NOT NULL DEFAULT 0,
	down INTEGER NOT NULL DEFAULT 0
);
`

func migratePanelDBV2() error {
	if _, err := panelDB.Exec(schemaV2DDL); err != nil {
		return fmt.Errorf("schema v2: %w", err)
	}
	return migrateBlobsToNormalized()
}

func migratePanelDBV3() error {
	for _, stmt := range []string{
		`ALTER TABLE wdtt_global ADD COLUMN admin_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE wdtt_global ADD COLUMN bot_token TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := panelDB.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}

func migratePanelDBV4() error {
	if _, err := panelDB.Exec(`CREATE TABLE IF NOT EXISTS xray_config (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		raw_json TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return err
	}
	if tableHasRows(`SELECT COUNT(*) FROM xray_config WHERE length(trim(raw_json)) > 0`) {
		return nil
	}
	data, err := os.ReadFile(xrayConfigPath)
	if err != nil {
		return nil
	}
	if _, err := panelDB.Exec(`INSERT INTO xray_config (id, raw_json) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET raw_json = excluded.raw_json`, string(data)); err != nil {
		return err
	}
	log.Printf("panel db: migrated xray_config from %s", xrayConfigPath)
	return nil
}

func tableHasRows(query string) bool {
	var n int
	if err := panelDB.QueryRow(query).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

func migrateBlobsToNormalized() error {
	if tableHasRows(`SELECT COUNT(*) FROM panel_config`) {
		return nil
	}

	if cfg, err := loadPanelConfigFromBlob(); err == nil {
		if err := savePanelConfigNorm(cfg); err != nil {
			return err
		}
		log.Printf("panel db: normalized panel_config")
	}

	var pdb PasswordsDB
	if ok, _ := dbLoadJSON(dbKeyPasswords, &pdb); ok {
		if err := savePasswordsNorm(&pdb); err != nil {
			return err
		}
		log.Printf("panel db: normalized wdtt_users (%d)", len(pdb.Passwords))
	}

	var inb WdttInboundConfig
	if ok, _ := dbLoadJSON(dbKeyInbound, &inb); ok {
		inb.normalize()
		if err := saveInboundNorm(inb); err != nil {
			return err
		}
		log.Printf("panel db: normalized wdtt_inbound")
	}

	var xm panelXrayMeta
	if ok, _ := dbLoadJSON(dbKeyXrayMeta, &xm); ok {
		if err := saveXrayMetaNorm(xm); err != nil {
			return err
		}
		log.Printf("panel db: normalized xray_panel_meta")
	}

	meta := map[string]PanelXrayInboundMeta{}
	if ok, _ := dbLoadJSON(dbKeyXrayInboundMeta, &meta); ok {
		if err := saveXrayInboundMetaNorm(meta); err != nil {
			return err
		}
		log.Printf("panel db: normalized xray_inbound_meta (%d)", len(meta))
	}

	var tr xrayTrafficPersist
	if ok, _ := dbLoadJSON(dbKeyXrayTraffic, &tr); ok {
		if err := saveXrayTrafficNorm(tr); err != nil {
			return err
		}
		log.Printf("panel db: normalized xray_traffic_totals")
	}
	return nil
}

func loadPanelConfigFromBlob() (*PanelConfig, error) {
	raw, ok, err := dbSettingGet(panelDBSetting)
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

func loadPanelConfigNorm() (*PanelConfig, error) {
	if !panelDBEnabled() {
		return nil, os.ErrNotExist
	}
	var cfg PanelConfig
	var blockPing int
	err := panelDB.QueryRow(`SELECT username, password_hash, port, web_base_path, session_key,
		web_listen, web_domain, web_cert_file, web_key_file, session_max_age, page_size, remark_model, block_ping
		FROM panel_config WHERE id = 1`).Scan(
		&cfg.Username, &cfg.PasswordHash, &cfg.Port, &cfg.WebBasePath, &cfg.SessionKey,
		&cfg.WebListen, &cfg.WebDomain, &cfg.WebCertFile, &cfg.WebKeyFile,
		&cfg.SessionMaxAge, &cfg.PageSize, &cfg.RemarkModel, &blockPing,
	)
	if err == sql.ErrNoRows {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	cfg.BlockPing = blockPing != 0
	return &cfg, nil
}

func savePanelConfigNorm(cfg *PanelConfig) error {
	if !panelDBEnabled() || cfg == nil {
		return nil
	}
	bp := 0
	if cfg.BlockPing {
		bp = 1
	}
	_, err := panelDB.Exec(`INSERT INTO panel_config (
		id, username, password_hash, port, web_base_path, session_key,
		web_listen, web_domain, web_cert_file, web_key_file,
		session_max_age, page_size, remark_model, block_ping
	) VALUES (1,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET
		username=excluded.username, password_hash=excluded.password_hash, port=excluded.port,
		web_base_path=excluded.web_base_path, session_key=excluded.session_key,
		web_listen=excluded.web_listen, web_domain=excluded.web_domain,
		web_cert_file=excluded.web_cert_file, web_key_file=excluded.web_key_file,
		session_max_age=excluded.session_max_age, page_size=excluded.page_size,
		remark_model=excluded.remark_model, block_ping=excluded.block_ping`,
		cfg.Username, cfg.PasswordHash, cfg.Port, cfg.WebBasePath, cfg.SessionKey,
		cfg.WebListen, cfg.WebDomain, cfg.WebCertFile, cfg.WebKeyFile,
		cfg.SessionMaxAge, cfg.PageSize, cfg.RemarkModel, bp,
	)
	return err
}

func loadPasswordsNorm() (*PasswordsDB, error) {
	if !panelDBEnabled() || !tableHasRows(`SELECT COUNT(*) FROM wdtt_users`) {
		return nil, os.ErrNotExist
	}
	db := &PasswordsDB{
		Passwords: map[string]*PasswordEntry{},
		Devices:   map[string]*DeviceEntry{},
	}
	_ = panelDB.QueryRow(`SELECT main_password, admin_id, bot_token FROM wdtt_global WHERE id = 1`).Scan(
		&db.MainPassword, &db.AdminID, &db.BotToken,
	)
	rows, err := panelDB.Query(`SELECT password, device_id, max_devices, expires_at, down_bytes, up_bytes,
		total_bytes, max_down_mbps, max_up_mbps, is_deactivated, comment, ports, vk_hash FROM wdtt_users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		e := &PasswordEntry{}
		var pass string
		var deactivated int
		if err := rows.Scan(&pass, &e.DeviceID, &e.MaxDevices, &e.ExpiresAt, &e.DownBytes, &e.UpBytes,
			&e.TotalBytes, &e.MaxDownMBps, &e.MaxUpMBps, &deactivated, &e.Comment, &e.Ports, &e.VkHash); err != nil {
			return nil, err
		}
		e.IsDeactivated = deactivated != 0
		db.Passwords[pass] = e
	}
	drows, err := panelDB.Query(`SELECT password, device_id, sort_order FROM wdtt_user_devices ORDER BY password, sort_order`)
	if err != nil {
		return nil, err
	}
	defer drows.Close()
	for drows.Next() {
		var pass, did string
		var ord int
		if err := drows.Scan(&pass, &did, &ord); err != nil {
			return nil, err
		}
		if e := db.Passwords[pass]; e != nil {
			e.DeviceIDs = append(e.DeviceIDs, did)
		}
	}
	devRows, err := panelDB.Query(`SELECT device_id, ip, priv_key, pub_key FROM wdtt_devices`)
	if err != nil {
		return nil, err
	}
	defer devRows.Close()
	for devRows.Next() {
		d := &DeviceEntry{}
		if err := devRows.Scan(&d.DeviceID, &d.IP, &d.PrivKey, &d.PubKey); err != nil {
			return nil, err
		}
		db.Devices[d.DeviceID] = d
	}
	for _, e := range db.Passwords {
		normalizeEntryDevices(e)
	}
	dedupePasswordDeviceBindings(db)
	return db, nil
}

func savePasswordsNorm(db *PasswordsDB) error {
	if !panelDBEnabled() || db == nil {
		return nil
	}
	tx, err := panelDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO wdtt_global (id, main_password, admin_id, bot_token) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			main_password = excluded.main_password,
			admin_id = excluded.admin_id,
			bot_token = excluded.bot_token`,
		db.MainPassword, db.AdminID, db.BotToken); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wdtt_user_devices`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wdtt_users`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wdtt_devices`); err != nil {
		return err
	}
	for pass, entry := range db.Passwords {
		if entry == nil {
			continue
		}
		normalizeEntryDevices(entry)
		deact := 0
		if entry.IsDeactivated {
			deact = 1
		}
		if _, err := tx.Exec(`INSERT INTO wdtt_users (
			password, device_id, max_devices, expires_at, down_bytes, up_bytes, total_bytes,
			max_down_mbps, max_up_mbps, is_deactivated, comment, ports, vk_hash
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			pass, entry.DeviceID, entry.MaxDevices, entry.ExpiresAt, entry.DownBytes, entry.UpBytes,
			entry.TotalBytes, entry.MaxDownMBps, entry.MaxUpMBps, deact, entry.Comment, entry.Ports, entry.VkHash,
		); err != nil {
			return err
		}
		for i, did := range allEntryDeviceIDsPanel(entry) {
			if _, err := tx.Exec(`INSERT INTO wdtt_user_devices (password, device_id, sort_order) VALUES (?,?,?)`,
				pass, did, i); err != nil {
				return err
			}
		}
	}
	for id, dev := range db.Devices {
		if dev == nil {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO wdtt_devices (device_id, ip, priv_key, pub_key) VALUES (?,?,?,?)`,
			id, dev.IP, dev.PrivKey, dev.PubKey); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func loadInboundNorm() (WdttInboundConfig, error) {
	cfg := defaultWdttInbound()
	if !panelDBEnabled() || !tableHasRows(`SELECT COUNT(*) FROM wdtt_inbound`) {
		return cfg, os.ErrNotExist
	}
	var enable int
	err := panelDB.QueryRow(`SELECT tag, remark, enable, listen_host, server_host, dtls_port, wg_port,
		client_port, dns, mtu, max_users, handshake_timeout_sec, max_dtls_per_device, admin_addr
		FROM wdtt_inbound WHERE id = 1`).Scan(
		&cfg.Tag, &cfg.Remark, &enable, &cfg.ListenHost, &cfg.ServerHost, &cfg.DtlsPort, &cfg.WgPort,
		&cfg.ClientPort, &cfg.DNS, &cfg.MTU, &cfg.MaxUsers, &cfg.HandshakeTimeoutSec, &cfg.MaxDtlsPerDevice, &cfg.AdminAddr,
	)
	if err == sql.ErrNoRows {
		return cfg, os.ErrNotExist
	}
	if err != nil {
		return cfg, err
	}
	cfg.Enable = enable != 0
	cfg.normalize()
	return cfg, nil
}

func saveInboundNorm(cfg WdttInboundConfig) error {
	if !panelDBEnabled() {
		return nil
	}
	cfg.normalize()
	en := 0
	if cfg.Enable {
		en = 1
	}
	_, err := panelDB.Exec(`INSERT INTO wdtt_inbound (
		id, tag, remark, enable, listen_host, server_host, dtls_port, wg_port, client_port, dns, mtu,
		max_users, handshake_timeout_sec, max_dtls_per_device, admin_addr
	) VALUES (1,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET
		tag=excluded.tag, remark=excluded.remark, enable=excluded.enable,
		listen_host=excluded.listen_host, server_host=excluded.server_host,
		dtls_port=excluded.dtls_port, wg_port=excluded.wg_port, client_port=excluded.client_port,
		dns=excluded.dns, mtu=excluded.mtu, max_users=excluded.max_users,
		handshake_timeout_sec=excluded.handshake_timeout_sec,
		max_dtls_per_device=excluded.max_dtls_per_device, admin_addr=excluded.admin_addr`,
		cfg.Tag, cfg.Remark, en, cfg.ListenHost, cfg.ServerHost, cfg.DtlsPort, cfg.WgPort,
		cfg.ClientPort, cfg.DNS, cfg.MTU, cfg.MaxUsers, cfg.HandshakeTimeoutSec, cfg.MaxDtlsPerDevice, cfg.AdminAddr,
	)
	return err
}

func loadXrayMetaNorm() (panelXrayMeta, bool, error) {
	meta := panelXrayMeta{OutboundTestURL: "https://www.google.com/generate_204"}
	if !panelDBEnabled() || !tableHasRows(`SELECT COUNT(*) FROM xray_panel_meta`) {
		return meta, false, nil
	}
	err := panelDB.QueryRow(`SELECT outbound_test_url, warp FROM xray_panel_meta WHERE id = 1`).Scan(&meta.OutboundTestURL, &meta.Warp)
	if err == sql.ErrNoRows {
		return meta, false, nil
	}
	if err != nil {
		return meta, false, err
	}
	if meta.OutboundTestURL == "" {
		meta.OutboundTestURL = "https://www.google.com/generate_204"
	}
	return meta, true, nil
}

func saveXrayMetaNorm(meta panelXrayMeta) error {
	if !panelDBEnabled() {
		return nil
	}
	if meta.OutboundTestURL == "" {
		meta.OutboundTestURL = "https://www.google.com/generate_204"
	}
	_, err := panelDB.Exec(`INSERT INTO xray_panel_meta (id, outbound_test_url, warp) VALUES (1,?,?)
		ON CONFLICT(id) DO UPDATE SET outbound_test_url=excluded.outbound_test_url, warp=excluded.warp`,
		meta.OutboundTestURL, meta.Warp)
	return err
}

func loadXrayInboundMetaNorm() (map[string]PanelXrayInboundMeta, bool, error) {
	out := map[string]PanelXrayInboundMeta{}
	if !panelDBEnabled() || !tableHasRows(`SELECT COUNT(*) FROM xray_inbound_meta`) {
		return out, false, nil
	}
	rows, err := panelDB.Query(`SELECT tag, remark, enable, total, expiry_time, traffic_reset FROM xray_inbound_meta`)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var tag string
		var m PanelXrayInboundMeta
		var en int
		if err := rows.Scan(&tag, &m.Remark, &en, &m.Total, &m.ExpiryTime, &m.TrafficReset); err != nil {
			return nil, false, err
		}
		m.Enable = en != 0
		out[tag] = m
	}
	return out, true, nil
}

func saveXrayInboundMetaNorm(meta map[string]PanelXrayInboundMeta) error {
	if !panelDBEnabled() {
		return nil
	}
	tx, err := panelDB.Begin()
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

func loadXrayTrafficNorm() (xrayTrafficPersist, bool, error) {
	var p xrayTrafficPersist
	if !panelDBEnabled() || !tableHasRows(`SELECT COUNT(*) FROM xray_traffic_totals`) {
		return p, false, nil
	}
	err := panelDB.QueryRow(`SELECT up, down FROM xray_traffic_totals WHERE id = 1`).Scan(&p.Up, &p.Down)
	if err == sql.ErrNoRows {
		return p, false, nil
	}
	return p, err == nil, err
}

func saveXrayTrafficNorm(p xrayTrafficPersist) error {
	if !panelDBEnabled() {
		return nil
	}
	_, err := panelDB.Exec(`INSERT INTO xray_traffic_totals (id, up, down) VALUES (1,?,?)
		ON CONFLICT(id) DO UPDATE SET up=excluded.up, down=excluded.down`, p.Up, p.Down)
	return err
}

func loadXrayConfigNorm() (string, bool, error) {
	if !panelDBEnabled() || !tableHasRows(`SELECT COUNT(*) FROM xray_config WHERE length(trim(raw_json)) > 0`) {
		return "", false, nil
	}
	var raw string
	err := panelDB.QueryRow(`SELECT raw_json FROM xray_config WHERE id = 1`).Scan(&raw)
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

func saveXrayConfigNorm(raw string) error {
	if !panelDBEnabled() {
		return nil
	}
	_, err := panelDB.Exec(`INSERT INTO xray_config (id, raw_json) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET raw_json = excluded.raw_json`, raw)
	return err
}

func migrateToNormalizedTables() {
	if !panelDBEnabled() {
		return
	}
	if err := migrateBlobsToNormalized(); err != nil {
		log.Printf("panel db normalize: %v", err)
	}
}
