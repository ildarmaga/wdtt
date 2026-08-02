package panel

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

func TestMainUserRowIncludesVkHashAndPorts(t *testing.T) {
	db := &PasswordsDB{
		MainPassword: "mainpass",
		Passwords: map[string]*PasswordEntry{
			"mainpass": {
				VkHash: "hhh1",
				Ports:  "56100,56101,9100",
				SubID:  "submain",
			},
		},
	}
	inbound := defaultWdttInbound()
	inbound.DtlsPort = 56000
	inbound.WgPort = 56001
	inbound.ClientPort = 9000
	row := mainUserRow(db, nil, inbound, "1.2.3.4", "WDTT", "https://example/sub/submain")
	if row["vk_hash"] != "hhh1" {
		t.Fatalf("vk_hash=%v", row["vk_hash"])
	}
	if row["ports"] != "56100,56101,9100" {
		t.Fatalf("ports=%v", row["ports"])
	}
	if row["dtls_port"] != 56100 {
		t.Fatalf("dtls_port=%v", row["dtls_port"])
	}
	link, _ := row["link"].(string)
	if !strings.HasPrefix(link, "wdtt://") {
		t.Fatalf("link=%q", link)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(link, "wdtt://"))
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["hash"] != "hhh1" {
		t.Fatalf("decoded hash=%v body=%s", obj["hash"], raw)
	}
	if obj["sub"] != "https://example/sub/submain" {
		t.Fatalf("sub=%v", obj["sub"])
	}
	_ = paneldb.MainUserComment
}
