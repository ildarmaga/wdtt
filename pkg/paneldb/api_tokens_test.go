package paneldb

import (
	"testing"
)

func TestAPITokenCRUD(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Create table
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS api_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		token TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL DEFAULT '',
		scope TEXT NOT NULL DEFAULT 'admin',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL DEFAULT 0,
		last_used INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatal(err)
	}

	// Create token
	tok, err := CreateAPIToken(db, "test-token", APITokenScopeAdmin)
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if tok.Token == "" || len(tok.Token) < 10 {
		t.Fatalf("token too short: %q", tok.Token)
	}
	if tok.Name != "test-token" {
		t.Fatalf("name mismatch: %q", tok.Name)
	}
	if tok.Scope != APITokenScopeAdmin {
		t.Fatalf("scope mismatch: %q", tok.Scope)
	}
	if !tok.Enabled {
		t.Fatal("token should be enabled")
	}

	// Lookup
	found, err := LookupAPIToken(db, tok.Token)
	if err != nil {
		t.Fatalf("LookupAPIToken: %v", err)
	}
	if found == nil {
		t.Fatal("token not found")
	}
	if found.ID != tok.ID {
		t.Fatalf("ID mismatch: %d vs %d", found.ID, tok.ID)
	}

	// List
	tokens, err := ListAPITokens(db)
	if err != nil {
		t.Fatalf("ListAPITokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	// Token should be masked in list
	if tokens[0].Token == tok.Token {
		t.Fatal("token should be masked in list")
	}

	// Toggle
	if err := ToggleAPIToken(db, tok.ID); err != nil {
		t.Fatalf("ToggleAPIToken: %v", err)
	}
	found, _ = LookupAPIToken(db, tok.Token)
	if found != nil {
		t.Fatal("disabled token should not be found")
	}

	// Toggle back
	if err := ToggleAPIToken(db, tok.ID); err != nil {
		t.Fatalf("ToggleAPIToken: %v", err)
	}
	found, _ = LookupAPIToken(db, tok.Token)
	if found == nil {
		t.Fatal("re-enabled token should be found")
	}

	// Touch
	TouchAPIToken(db, tok.ID)

	// Delete
	if err := DeleteAPIToken(db, tok.ID); err != nil {
		t.Fatalf("DeleteAPIToken: %v", err)
	}
	tokens, _ = ListAPITokens(db)
	if len(tokens) != 0 {
		t.Fatalf("expected 0 tokens after delete, got %d", len(tokens))
	}
}

func TestAPITokenReadonlyScope(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS api_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		token TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL DEFAULT '',
		scope TEXT NOT NULL DEFAULT 'admin',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL DEFAULT 0,
		last_used INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatal(err)
	}

	tok, err := CreateAPIToken(db, "readonly", APITokenScopeReadonly)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Scope != APITokenScopeReadonly {
		t.Fatalf("expected readonly, got %q", tok.Scope)
	}

	found, _ := LookupAPIToken(db, tok.Token)
	if found == nil || found.Scope != APITokenScopeReadonly {
		t.Fatal("readonly scope not preserved")
	}
}

func TestGenerateAPIToken(t *testing.T) {
	tok1, err := GenerateAPIToken()
	if err != nil {
		t.Fatal(err)
	}
	tok2, err := GenerateAPIToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok1 == tok2 {
		t.Fatal("tokens should be unique")
	}
	if len(tok1) != 53 { // "wdtt_" + 48 hex chars (24 bytes)
		t.Fatalf("unexpected token length: %d", len(tok1))
	}
}
