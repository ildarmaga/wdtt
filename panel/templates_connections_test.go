package panel

import (
	"strings"
	"testing"
)

func TestConnectionsHTMLHasNoWBTemplates(t *testing.T) {
	raw, err := htmlFS.ReadFile("web/html/connections.html")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, bad := range []string{
		`modals/wbCookiesModal`,
		`modals/wbInboundModal`,
		`WB Stream`,
		`wbCookiesModal`,
		`loadWb(`,
	} {
		if strings.Contains(body, bad) {
			t.Fatalf("public connections.html must not reference %q", bad)
		}
	}
	if !strings.Contains(body, `modals/wdttInboundModal`) {
		t.Fatal("expected wdtt inbound modal")
	}
}

func TestInitTemplatesLoadsConnections(t *testing.T) {
	if err := initTemplates(); err != nil {
		t.Fatal(err)
	}
	if htmlTemplates.Lookup("connections.html") == nil {
		t.Fatal("connections.html not registered")
	}
	if htmlTemplates.Lookup("modals/wdttInboundModal") == nil {
		t.Fatal("wdttInboundModal missing")
	}
}
