package panel

import "testing"

func TestAcmeCertInstallDir(t *testing.T) {
	if got := acmeCertInstallDir("203.0.113.10"); got != "/root/cert/ip" {
		t.Fatalf("ip cert dir: got %q", got)
	}
	if got := acmeCertInstallDir("ip"); got != "/root/cert/ip" {
		t.Fatalf("ip alias: got %q", got)
	}
	if got := acmeCertInstallDir("vpn.example.com"); got != "/root/cert/vpn.example.com" {
		t.Fatalf("domain cert dir: got %q", got)
	}
}
