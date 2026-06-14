package paneldb

import (
	"database/sql"
	"testing"
)

const testPanelConfigDDL = `
CREATE TABLE panel_config (
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
	block_ping INTEGER NOT NULL DEFAULT 0,
	sub_enable INTEGER NOT NULL DEFAULT 1,
	sub_listen TEXT NOT NULL DEFAULT '',
	sub_port INTEGER NOT NULL DEFAULT 2096,
	sub_path TEXT NOT NULL DEFAULT '/sub/',
	sub_domain TEXT NOT NULL DEFAULT '',
	sub_cert_file TEXT NOT NULL DEFAULT '',
	sub_key_file TEXT NOT NULL DEFAULT '',
	sub_encrypt INTEGER NOT NULL DEFAULT 1,
	sub_updates INTEGER NOT NULL DEFAULT 12,
	sub_title TEXT NOT NULL DEFAULT '',
	sub_support_url TEXT NOT NULL DEFAULT '',
	sub_profile_url TEXT NOT NULL DEFAULT '',
	sub_announce TEXT NOT NULL DEFAULT '',
	sub_uri TEXT NOT NULL DEFAULT '',
	sub_show_info INTEGER NOT NULL DEFAULT 1
);
`

func openPanelConfigTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:panel_config_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(testPanelConfigDDL); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPanelConfigLoadSave(t *testing.T) {
	db := openPanelConfigTestDB(t)
	defer db.Close()

	cfg := &PanelConfig{
		Username: "admin", PasswordHash: "hash", Port: 3000, WebBasePath: "/wdtt/",
		SessionKey: "key", SubEnable: true, SubPort: 2097, SubPath: "/sub/",
		SubEncrypt: true, SubShowInfo: true, SubUpdates: 12,
	}
	if err := SavePanelConfig(db, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPanelConfig(db)
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != 3000 || got.SubPort != 2097 || !got.SubEnable || got.PasswordHash != "hash" {
		t.Fatalf("config: %+v", got)
	}
}

func TestLoadPanelServicePorts(t *testing.T) {
	db := openPanelConfigTestDB(t)
	defer db.Close()

	if err := SavePanelConfig(db, &PanelConfig{Port: 2861, SubPort: 2098}); err != nil {
		t.Fatal(err)
	}
	p, s, ok, err := LoadPanelServicePorts(db)
	if err != nil || !ok {
		t.Fatalf("ports: ok=%v err=%v", ok, err)
	}
	if p != 2861 || s != 2098 {
		t.Fatalf("ports: panel=%d sub=%d", p, s)
	}
}
