package panel

import (
	"strings"
	"testing"
)

func TestBuildAllSubscriptionLinks(t *testing.T) {
	entry := &PasswordEntry{
		Comment: "TestUser",
		VkHash:  "abc123,def456",
		Ports:   "56000,56001,9000",
	}
	inbound := WdttInboundConfig{
		ServerHost: "94.156.114.115",
		DtlsPort:   56000,
		WgPort:     56001,
		ClientPort: 9000,
		Tag:        "WDTT",
	}
	links, titles, err := buildAllSubscriptionLinks("94.156.114.115", "mypass", "TestUser", "WDTT VPN", entry, inbound, "https://example.com/sub/abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) < 4 {
		t.Fatalf("expected at least 4 links, got %d: %v", len(links), links)
	}
	if len(titles) != len(links) {
		t.Fatalf("titles %d != links %d", len(titles), len(links))
	}
	if titles[0] != "WDTT JSON" {
		t.Fatalf("first title %q", titles[0])
	}
	foundColon := false
	foundQwdtt := false
	for _, l := range links {
		if strings.HasPrefix(l, "wdtt://") && !strings.HasPrefix(l, "wdtt://eyJ") {
			foundColon = true
		}
		if strings.HasPrefix(l, "qwdtt://") {
			foundQwdtt = true
		}
	}
	if !foundColon {
		t.Fatalf("missing colon link: %v", links)
	}
	if !foundQwdtt {
		t.Fatalf("missing qwdtt link: %v", links)
	}
}
