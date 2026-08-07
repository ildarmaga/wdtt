package panel

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestParseLoginFormPercentDecodesPassword(t *testing.T) {
	body := url.Values{
		"username": {"admin"},
		"password": {"P@ss w0rd%"},
	}.Encode()
	req, err := http.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	form, err := parseLoginForm(req)
	if err != nil {
		t.Fatal(err)
	}
	if form.Username != "admin" {
		t.Fatalf("username=%q", form.Username)
	}
	if form.Password != "P@ss w0rd%" {
		t.Fatalf("password not decoded: got %q", form.Password)
	}
}
