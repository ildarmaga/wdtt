package main

import (
	"os"
	"strings"
	"testing"
)

func TestResolveAcmeContactEmail(t *testing.T) {
	if got := resolveAcmeContactEmail(nil, "user@gmail.com"); got != "user@gmail.com" {
		t.Fatalf("override: got %q", got)
	}
	got := resolveAcmeContactEmail(&PanelConfig{WebDomain: "vpn.example.com"}, "")
	if got != "admin@vpn.example.com" {
		t.Fatalf("webDomain: got %q", got)
	}
	if resolveAcmeContactEmail(&PanelConfig{WebDomain: "45.146.131.254"}, "user@gmail.com") != "user@gmail.com" {
		t.Fatal("override should win over ip webDomain")
	}
	if resolveAcmeContactEmail(&PanelConfig{WebDomain: "45.146.131.254"}, "") != "" {
		t.Fatal("ip-only server should require explicit email")
	}
}

func TestIsValidAcmeContactEmail(t *testing.T) {
	cases := map[string]bool{
		"user@gmail.com":        true,
		"admin@vpn.example.com": true,
		"admin@localhost":       false,
		"noreply@example.com":   false,
		"bad":                   false,
	}
	for email, want := range cases {
		if got := isValidAcmeContactEmail(email); got != want {
			t.Fatalf("%q: got %v want %v", email, got, want)
		}
	}
}

func TestSetConfKey(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.conf"
	if err := setConfKey(path, "ACCOUNT_EMAIL", "user@gmail.com"); err != nil {
		t.Fatal(err)
	}
	if err := setConfKey(path, "ACCOUNT_EMAIL", "new@gmail.com"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "ACCOUNT_EMAIL='new@gmail.com'") {
		t.Fatalf("unexpected content: %s", data)
	}
}
