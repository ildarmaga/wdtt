package sharelink

import (
	"strings"
	"testing"
)

func TestBuildPanelLinkNoHashNoPs(t *testing.T) {
	link, err := BuildPanelLink(PanelLinkParams{
		Host: "example.com", Password: "secret", UserName: "user1", VpnName: "MyVPN",
		DtlsPort: 56000, SubURL: "https://example.com/sub/x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(link, "hash") {
		t.Fatalf("panel link JSON must not contain hash field")
	}
	p, err := Decode(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Hash != "" {
		t.Fatalf("hash must be empty, got %q", p.Hash)
	}
	if p.Name != "user1" || p.Vpn != "MyVPN" || p.IP != "example.com" || p.Pass != "secret" {
		t.Fatalf("unexpected payload: %+v", p)
	}
}

func TestDecodeLegacyPs(t *testing.T) {
	link, err := BuildBotLink(BotLinkParams{
		Host: "h.example", Password: "p", Remark: "legacy", VkHash: "abc123", DtlsPort: 56000,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := Decode(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Ps != "legacy" && p.Name != "legacy" {
		t.Fatalf("expected legacy name, got %+v", p)
	}
	if p.Hash != "abc123" {
		t.Fatalf("bot hash: %q", p.Hash)
	}
}
