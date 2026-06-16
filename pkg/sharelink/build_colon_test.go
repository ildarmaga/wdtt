package sharelink

import (
	"strings"
	"testing"
)

func TestBuildColonLink(t *testing.T) {
	link := BuildColonLink(ColonLinkParams{
		Host: "1.2.3.4", Password: "secret", VkHash: "abc123",
		DtlsPort: 56000, WgPort: 56001, LocalPort: 9000, HashLimit: 1,
	})
	want := "wdtt://1.2.3.4:56000:56001:9000:secret:abc123"
	if link != want {
		t.Fatalf("got %q want %q", link, want)
	}
	withName := BuildColonLink(ColonLinkParams{
		Host: "1.2.3.4", Password: "secret", Name: "User1", VkHash: "h1,h2",
		DtlsPort: 56000, WgPort: 56001, WithName: true,
	})
	if withName != "wdtt://1.2.3.4:56000:56001:0:secret:h1,h2#User1" {
		t.Fatalf("with name: %q", withName)
	}
}

func TestBuildQwdttLink(t *testing.T) {
	link := BuildQwdttLink(QwdttLinkParams{
		Host: "peer.example", Password: "f1k-23&-89@", Name: "MyVPN", VkHash: "hash1",
		Port: 9000, Workers: 18,
	})
	if link == "" {
		t.Fatal("empty link")
	}
	if !strings.HasSuffix(link, "&pass=f1k-23&-89@") {
		t.Fatalf("pass must be raw at end, got %q", link)
	}
	if strings.Contains(link, "%26") || strings.Contains(link, "%40") {
		t.Fatalf("pass must not be encoded: %q", link)
	}
}
