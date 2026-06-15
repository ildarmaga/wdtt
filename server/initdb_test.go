package server

import "testing"

func TestApplyDBGlobalFromCLI(t *testing.T) {
	db = &Database{
		MainPassword: "keep-main",
		AdminID:      "keep-admin",
		BotToken:     "keep-bot",
	}
	applyDBGlobalFromCLI("", "", "")
	if db.MainPassword != "keep-main" || db.AdminID != "keep-admin" || db.BotToken != "keep-bot" {
		t.Fatalf("empty CLI should preserve globals: %+v", db)
	}
	applyDBGlobalFromCLI("new-main", "new-admin", "new-bot")
	if db.MainPassword != "new-main" || db.AdminID != "new-admin" || db.BotToken != "new-bot" {
		t.Fatalf("CLI should override set fields: %+v", db)
	}
}
