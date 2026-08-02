package server

import (
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	_ "modernc.org/sqlite"
)

func TestTrustPanelTrafficCounters(t *testing.T) {
	dbMutex.Lock()
	old := appliedUsersRev
	appliedUsersRev = 5
	dbMutex.Unlock()
	defer func() {
		dbMutex.Lock()
		appliedUsersRev = old
		dbMutex.Unlock()
	}()

	if !trustPanelTrafficCounters(6) {
		t.Fatal("expected trust when disk rev ahead of applied")
	}
	if trustPanelTrafficCounters(5) {
		t.Fatal("expected no trust when disk rev equals applied")
	}
	if trustPanelTrafficCounters(4) {
		t.Fatal("expected no trust when disk rev behind applied")
	}
}

func TestReloadDBFromDiskRespectsTrafficReset(t *testing.T) {
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
	if _, err := sqlDB.Exec(`INSERT INTO wdtt_global (id, main_password) VALUES (1, 'main')`); err != nil {
		t.Fatal(err)
	}
	if err := paneldb.UpsertUser(sqlDB, "main", "user1", &paneldb.User{
		UpBytes:   7_000_000_000,
		DownBytes: 1_000_000_000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := paneldb.ResetUserTraffic(sqlDB, "user1"); err != nil {
		t.Fatal(err)
	}
	_ = sqlDB.Close()

	dbMutex.Lock()
	db = &Database{
		Passwords: map[string]*PasswordEntry{
			"user1": {UpBytes: 7_000_000_000, DownBytes: 1_000_000_000},
		},
		Devices: make(map[string]*ClientDevice),
	}
	appliedUsersRev = 0
	trafficDirty.Store(true)
	dbMutex.Unlock()

	if err := reloadDBFromDisk(nil); err != nil {
		t.Fatal(err)
	}

	dbMutex.Lock()
	entry := db.Passwords["user1"]
	gotRev := appliedUsersRev
	dirty := trafficDirty.Load()
	dbMutex.Unlock()

	if entry == nil {
		t.Fatal("user missing after reload")
	}
	if entry.UpBytes != 0 || entry.DownBytes != 0 {
		t.Fatalf("expected zero traffic after reset reload, got up=%d down=%d", entry.UpBytes, entry.DownBytes)
	}
	if gotRev <= 0 {
		t.Fatalf("expected users_rev applied, got %d", gotRev)
	}
	if dirty {
		t.Fatal("trafficDirty should be cleared when trusting panel counters")
	}
}

func TestTrafficFlushFencedAfterReset(t *testing.T) {
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
	if _, err := sqlDB.Exec(`INSERT INTO wdtt_global (id, main_password) VALUES (1, 'main')`); err != nil {
		t.Fatal(err)
	}
	if err := paneldb.UpsertUser(sqlDB, "main", "user1", &paneldb.User{
		UpBytes:   7_000_000_000,
		DownBytes: 1_000_000_000,
	}); err != nil {
		t.Fatal(err)
	}
	_ = sqlDB.Close()

	dbMutex.Lock()
	db = &Database{
		MainPassword: "main",
		Passwords: map[string]*PasswordEntry{
			"user1": {UpBytes: 7_000_000_000, DownBytes: 1_000_000_000},
		},
		Devices: make(map[string]*ClientDevice),
	}
	appliedUsersRev = 0
	trafficDirty.Store(true)
	dbMutex.Unlock()

	// Panel reset bumps users_rev and zeros disk — before server reload.
	sqlDB, err = sql.Open("sqlite", paneldb.DSN(panelDBPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := paneldb.ResetUserTraffic(sqlDB, "user1"); err != nil {
		t.Fatal(err)
	}
	_ = sqlDB.Close()

	dbMutex.Lock()
	flushErr := saveTrafficToSQLiteLocked()
	dbMutex.Unlock()
	if !errors.Is(flushErr, errTrafficFlushFenced) {
		t.Fatalf("expected fenced flush, got %v", flushErr)
	}

	// Disk must still be zeros.
	sqlDB, err = sql.Open("sqlite", paneldb.DSN(panelDBPath))
	if err != nil {
		t.Fatal(err)
	}
	var up, down int64
	if err := sqlDB.QueryRow(`SELECT up_bytes, down_bytes FROM wdtt_users WHERE password=?`, "user1").Scan(&up, &down); err != nil {
		t.Fatal(err)
	}
	_ = sqlDB.Close()
	if up != 0 || down != 0 {
		t.Fatalf("flush overwrote reset: up=%d down=%d", up, down)
	}

	if err := reloadDBFromDisk(nil); err != nil {
		t.Fatal(err)
	}
	dbMutex.Lock()
	entry := db.Passwords["user1"]
	dbMutex.Unlock()
	if entry == nil || entry.UpBytes != 0 || entry.DownBytes != 0 {
		t.Fatalf("after reload want zeros, entry=%+v", entry)
	}
}
