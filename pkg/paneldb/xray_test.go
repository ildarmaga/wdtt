package paneldb

import (
	"database/sql"
	"testing"
)

const testXrayDDL = `
CREATE TABLE xray_panel_meta (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	outbound_test_url TEXT NOT NULL DEFAULT 'https://www.google.com/generate_204',
	warp TEXT NOT NULL DEFAULT ''
);
CREATE TABLE xray_inbound_meta (
	tag TEXT PRIMARY KEY,
	remark TEXT NOT NULL DEFAULT '',
	enable INTEGER NOT NULL DEFAULT 1,
	total INTEGER NOT NULL DEFAULT 0,
	expiry_time INTEGER NOT NULL DEFAULT 0,
	traffic_reset TEXT NOT NULL DEFAULT ''
);
CREATE TABLE xray_traffic_totals (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	up INTEGER NOT NULL DEFAULT 0,
	down INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE xray_config (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	raw_json TEXT NOT NULL DEFAULT ''
);
`

func openXrayTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:xray_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(testXrayDDL); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestXrayMetaLoadSave(t *testing.T) {
	db := openXrayTestDB(t)
	defer db.Close()

	if err := SaveXrayMeta(db, XrayMeta{OutboundTestURL: "https://example.com/test", Warp: "on"}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadXrayMeta(db)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.OutboundTestURL != "https://example.com/test" || got.Warp != "on" {
		t.Fatalf("meta: %+v", got)
	}
}

func TestXrayInboundMetaLoadSave(t *testing.T) {
	db := openXrayTestDB(t)
	defer db.Close()

	in := map[string]XrayInboundMeta{
		"vless-in": {Remark: "VLESS", Enable: true, Total: 100},
	}
	if err := SaveXrayInboundMeta(db, in); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadXrayInboundMeta(db)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got["vless-in"].Remark != "VLESS" || got["vless-in"].Total != 100 {
		t.Fatalf("inbound meta: %+v", got)
	}
}

func TestXrayTrafficAndConfig(t *testing.T) {
	db := openXrayTestDB(t)
	defer db.Close()

	if err := SaveXrayTraffic(db, XrayTrafficTotals{Up: 10, Down: 20}); err != nil {
		t.Fatal(err)
	}
	tr, ok, err := LoadXrayTraffic(db)
	if err != nil || !ok || tr.Up != 10 || tr.Down != 20 {
		t.Fatalf("traffic: %+v ok=%v err=%v", tr, ok, err)
	}

	raw := `{"log":{}}`
	if err := SaveXrayConfig(db, raw); err != nil {
		t.Fatal(err)
	}
	cfg, ok, err := LoadXrayConfig(db)
	if err != nil || !ok || cfg != raw {
		t.Fatalf("config: %q ok=%v err=%v", cfg, ok, err)
	}
}
