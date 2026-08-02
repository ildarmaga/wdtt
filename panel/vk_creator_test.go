package panel

import (
	"database/sql"
	"fmt"
	"strconv"
	"testing"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	"github.com/ildarmaga/wdtt/pkg/vkhash"
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
	oldValidate := vkCookiesValidateLive
	vkCookiesValidateLive = func(string) error { return nil }
	defer func() { vkCookiesValidateLive = oldValidate }()
	invalidateVKCookiesStatusCache()
	ok, _, _, expired := vkCookiesStatus()
	if !ok || expired {
		t.Fatal("expected cookies ok")
	}
}

func TestVKCookiesStatusExpired(t *testing.T) {
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
	if err := saveVKCookies([]byte("remixsid=expired; remixlang=0")); err != nil {
		t.Fatal(err)
	}
	oldValidate := vkCookiesValidateLive
	vkCookiesValidateLive = func(string) error { return fmt.Errorf("empty token") }
	defer func() { vkCookiesValidateLive = oldValidate }()
	invalidateVKCookiesStatusCache()
	ok, hint, present, expired := vkCookiesStatus()
	if ok || !present || !expired {
		t.Fatalf("expected expired cookies, ok=%v present=%v expired=%v hint=%q", ok, present, expired, hint)
	}
	if hint != vkCookiesExpiredHint {
		t.Fatalf("hint=%q", hint)
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

func TestCountVKCreatorSessionsForPassword(t *testing.T) {
	db, err := sql.Open("sqlite", "file:vk_count_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE vk_calls (
		call_id TEXT PRIMARY KEY, password TEXT NOT NULL DEFAULT '',
		join_link TEXT NOT NULL DEFAULT '', vk_hash TEXT NOT NULL DEFAULT '',
		started_at INTEGER NOT NULL DEFAULT 0, finishing INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	pass := "user-secret"
	for i := 0; i < vkhash.Max; i++ {
		id := fmt.Sprintf("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeee%02d", i)
		if err := paneldb.InsertVKCall(db, paneldb.VKCall{
			CallID: id, Password: pass, JoinLink: "https://vk.com/call/join/x",
			VkHash: "h" + strconv.Itoa(i), StartedAt: int64(i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	calls, err := paneldb.ListVKCalls(db)
	if err != nil || len(calls) != vkhash.Max {
		t.Fatalf("calls: %d err=%v", len(calls), err)
	}
}

func TestSubtractVKHashes(t *testing.T) {
	merged := mergeVKHashes("hash1,hash2", "hash3")
	got := subtractVKHashes(merged, []string{"hash2"})
	if got != "hash1,hash3" {
		t.Fatalf("got %q", got)
	}
	if subtractVKHashes("a,b", []string{"c"}) != "a,b" {
		t.Fatal("unchanged when hash not present")
	}
	if subtractVKHashes("only", []string{"only"}) != "" {
		t.Fatal("expected empty")
	}
}

func TestMergeVKHashesKeepsExistingOnApply(t *testing.T) {
	// API create+apply must expose this merged value as vk_hash (not only the new token).
	got := mergeVKHashes("aaa,bbb", "ccc")
	if got != "aaa,bbb,ccc" {
		t.Fatalf("got %q want aaa,bbb,ccc", got)
	}
	if mergeVKHashes("aaa", "aaa") != "aaa" {
		t.Fatal("dedup failed")
	}
}
