package paneldb

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

const testWDTTDDL = `
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

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:paneldb_test?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(testWDTTDDL); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO wdtt_global (id, main_password) VALUES (1, 'main')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLoadSaveRoundtrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	s := NewStore()
	s.MainPassword = "main"
	s.Users["u1"] = &User{
		Comment: "test", DeviceIDs: []string{"dev-a", "dev-b"}, MaxDevices: 2,
		SubID: "sub123",
	}
	s.Devices["dev-a"] = &Device{DeviceID: "dev-a", IP: "10.66.66.2"}

	if err := SaveStore(db, s, SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if got.MainPassword != "main" {
		t.Fatalf("main: %q", got.MainPassword)
	}
	u := got.Users["u1"]
	if u == nil || u.Comment != "test" || len(u.DeviceIDs) != 2 || u.SubID != "sub123" {
		t.Fatalf("user: %+v", u)
	}
	if got.Devices["dev-a"] == nil || got.Devices["dev-a"].IP != "10.66.66.2" {
		t.Fatalf("device: %+v", got.Devices["dev-a"])
	}
}

func TestSavePreserveSubID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	s1 := NewStore()
	s1.MainPassword = "main"
	s1.Users["u1"] = &User{SubID: "keep-me"}
	if err := SaveStore(db, s1, SaveOptions{}); err != nil {
		t.Fatal(err)
	}

	s2 := NewStore()
	s2.MainPassword = "main"
	s2.Users["u1"] = &User{Comment: "updated"}
	if err := SaveStore(db, s2, SaveOptions{PreserveSubIDs: true}); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadStore(db)
	if got.Users["u1"].SubID != "keep-me" {
		t.Fatalf("sub_id lost: %q", got.Users["u1"].SubID)
	}
}

func TestUpsertDevice(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	if err := UpsertDevice(db, &Device{DeviceID: "dev1", IP: "10.66.66.2", PrivKey: "p", PubKey: "k"}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertDevice(db, &Device{DeviceID: "dev1", IP: "10.66.66.3", PrivKey: "p2", PubKey: "k2"}); err != nil {
		t.Fatal(err)
	}
	var ip string
	if err := db.QueryRow(`SELECT ip FROM wdtt_devices WHERE device_id = 'dev1'`).Scan(&ip); err != nil {
		t.Fatal(err)
	}
	if ip != "10.66.66.3" {
		t.Fatalf("ip = %q", ip)
	}
}

func TestUserPatchIncremental(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	u := &User{
		Comment:    "bot-user",
		ExpiresAt:  999,
		VkHash:     "hash",
		Ports:      "56000,56001,9000",
		SubID:      "sub1",
		MaxDevices: 1,
	}
	if err := UpsertUser(db, "main", "pass1", u); err != nil {
		t.Fatal(err)
	}
	if err := SetUserDeactivated(db, "pass1", true); err != nil {
		t.Fatal(err)
	}
	got, err := LoadStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Users["pass1"].IsDeactivated {
		t.Fatal("expected deactivated")
	}
	if err := DeleteUser(db, "pass1", nil); err != nil {
		t.Fatal(err)
	}
	got, err = LoadStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Users) != 0 {
		t.Fatalf("users left: %d", len(got.Users))
	}
}

func TestSetMainPasswordAndResetTraffic(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if err := UpsertUser(db, "main", "main", &User{Comment: "owner", UpBytes: 100, DownBytes: 200}); err != nil {
		t.Fatal(err)
	}
	if err := RenameUserPassword(db, "main", "newmain"); err != nil {
		t.Fatal(err)
	}
	if err := SetMainPassword(db, "newmain"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if got.MainPassword != "newmain" {
		t.Fatalf("main password: %q", got.MainPassword)
	}
	if got.Users["newmain"] == nil {
		t.Fatal("main user row missing after rename")
	}
	if err := ResetUserTraffic(db, "newmain"); err != nil {
		t.Fatal(err)
	}
	got, err = LoadStore(db)
	if err != nil {
		t.Fatal(err)
	}
	u := got.Users["newmain"]
	if u == nil || u.UpBytes != 0 || u.DownBytes != 0 || u.IsDeactivated {
		t.Fatalf("after reset: %+v", u)
	}
}
