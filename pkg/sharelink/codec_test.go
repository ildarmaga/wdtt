package sharelink

import (
	"testing"
)

func TestBuildPanelLinkWithHash(t *testing.T) {
	link, err := BuildPanelLink(PanelLinkParams{
		Host: "example.com", Password: "secret", UserName: "user1", VpnName: "MyVPN",
		VkHash: "hash1,hash2", DtlsPort: 56000, SubURL: "https://example.com/sub/x",
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := Decode(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Hash != "hash1,hash2" {
		t.Fatalf("hash: got %q", p.Hash)
	}
	if p.Name != "user1" || p.Vpn != "MyVPN" {
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
