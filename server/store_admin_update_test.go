package server

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	_ "modernc.org/sqlite"
)

func setupStoreAdminPanelDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	oldPath := panelDBPath
	panelDBPath = filepath.Join(dir, "panel.db")
	t.Cleanup(func() {
		panelDBPath = oldPath
		serverPanelDB = nil
		serverPanelDBErr = nil
		serverPanelDBOnce = sync.Once{}
	})

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
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func listUserPasswords(t *testing.T, sqlDB *sql.DB) []string {
	t.Helper()
	rows, err := sqlDB.Query(`SELECT password FROM wdtt_users ORDER BY password`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func TestApplyPanelUserUpdateClearsVkHashAndPorts(t *testing.T) {
	sqlDB := setupStoreAdminPanelDB(t)
	if err := paneldb.UpsertUser(sqlDB, "main", "u1", &paneldb.User{
		MaxDevices: 1,
		VkHash:     "abc,def",
		Ports:      "56000,56001,9000",
		SubID:      "s1",
	}); err != nil {
		t.Fatal(err)
	}

	dbMutex.Lock()
	db = &Database{
		MainPassword: "main",
		Passwords: map[string]*PasswordEntry{
			"main": {MaxDevices: 0},
			"u1":   {MaxDevices: 1, VkHash: "abc,def", Ports: "56000,56001,9000", SubID: "s1"},
		},
		Devices: make(map[string]*ClientDevice),
	}
	active := true
	err := applyPanelUserUpdateLocked(nil, panelUserUpdateReq{
		OldPassword:    "u1",
		Password:       "u1",
		MaxDevices:     1,
		Active:         &active,
		VkHash:         "",
		UseCustomPorts: false,
	}, false)
	dbMutex.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	var vk, ports string
	if err := sqlDB.QueryRow(`SELECT vk_hash, ports FROM wdtt_users WHERE password=?`, "u1").Scan(&vk, &ports); err != nil {
		t.Fatal(err)
	}
	if vk != "" || ports != "" {
		t.Fatalf("want cleared vk/ports, got vk=%q ports=%q", vk, ports)
	}
}

func TestApplyPanelUserUpdatePreservesPortsWhenProvidedWithoutCustomFlag(t *testing.T) {
	// VK Creator / removeVKHash send Ports: entry.Ports with UseCustomPorts=false.
	sqlDB := setupStoreAdminPanelDB(t)
	if err := paneldb.UpsertUser(sqlDB, "main", "u1", &paneldb.User{
		MaxDevices: 1, Ports: "111,222,333", VkHash: "a", SubID: "s1",
	}); err != nil {
		t.Fatal(err)
	}
	dbMutex.Lock()
	db = &Database{
		MainPassword: "main",
		Passwords: map[string]*PasswordEntry{
			"main": {MaxDevices: 0},
			"u1":   {MaxDevices: 1, Ports: "111,222,333", VkHash: "a", SubID: "s1"},
		},
		Devices: make(map[string]*ClientDevice),
	}
	active := true
	err := applyPanelUserUpdateLocked(nil, panelUserUpdateReq{
		OldPassword: "u1", Password: "u1", MaxDevices: 1, Active: &active,
		Ports: "111,222,333", VkHash: "a,b", UseCustomPorts: false,
	}, false)
	dbMutex.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var ports string
	if err := sqlDB.QueryRow(`SELECT ports FROM wdtt_users WHERE password=?`, "u1").Scan(&ports); err != nil {
		t.Fatal(err)
	}
	if ports != "111,222,333" {
		t.Fatalf("ports cleared unexpectedly: %q", ports)
	}
}

func TestApplyPanelUserUpdateNormalizesVkHashJoinURL(t *testing.T) {
	sqlDB := setupStoreAdminPanelDB(t)
	if err := paneldb.UpsertUser(sqlDB, "main", "u1", &paneldb.User{MaxDevices: 1, SubID: "s1"}); err != nil {
		t.Fatal(err)
	}
	dbMutex.Lock()
	db = &Database{
		MainPassword: "main",
		Passwords: map[string]*PasswordEntry{
			"main": {MaxDevices: 0},
			"u1":   {MaxDevices: 1, SubID: "s1"},
		},
		Devices: make(map[string]*ClientDevice),
	}
	active := true
	err := applyPanelUserUpdateLocked(nil, panelUserUpdateReq{
		OldPassword:    "u1",
		Password:       "u1",
		MaxDevices:     1,
		Active:         &active,
		VkHash:         "https://vk.ru/call/join/TokEn123",
		UseCustomPorts: true,
		DtlsPort:       56000,
		WgPort:         56001,
		ClientPort:     9000,
	}, false)
	dbMutex.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var vk, ports string
	if err := sqlDB.QueryRow(`SELECT vk_hash, ports FROM wdtt_users WHERE password=?`, "u1").Scan(&vk, &ports); err != nil {
		t.Fatal(err)
	}
	if vk != "TokEn123" {
		t.Fatalf("vk_hash=%q want TokEn123", vk)
	}
	if ports != "56000,56001,9000" {
		t.Fatalf("ports=%q", ports)
	}
}

func TestApplyPanelUserUpdateRenamesWithManageDevices(t *testing.T) {
	sqlDB := setupStoreAdminPanelDB(t)
	if err := paneldb.UpsertUser(sqlDB, "main", "oldpass", &paneldb.User{
		Comment:    "u1",
		MaxDevices: 2,
		VkHash:     "hashA",
		Ports:      "56000,56001,9000",
		SubID:      "sub1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := paneldb.PatchUserDeviceBindings(sqlDB, "main", "oldpass", []string{"dev1"}, nil); err != nil {
		t.Fatal(err)
	}

	dbMutex.Lock()
	db = &Database{
		MainPassword: "main",
		Passwords: map[string]*PasswordEntry{
			"main":    {MaxDevices: 0},
			"oldpass": {Comment: "u1", MaxDevices: 2, VkHash: "hashA", Ports: "56000,56001,9000", SubID: "sub1", DeviceIDs: []string{"dev1"}},
		},
		Devices: map[string]*ClientDevice{
			"dev1": {DeviceID: "dev1", IP: "10.66.66.2"},
		},
	}
	appliedUsersRev = 0
	active := true
	devs := []string{"dev1"}
	err := applyPanelUserUpdateLocked(nil, panelUserUpdateReq{
		OldPassword: "oldpass",
		Password:    "newpass",
		DeviceIDs:   &devs,
		MaxDevices:  2,
		Comment:     "u1",
		Active:      &active,
		VkHash:      "hashA",
		Ports:       "56000,56001,9000",
		UseCustomPorts: true,
	}, true)
	dbMutex.Unlock()
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	got := listUserPasswords(t, sqlDB)
	if len(got) != 1 || got[0] != "newpass" {
		t.Fatalf("sqlite passwords=%v want [newpass] only", got)
	}

	dbMutex.Lock()
	_, oldAlive := db.Passwords["oldpass"]
	_, newAlive := db.Passwords["newpass"]
	wrapN := serverWrapKeys.Count()
	dbMutex.Unlock()
	if oldAlive {
		t.Fatal("oldpass still in RAM")
	}
	if !newAlive {
		t.Fatal("newpass missing from RAM")
	}
	if wrapN < 1 {
		t.Fatalf("WRAP keys not refreshed, count=%d", wrapN)
	}

	// Simulate panel sync: old must not resurrect from disk.
	dbMutex.Lock()
	appliedUsersRev = 0
	syncPanelDeviceEditsLocked()
	_, oldAlive = db.Passwords["oldpass"]
	_, newAlive = db.Passwords["newpass"]
	dbMutex.Unlock()
	if oldAlive {
		t.Fatal("oldpass resurrected after syncPanelDeviceEditsLocked")
	}
	if !newAlive {
		t.Fatal("newpass missing after sync")
	}
}
