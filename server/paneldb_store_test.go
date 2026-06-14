package main

import (
	"testing"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

func TestStoreFromDatabaseRoundtrip(t *testing.T) {
	src := &Database{
		MainPassword: "main",
		AdminID:      "admin1",
		BotToken:     "bot",
		Passwords: map[string]*PasswordEntry{
			"pass1": {
				DeviceIDs:  []string{"dev-a", "dev-b"},
				MaxDevices: 2,
				Comment:    "user",
				SubID:      "sub-abc",
				LastSeenAt: 1710000000,
			},
		},
		Devices: map[string]*ClientDevice{
			"dev-a": {DeviceID: "dev-a", IP: "10.66.66.2", PrivKey: "priv", PubKey: "pub"},
		},
	}

	got := databaseFromStore(storeFromDatabase(src))
	if got.MainPassword != src.MainPassword || got.AdminID != src.AdminID || got.BotToken != src.BotToken {
		t.Fatalf("global fields mismatch: %+v", got)
	}
	if len(got.Passwords) != 1 || len(got.Devices) != 1 {
		t.Fatalf("expected 1 user and 1 device, got %d/%d", len(got.Passwords), len(got.Devices))
	}
	e := got.Passwords["pass1"]
	if e == nil || e.Comment != "user" || e.SubID != "sub-abc" || len(e.DeviceIDs) != 2 {
		t.Fatalf("password entry mismatch: %+v", e)
	}
	d := got.Devices["dev-a"]
	if d == nil || d.IP != "10.66.66.2" {
		t.Fatalf("device mismatch: %+v", d)
	}
}

func TestStoreFromDatabaseNil(t *testing.T) {
	if storeFromDatabase(nil) == nil {
		t.Fatal("expected non-nil store")
	}
	got := databaseFromStore(nil)
	if got == nil || got.Passwords == nil || got.Devices == nil {
		t.Fatal("expected empty database maps")
	}
}

func TestNormalizeEntryDevicesUsesPaneldb(t *testing.T) {
	entry := &PasswordEntry{DeviceID: "legacy", MaxDevices: 0}
	normalizeEntryDevices(entry)
	if entry.MaxDevices != 1 {
		t.Fatalf("expected max_devices=1, got %d", entry.MaxDevices)
	}
	u := userToPaneldb(entry)
	if len(paneldb.AllDeviceIDs(u)) != 1 || paneldb.AllDeviceIDs(u)[0] != "legacy" {
		t.Fatalf("expected legacy device id preserved, got %+v", paneldb.AllDeviceIDs(u))
	}
}
