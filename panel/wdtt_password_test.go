package panel

import (
	"testing"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

func TestMigrateMainPasswordDB(t *testing.T) {
	db := &PasswordsDB{
		MainPassword: "old",
		Passwords: map[string]*PasswordEntry{
			"old": {Comment: paneldb.MainUserComment, UpBytes: 100, DeviceIDs: []string{"dev1"}},
			"other": {Comment: "guest"},
		},
	}
	migrateMainPasswordDB(db, "new")
	if db.MainPassword != "new" {
		t.Fatalf("main_password = %q, want new", db.MainPassword)
	}
	if _, ok := db.Passwords["old"]; ok {
		t.Fatal("old main key still in passwords")
	}
	entry := db.Passwords["new"]
	if entry == nil || entry.UpBytes != 100 || len(entry.DeviceIDs) != 1 {
		t.Fatalf("main entry not migrated: %+v", entry)
	}
	if db.Passwords["other"] == nil {
		t.Fatal("other user removed")
	}
}
