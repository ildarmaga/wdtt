package panel

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

func TestBootstrapDBSeedsMainUserRow(t *testing.T) {
	dir := t.TempDir()
	oldPanelDB := panelDBPath
	oldConfigDir := wdttConfigDir
	defer func() {
		panelDBPath = oldPanelDB
		wdttConfigDir = oldConfigDir
		bootstrapOnce = sync.Once{}
		bootstrapErr = nil
		if panelDB != nil {
			panelDB.Close()
			panelDB = nil
		}
	}()

	wdttConfigDir = dir
	panelDBPath = filepath.Join(dir, "panel.db")
	mainPass := "test-bootstrap-main-pass"
	if err := os.WriteFile(filepath.Join(dir, "install-main-password.env"),
		[]byte("MAIN_PASSWORD="+mainPass+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := BootstrapDB(); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapDB(); err != nil { // idempotent
		t.Fatal(err)
	}

	s, err := paneldb.LoadStore(panelDB)
	if err != nil {
		t.Fatal(err)
	}
	if s.MainPassword != mainPass {
		t.Fatalf("main_password=%q want %q", s.MainPassword, mainPass)
	}
	if s.Users[mainPass] == nil {
		t.Fatal("wdtt_users row for main password missing")
	}
	n, err := paneldb.UserCount(panelDB)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("user count=%d want 1", n)
	}
}
