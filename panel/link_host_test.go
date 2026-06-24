package panel

import "testing"

func TestDomainFromCertPath(t *testing.T) {
	if got := domainFromCertPath("/root/.acme.sh/vpn.example.com/fullchain.cer"); got != "vpn.example.com" {
		t.Fatalf("got %q", got)
	}
	if domainFromCertPath("") != "" {
		t.Fatal("empty cert path")
	}
}

func TestPanelDomainExplicitWebDomain(t *testing.T) {
	cfg := &PanelConfig{WebDomain: "listen.example.com"}
	if got := cfg.panelDomain(); got != "listen.example.com" {
		t.Fatalf("panelDomain() = %q", got)
	}
}

func TestPanelDomainEmptyStaysEmptyInConfig(t *testing.T) {
	cfg := &PanelConfig{
		WebDomain:   "",
		WebCertFile: "/root/.acme.sh/vpn.example.com/fullchain.cer",
	}
	if cfg.WebDomain != "" {
		t.Fatal("WebDomain must remain empty when user cleared the field")
	}
}
