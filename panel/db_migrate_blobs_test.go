package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openBlobMigrateTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:blob_migrate_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE panel_config (id INTEGER PRIMARY KEY CHECK (id = 1), username TEXT NOT NULL DEFAULT 'admin', password_hash TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 2860, web_base_path TEXT NOT NULL DEFAULT '/wdtt/', session_key TEXT NOT NULL DEFAULT '', web_listen TEXT NOT NULL DEFAULT '', web_domain TEXT NOT NULL DEFAULT '', web_cert_file TEXT NOT NULL DEFAULT '', web_key_file TEXT NOT NULL DEFAULT '', session_max_age INTEGER NOT NULL DEFAULT 60, page_size INTEGER NOT NULL DEFAULT 50, remark_model TEXT NOT NULL DEFAULT '-ieo', block_ping INTEGER NOT NULL DEFAULT 0, sub_enable INTEGER NOT NULL DEFAULT 1, sub_listen TEXT NOT NULL DEFAULT '', sub_port INTEGER NOT NULL DEFAULT 2096, sub_path TEXT NOT NULL DEFAULT '/sub/', sub_domain TEXT NOT NULL DEFAULT '', sub_cert_file TEXT NOT NULL DEFAULT '', sub_key_file TEXT NOT NULL DEFAULT '', sub_encrypt INTEGER NOT NULL DEFAULT 1, sub_updates INTEGER NOT NULL DEFAULT 12, sub_title TEXT NOT NULL DEFAULT '', sub_support_url TEXT NOT NULL DEFAULT '', sub_profile_url TEXT NOT NULL DEFAULT '', sub_announce TEXT NOT NULL DEFAULT '', sub_uri TEXT NOT NULL DEFAULT '', sub_show_info INTEGER NOT NULL DEFAULT 1);
CREATE TABLE wdtt_global (id INTEGER PRIMARY KEY CHECK (id = 1), main_password TEXT NOT NULL DEFAULT '', admin_id TEXT NOT NULL DEFAULT '', bot_token TEXT NOT NULL DEFAULT '');
CREATE TABLE wdtt_users (password TEXT PRIMARY KEY, device_id TEXT NOT NULL DEFAULT '', max_devices INTEGER NOT NULL DEFAULT 0, expires_at INTEGER NOT NULL DEFAULT 0, down_bytes INTEGER NOT NULL DEFAULT 0, up_bytes INTEGER NOT NULL DEFAULT 0, total_bytes INTEGER NOT NULL DEFAULT 0, max_down_mbps REAL NOT NULL DEFAULT 0, max_up_mbps REAL NOT NULL DEFAULT 0, is_deactivated INTEGER NOT NULL DEFAULT 0, comment TEXT NOT NULL DEFAULT '', ports TEXT NOT NULL DEFAULT '', vk_hash TEXT NOT NULL DEFAULT '', sub_id TEXT NOT NULL DEFAULT '', last_seen_at INTEGER NOT NULL DEFAULT 0);
CREATE TABLE wdtt_user_devices (password TEXT NOT NULL, device_id TEXT NOT NULL, sort_order INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (password, device_id));
CREATE TABLE wdtt_devices (device_id TEXT PRIMARY KEY, ip TEXT NOT NULL DEFAULT '', priv_key TEXT NOT NULL DEFAULT '', pub_key TEXT NOT NULL DEFAULT '');
CREATE TABLE wdtt_inbound (id INTEGER PRIMARY KEY CHECK (id = 1), tag TEXT NOT NULL DEFAULT 'wdtt-in', remark TEXT NOT NULL DEFAULT 'WDTT', enable INTEGER NOT NULL DEFAULT 1, listen_host TEXT NOT NULL DEFAULT '0.0.0.0', server_host TEXT NOT NULL DEFAULT '', dtls_port INTEGER NOT NULL DEFAULT 56000, wg_port INTEGER NOT NULL DEFAULT 56001, client_port INTEGER NOT NULL DEFAULT 9000, dns TEXT NOT NULL DEFAULT '1.1.1.1', mtu INTEGER NOT NULL DEFAULT 1280, max_users INTEGER NOT NULL DEFAULT 10, handshake_timeout_sec INTEGER NOT NULL DEFAULT 30, max_dtls_per_device INTEGER NOT NULL DEFAULT 0, online_timeout_sec INTEGER NOT NULL DEFAULT 15, admin_addr TEXT NOT NULL DEFAULT '127.0.0.1:2861');
CREATE TABLE xray_panel_meta (id INTEGER PRIMARY KEY CHECK (id = 1), outbound_test_url TEXT NOT NULL DEFAULT 'https://www.google.com/generate_204', warp TEXT NOT NULL DEFAULT '');
CREATE TABLE xray_inbound_meta (tag TEXT PRIMARY KEY, remark TEXT NOT NULL DEFAULT '', enable INTEGER NOT NULL DEFAULT 1, total INTEGER NOT NULL DEFAULT 0, expiry_time INTEGER NOT NULL DEFAULT 0, traffic_reset TEXT NOT NULL DEFAULT '');
CREATE TABLE xray_traffic_totals (id INTEGER PRIMARY KEY CHECK (id = 1), up INTEGER NOT NULL DEFAULT 0, down INTEGER NOT NULL DEFAULT 0);
`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMigrateLegacySettingsBlobsImportsPanelConfig(t *testing.T) {
	db := openBlobMigrateTestDB(t)
	defer db.Close()

	panelDB = db
	oldPath := panelDBPath
	panelDBPath = ":memory:"
	defer func() {
		panelDB = nil
		panelDBPath = oldPath
	}()

	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`, legacySettingPanel, `{"username":"ops","port":3000,"webBasePath":"/wdtt/"}`); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacySettingsBlobs(); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadPanelConfigNorm()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Username != "ops" || cfg.Port != 3000 {
		t.Fatalf("panel config mismatch: %+v", cfg)
	}
}

func TestPurgeLegacySettingsBlobs(t *testing.T) {
	db := openBlobMigrateTestDB(t)
	defer db.Close()

	panelDB = db
	defer func() { panelDB = nil }()

	for _, key := range legacySettingKeys {
		if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`, key, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	if err := purgeLegacySettingsBlobs(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings WHERE key IN ('panel','passwords','inbound')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected legacy blobs removed, got %d rows", n)
	}
}
