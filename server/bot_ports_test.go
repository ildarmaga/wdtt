package server

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	_ "modernc.org/sqlite"
)

func TestDefaultInboundPortsCSVFromDB(t *testing.T) {
	dir := t.TempDir()
	oldPath := panelDBPath
	panelDBPath = filepath.Join(dir, "panel.db")
	defer func() {
		panelDBPath = oldPath
		serverPanelDB = nil
		serverPanelDBErr = nil
		serverPanelDBOnce = sync.Once{}
	}()

	sqlDB, err := sql.Open("sqlite", paneldb.DSN(panelDBPath))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(seedTestWDTTDDL); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`DROP TABLE wdtt_inbound`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`CREATE TABLE wdtt_inbound (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		tag TEXT NOT NULL DEFAULT '',
		remark TEXT NOT NULL DEFAULT '',
		enable INTEGER NOT NULL DEFAULT 1,
		listen_host TEXT NOT NULL DEFAULT '',
		server_host TEXT NOT NULL DEFAULT '',
		dtls_port INTEGER NOT NULL DEFAULT 56000,
		wg_port INTEGER NOT NULL DEFAULT 56001,
		client_port INTEGER NOT NULL DEFAULT 9000,
		dns TEXT NOT NULL DEFAULT '',
		mtu INTEGER NOT NULL DEFAULT 1280,
		max_users INTEGER NOT NULL DEFAULT 10,
		handshake_timeout_sec INTEGER NOT NULL DEFAULT 30,
		max_dtls_per_device INTEGER NOT NULL DEFAULT 0,
		online_timeout_sec INTEGER NOT NULL DEFAULT 60,
		wg_keepalive_sec INTEGER NOT NULL DEFAULT 25,
		stats_interval_sec INTEGER NOT NULL DEFAULT 2,
		admin_addr TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO wdtt_global (id, main_password) VALUES (1, 'main')`); err != nil {
		t.Fatal(err)
	}
	if err := paneldb.SaveInbound(sqlDB, &paneldb.Inbound{
		Tag: "wdtt", Enable: true, DtlsPort: 56100, WgPort: 56101, ClientPort: 9100, MTU: 1280, MaxUsers: 10,
	}); err != nil {
		t.Fatal(err)
	}
	_ = sqlDB.Close()

	got := defaultInboundPortsCSV()
	if got != "56100,56101,9100" {
		t.Fatalf("got %q", got)
	}
}
