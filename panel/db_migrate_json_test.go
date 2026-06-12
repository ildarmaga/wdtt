package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyJSONFiles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "panel.db")
	oldPanelDBPath := panelDBPath
	oldPanelConfigPath := panelConfigPath
	oldWdttInboundPath := wdttInboundPath
	oldLegacyPasswordsPath := legacyPasswordsPath
	defer func() {
		panelDBPath = oldPanelDBPath
		panelConfigPath = oldPanelConfigPath
		wdttInboundPath = oldWdttInboundPath
		legacyPasswordsPath = oldLegacyPasswordsPath
		if panelDB != nil {
			panelDB.Close()
			panelDB = nil
		}
	}()

	panelDBPath = dbPath
	panelConfigPath = filepath.Join(dir, "panel.json")
	wdttInboundPath = filepath.Join(dir, "inbound.json")
	legacyPasswordsPath = filepath.Join(dir, "passwords.json")

	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(panelConfigPath, []byte(`{
  "username": "admin",
  "password_hash": "$2a$10$abcdefghijklmnopqrstuv",
  "port": 2860,
  "web_base_path": "/wdtt/",
  "session_key": "abc123"
}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wdttInboundPath, []byte(`{
  "tag": "wdtt-in",
  "remark": "Test",
  "enable": true,
  "listen_host": "0.0.0.0",
  "dtls_port": 56000,
  "wg_port": 56001,
  "client_port": 9000,
  "dns": "1.1.1.1",
  "mtu": 1280,
  "max_users": 10,
  "handshake_timeout_sec": 30,
  "max_dtls_per_device": 0,
  "admin_addr": "127.0.0.1:2861"
}`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := initPanelDB(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(panelConfigPath); !os.IsNotExist(err) {
		t.Fatalf("panel.json should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(wdttInboundPath); !os.IsNotExist(err) {
		t.Fatalf("inbound.json should be removed, stat err=%v", err)
	}

	cfg, err := loadPanelConfigFromDB()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 2860 {
		t.Fatalf("port=%d", cfg.Port)
	}
	inb, err := loadInboundNorm()
	if err != nil {
		t.Fatal(err)
	}
	if inb.DtlsPort != 56000 {
		t.Fatalf("dtls_port=%d", inb.DtlsPort)
	}
}
