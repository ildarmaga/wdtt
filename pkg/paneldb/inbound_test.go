package paneldb

import (
	"database/sql"
	"testing"
)

const testInboundDDL = `
CREATE TABLE wdtt_inbound (
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
`

func openInboundTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:inbound_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(testInboundDDL); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestInboundLoadSave(t *testing.T) {
	db := openInboundTestDB(t)
	defer db.Close()

	in := &Inbound{
		Tag: "wdtt-in", Remark: "test", Enable: true,
		ListenHost: "0.0.0.0", DtlsPort: 56000, WgPort: 56001,
		DNS: "8.8.8.8", MTU: 1400, MaxUsers: 5, OnlineTimeoutSec: 20,
	}
	if err := SaveInbound(db, in); err != nil {
		t.Fatal(err)
	}
	got, err := LoadInbound(db)
	if err != nil {
		t.Fatal(err)
	}
	if got.DNS != "8.8.8.8" || got.MTU != 1400 || got.MaxUsers != 5 || got.OnlineTimeoutSec != 20 {
		t.Fatalf("inbound: %+v", got)
	}
}

func TestLoadRuntimeSettings(t *testing.T) {
	db := openInboundTestDB(t)
	defer db.Close()

	if err := SaveInbound(db, &Inbound{DNS: "1.0.0.1", MTU: 1280, MaxUsers: 10, HandshakeTimeoutSec: 30, OnlineTimeoutSec: 15}); err != nil {
		t.Fatal(err)
	}
	rs, ok, err := LoadRuntimeSettings(db)
	if err != nil || !ok {
		t.Fatalf("load runtime: ok=%v err=%v", ok, err)
	}
	if rs.DNS != "1.0.0.1" || rs.OnlineTimeoutSec != 15 {
		t.Fatalf("runtime: %+v", rs)
	}
}
