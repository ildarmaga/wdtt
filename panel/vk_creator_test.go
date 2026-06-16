package panel

import (
	"database/sql"
	"testing"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

func TestSaveVKCookieStringDB(t *testing.T) {
	dir := t.TempDir()
	oldPath := panelDBPath
	panelDBPath = dir + "/panel.db"
	defer func() {
		if panelDB != nil {
			panelDB.Close()
			panelDB = nil
		}
		panelDBPath = oldPath
	}()
	if err := initPanelDB(); err != nil {
		t.Fatal(err)
	}
	if err := saveVKCookies([]byte("remixsid=test123; remixlang=0")); err != nil {
		t.Fatal(err)
	}
	ok, _ := vkCookiesStatus()
	if !ok {
		t.Fatal("expected cookies ok")
	}
}

func TestVKCallsDBRoundtrip(t *testing.T) {
	db, err := sql.Open("sqlite", "file:vk_calls_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE vk_calls (
			call_id TEXT PRIMARY KEY,
			password TEXT NOT NULL DEFAULT '',
			join_link TEXT NOT NULL DEFAULT '',
			vk_hash TEXT NOT NULL DEFAULT '',
			started_at INTEGER NOT NULL DEFAULT 0,
			finishing INTEGER NOT NULL DEFAULT 0
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	call := paneldb.VKCall{
		CallID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		Password: "user1", JoinLink: "https://vk.com/call/join/abc",
		VkHash: "abc", StartedAt: 100,
	}
	if err := paneldb.InsertVKCall(db, call); err != nil {
		t.Fatal(err)
	}
	list, err := paneldb.ListVKCalls(db)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v err=%v", list, err)
	}
	if err := paneldb.DeleteVKCall(db, call.CallID); err != nil {
		t.Fatal(err)
	}
	list, _ = paneldb.ListVKCalls(db)
	if len(list) != 0 {
		t.Fatalf("expected empty, got %d", len(list))
	}
}
