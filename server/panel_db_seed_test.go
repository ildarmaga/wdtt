package server

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	_ "modernc.org/sqlite"
)

const seedTestWDTTDDL = `
CREATE TABLE wdtt_global (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	main_password TEXT NOT NULL DEFAULT '',
	admin_id TEXT NOT NULL DEFAULT '',
	bot_token TEXT NOT NULL DEFAULT '',
	users_rev INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE wdtt_users (
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
	vk_hash TEXT NOT NULL DEFAULT '',
	sub_id TEXT NOT NULL DEFAULT '',
	last_seen_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE wdtt_user_devices (
	password TEXT NOT NULL,
	device_id TEXT NOT NULL,
	sort_order INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (password, device_id)
);
CREATE TABLE wdtt_devices (
	device_id TEXT PRIMARY KEY,
	ip TEXT NOT NULL DEFAULT '',
	priv_key TEXT NOT NULL DEFAULT '',
	pub_key TEXT NOT NULL DEFAULT ''
);
CREATE TABLE wdtt_inbound (id INTEGER PRIMARY KEY);
`

func TestLoadDatabaseFromSQLiteMainPasswordOnly(t *testing.T) {
	dir := t.TempDir()
	oldPath := panelDBPath
	panelDBPath = filepath.Join(dir, "panel.db")
	defer func() {
		panelDBPath = oldPath
		serverPanelDB = nil
		serverPanelDBErr = nil
		serverPanelDBOnce = sync.Once{}
	}()

	db, err := sql.Open("sqlite", paneldb.DSN(panelDBPath))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(seedTestWDTTDDL); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO wdtt_global (id, main_password) VALUES (1, 'main-only')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	got, ok, err := loadDatabaseFromSQLite()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected ok with main_password only")
	}
	if got.MainPassword != "main-only" {
		t.Fatalf("main_password=%q", got.MainPassword)
	}
	if len(got.Passwords) != 0 {
		t.Fatalf("users=%d want 0 before ensureMainPasswordEntryLocked", len(got.Passwords))
	}
}
