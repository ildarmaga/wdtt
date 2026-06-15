package panel

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

const dbSchemaVersion = 10

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
	online_timeout_sec INTEGER NOT NULL DEFAULT 15,
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
	return migrateLegacySettingsBlobs()
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

func migratePanelDBV5() error {
	return nil
}

func migratePanelDBV6() error {
	for _, stmt := range []string{
		`ALTER TABLE panel_config ADD COLUMN sub_enable INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE panel_config ADD COLUMN sub_listen TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE panel_config ADD COLUMN sub_port INTEGER NOT NULL DEFAULT 2096`,
		`ALTER TABLE panel_config ADD COLUMN sub_path TEXT NOT NULL DEFAULT '/sub/'`,
		`ALTER TABLE panel_config ADD COLUMN sub_domain TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE panel_config ADD COLUMN sub_cert_file TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE panel_config ADD COLUMN sub_key_file TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE panel_config ADD COLUMN sub_encrypt INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE panel_config ADD COLUMN sub_updates INTEGER NOT NULL DEFAULT 12`,
		`ALTER TABLE panel_config ADD COLUMN sub_title TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE panel_config ADD COLUMN sub_support_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE panel_config ADD COLUMN sub_profile_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE panel_config ADD COLUMN sub_announce TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE panel_config ADD COLUMN sub_uri TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE panel_config ADD COLUMN sub_show_info INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE wdtt_users ADD COLUMN sub_id TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := panelDB.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	if _, err := panelDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_wdtt_users_sub_id ON wdtt_users(sub_id) WHERE sub_id != ''`); err != nil {
		return err
	}
	migrateLegacyJSONFiles()
	return migrateEnsureUserSubIDs()
}

func migratePanelDBV7() error {
	if _, err := panelDB.Exec(`ALTER TABLE wdtt_users ADD COLUMN last_seen_at INTEGER NOT NULL DEFAULT 0`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return err
	}
	return nil
}

func migratePanelDBV8() error {
	if _, err := panelDB.Exec(`ALTER TABLE wdtt_inbound ADD COLUMN online_timeout_sec INTEGER NOT NULL DEFAULT 15`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return err
	}
	return nil
}

