package panel

import (
	"strings"
	"testing"
)

func TestBuildDNSIPDomainMap(t *testing.T) {
	lines := []string{
		"2026/06/18 12:56:46.419257 UDP:1.1.1.1:53 got answer: i.instagram.com. -> [57.144.222.192] 43.556002ms",
		"2026/06/18 12:55:37.971808 UDP:1.1.1.1:53 got answer: www.google.com. -> [142.251.152.119, 142.251.153.119] 43.324824ms",
	}
	m := buildDNSIPDomainMap(lines)
	if m["57.144.222.192"] != "i.instagram.com" {
		t.Fatalf("instagram map=%q", m["57.144.222.192"])
	}
	if m["142.251.153.119"] != "www.google.com" {
		t.Fatalf("google map=%q", m["142.251.153.119"])
	}
}

func TestParseXrayAccessLogLinesWithDNSDomains(t *testing.T) {
	raw := []string{
		"2026/06/18 12:56:46.419257 UDP:1.1.1.1:53 got answer: ya.ru. -> [77.88.21.119] 40ms",
		"2026/06/18 12:56:47.000000 from 10.66.66.2:12345 accepted tcp:77.88.21.119:443 [redirect-in -> blocked]",
	}
	entries := parseXrayAccessLogLines(raw, 10, "", true, true, true, []string{"direct"}, []string{"blocked"}, defaultXrayAPIPort)
	if len(entries) != 1 {
		t.Fatalf("entries=%d", len(entries))
	}
	if entries[0].ToAddress != "tcp:ya.ru:443" {
		t.Fatalf("ToAddress=%q", entries[0].ToAddress)
	}
	if strings.Contains(entries[0].ToAddress, "77.88") {
		t.Fatalf("expected domain not IP: %q", entries[0].ToAddress)
	}
}

func TestIsInternalXrayAccessLine(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"2026/06/16 15:10:02 from 127.0.0.1:58142 accepted tcp:127.0.0.1:62789 [api -> api]", true},
		{"2026/06/16 15:10:02 from 10.66.66.2:12345 accepted tcp:vk.com:443 [redirect-in -> direct]", false},
		{"2026/06/16 10:11:35 from DNS accepted udp:1.1.1.1:53 [xray.system.uuid -> NL]", false},
	}
	for _, tc := range cases {
		if got := isInternalXrayAccessLine(tc.line, defaultXrayAPIPort); got != tc.want {
			t.Fatalf("line=%q got=%v want=%v", tc.line, got, tc.want)
		}
	}
}
