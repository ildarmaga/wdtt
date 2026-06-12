package main

import "testing"

func TestAcmeContactEmail(t *testing.T) {
	if got := acmeContactEmail(nil); got != acmeFallbackEmail {
		t.Fatalf("nil cfg: got %q want %q", got, acmeFallbackEmail)
	}
	got := acmeContactEmail(&PanelConfig{WebDomain: "dev.example.com"})
	if got != "admin@dev.example.com" {
		t.Fatalf("domain cfg: got %q", got)
	}
	got = acmeContactEmail(&PanelConfig{WebDomain: "45.146.131.254"})
	if got != acmeFallbackEmail {
		t.Fatalf("ip webDomain should fallback: got %q", got)
	}
}