func migrateEnsureUserSubIDs() error {
	rows, err := panelDB.Query(`SELECT password, sub_id FROM wdtt_users`)
	if err != nil {
		return err
	}
	var needSubID []string
	for rows.Next() {
		var pass, subID string
		if err := rows.Scan(&pass, &subID); err != nil {
			rows.Close()
			return err
		}
		if strings.TrimSpace(subID) == "" {
			needSubID = append(needSubID, pass)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, pass := range needSubID {
		newID, err := genSubID()
		if err != nil {
			return err
		}
		if _, err := panelDB.Exec(`UPDATE wdtt_users SET sub_id = ? WHERE password = ?`, newID, pass); err != nil {
			return err
		}
		log.Printf("panel db: assigned sub_id for user %s", maskPassword(pass))
	}
	return nil
}

func tableHasRows(query string) bool {
	var n int
	if err := panelDB.QueryRow(query).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

func ensureLegacySettingsImported() {
	if !panelDBEnabled() {
		return
	}
	if err := migrateLegacySettingsBlobs(); err != nil {
		log.Printf("panel db: import legacy settings blobs: %v", err)
	}
}

func loadPanelConfigNorm() (*PanelConfig, error) {
	if !panelDBEnabled() {
		return nil, os.ErrNotExist
	}
	pc, err := paneldb.LoadPanelConfig(panelDB)
	if err == sql.ErrNoRows {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	cfg := panelConfigFromPaneldb(pc)
	normalizeSubConfig(cfg)
	return cfg, nil
}

func savePanelConfigNorm(cfg *PanelConfig) error {
	if !panelDBEnabled() || cfg == nil {
		return nil
	}
	normalizeSubConfig(cfg)
	return paneldb.SavePanelConfig(panelDB, panelConfigToPaneldb(cfg))
}

func loadPasswordsNorm() (*PasswordsDB, error) {
	if !panelDBEnabled() {
		return nil, os.ErrNotExist
	}
	has, err := paneldb.HasUsers(panelDB)
	if err != nil || !has {
		return nil, os.ErrNotExist
	}
	s, err := paneldb.LoadStore(panelDB)
	if err != nil {
		return nil, err
	}
	db := passwordsDBFromStore(s)
	dedupePasswordDeviceBindings(db)
	return db, nil
}

func savePasswordsNorm(db *PasswordsDB) error {
	if !panelDBEnabled() || db == nil {
		return nil
	}
	if err := mergeTrafficFromDisk(db); err != nil {
		return err
	}
	return paneldb.SaveStore(panelDB, storeFromPasswordsDB(db), paneldb.SaveOptions{PreserveSubIDs: true})
}

func patchUserDeviceBindingsNorm(db *PasswordsDB, password string, entry *PasswordEntry, removeDeviceIDs []string) error {
	if !panelDBEnabled() || db == nil || entry == nil {
		return fmt.Errorf("panel database not available")
	}
	return paneldb.PatchUserDeviceBindings(panelDB, db.MainPassword, password, entry.DeviceIDs, removeDeviceIDs)
}

func mergeTrafficFromDisk(db *PasswordsDB) error {
	if !panelDBEnabled() || db == nil {
		return nil
	}
	s, err := paneldb.LoadStore(panelDB)
	if err != nil {
		return err
	}
	snap := make(map[string]paneldb.TrafficSnapshot, len(s.Users))
	for pass, u := range s.Users {
		if u == nil {
			continue
		}
		snap[pass] = paneldb.TrafficSnapshot{
			UpBytes:       u.UpBytes,
			DownBytes:     u.DownBytes,
			LastSeenAt:    u.LastSeenAt,
			IsDeactivated: u.IsDeactivated,
		}
	}
	users := make(map[string]*paneldb.User, len(db.Passwords))
	for pass, e := range db.Passwords {
		if e == nil {
			continue
		}
		users[pass] = userEntryToPaneldb(e)
	}
	paneldb.MergeTrafficSnapshots(users, snap)
	for pass, u := range users {
		if e := db.Passwords[pass]; e != nil && u != nil {
			e.UpBytes = u.UpBytes
			e.DownBytes = u.DownBytes
			e.LastSeenAt = u.LastSeenAt
			if u.IsDeactivated {
				e.IsDeactivated = true
			}
		}
	}
	return nil
}

func loadInboundNorm() (WdttInboundConfig, error) {
	cfg := defaultWdttInbound()
	if !panelDBEnabled() {
		return cfg, os.ErrNotExist
	}
	has, err := paneldb.HasInbound(panelDB)
	if err != nil || !has {
		return cfg, os.ErrNotExist
	}
	in, err := paneldb.LoadInbound(panelDB)
	if err != nil {
		return cfg, err
	}
	cfg = wdttInboundFromPaneldb(in)
	cfg.normalize()
	return cfg, nil
}

func saveInboundNorm(cfg WdttInboundConfig) error {
	if !panelDBEnabled() {
		return nil
	}
	cfg.normalize()
	return paneldb.SaveInbound(panelDB, wdttInboundToPaneldb(cfg))
}

func loadXrayMetaNorm() (panelXrayMeta, bool, error) {
	if !panelDBEnabled() {
		return panelXrayMeta{OutboundTestURL: paneldb.DefaultXrayOutboundTestURL}, false, nil
	}
	meta, ok, err := paneldb.LoadXrayMeta(panelDB)
	if err != nil {
		return panelXrayMeta{}, false, err
	}
	return xrayMetaFromPaneldb(meta), ok, nil
}

func saveXrayMetaNorm(meta panelXrayMeta) error {
	if !panelDBEnabled() {
		return nil
	}
	return paneldb.SaveXrayMeta(panelDB, xrayMetaToPaneldb(meta))
}

func loadXrayInboundMetaNorm() (map[string]PanelXrayInboundMeta, bool, error) {
	if !panelDBEnabled() {
		return map[string]PanelXrayInboundMeta{}, false, nil
	}
	meta, ok, err := paneldb.LoadXrayInboundMeta(panelDB)
	if err != nil {
		return nil, false, err
	}
	return xrayInboundMetaFromPaneldb(meta), ok, nil
}

func saveXrayInboundMetaNorm(meta map[string]PanelXrayInboundMeta) error {
	if !panelDBEnabled() {
		return nil
	}
	return paneldb.SaveXrayInboundMeta(panelDB, xrayInboundMetaToPaneldb(meta))
}

func loadXrayTrafficNorm() (xrayTrafficPersist, bool, error) {
	if !panelDBEnabled() {
		return xrayTrafficPersist{}, false, nil
	}
	p, ok, err := paneldb.LoadXrayTraffic(panelDB)
	if err != nil {
		return xrayTrafficPersist{}, false, err
	}
	return xrayTrafficFromPaneldb(p), ok, nil
}

func saveXrayTrafficNorm(p xrayTrafficPersist) error {
	if !panelDBEnabled() {
		return nil
	}
	return paneldb.SaveXrayTraffic(panelDB, xrayTrafficToPaneldb(p))
}

func loadXrayConfigNorm() (string, bool, error) {
	if !panelDBEnabled() {
		return "", false, nil
	}
	return paneldb.LoadXrayConfig(panelDB)
}

func saveXrayConfigNorm(raw string) error {
	if !panelDBEnabled() {
		return nil
	}
	return paneldb.SaveXrayConfig(panelDB, raw)
}
