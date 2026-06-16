package panel

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHandleVKCreatorSaveCookiesForm(t *testing.T) {
	cookies := `[{"name":"remixsid","value":"abc123"}]`
	form := url.Values{"cookies_json": {cookies}}
	req := httptest.NewRequest("POST", "/panel/api/vk/cookies", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Simulate the old bug: ParseMultipartForm consumes urlencoded body then fails.
	_ = req.ParseMultipartForm(1 << 20)
	if req.Form == nil {
		t.Fatal("expected Form populated after ParseMultipartForm attempt")
	}
	if got := req.Form.Get("cookies_json"); got != cookies {
		t.Fatalf("Form cookies_json=%q", got)
	}

	// Handler path: ParseForm after multipart attempt must still work.
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}
	raw := strings.TrimSpace(req.Form.Get("cookies_json"))
	if raw != cookies {
		t.Fatalf("got %q", raw)
	}
}

func TestExtractCookiesPayloadFormEncoded(t *testing.T) {
	cookies := `[{"name":"remixsid","value":"1_b_test"}]`
	body := url.Values{"cookies_json": {cookies}}.Encode()
	raw := extractCookiesPayload([]byte(body))
	if raw != cookies {
		t.Fatalf("got %q", raw)
	}
}
