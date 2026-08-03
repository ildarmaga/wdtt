package paneldb

import "testing"

func TestReconcileVKHashesSync(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS vk_calls (
		call_id TEXT PRIMARY KEY,
		password TEXT NOT NULL DEFAULT '',
		join_link TEXT NOT NULL DEFAULT '',
		vk_hash TEXT NOT NULL DEFAULT '',
		started_at INTEGER NOT NULL DEFAULT 0,
		finishing INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatal(err)
	}
	// a: manual "dead" + live1; creator adds live2, finishing should drop "finishing" if present
	if err := UpsertUser(db, "main", "a", &User{MaxDevices: 1, SubID: "sa", VkHash: "dead,live1,finishing"}); err != nil {
		t.Fatal(err)
	}
	// b: manual-only hash, no vk_calls — must survive (issue #23)
	if err := UpsertUser(db, "main", "b", &User{MaxDevices: 1, SubID: "sb", VkHash: "manual-hash"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO vk_calls (call_id, password, vk_hash, started_at, finishing) VALUES
		('c1','a','live1',1,0),
		('c2','a','live2',2,0),
		('c3','a','finishing',3,1)`); err != nil {
		t.Fatal(err)
	}
	n, err := ReconcileVKHashesSync(db)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("fixed=%d want 1 (only a)", n)
	}
	var ha, hb string
	_ = db.QueryRow(`SELECT vk_hash FROM wdtt_users WHERE password='a'`).Scan(&ha)
	_ = db.QueryRow(`SELECT vk_hash FROM wdtt_users WHERE password='b'`).Scan(&hb)
	if ha != "dead,live1,live2" {
		t.Fatalf("a vk_hash=%q want dead,live1,live2", ha)
	}
	if hb != "manual-hash" {
		t.Fatalf("b must keep manual hash, got %q", hb)
	}
}

func TestMergeVKHashReconcile(t *testing.T) {
	got := mergeVKHashReconcile("manual,old", []string{"live"}, map[string]bool{"old": true})
	if got != "manual,live" {
		t.Fatalf("got %q", got)
	}
	got = mergeVKHashReconcile("only-manual", nil, nil)
	if got != "only-manual" {
		t.Fatalf("manual wiped: %q", got)
	}
}
