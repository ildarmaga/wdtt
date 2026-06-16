package panel

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func TestVkRawJSONString(t *testing.T) {
	if got := vkRawJSONString(json.RawMessage(`"123"`)); got != "123" {
		t.Fatalf("string: got %q", got)
	}
	if got := vkRawJSONString(json.RawMessage(`825644198168`)); got != "825644198168" {
		t.Fatalf("number: got %q", got)
	}
}

func TestVkOAuthPasswordLogin_InvalidCredentials(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := vkLoginHTTPClient(jar)
	if err := vkWarmupLoginSession(client); err != nil {
		t.Fatal(err)
	}
	out := vkOAuthPasswordLogin(client, jar, "+70000000000", "wrong-password-xyz", vkPasswordLoginInput{})
	if out.Status != "error" {
		t.Fatalf("expected error status, got %+v", out)
	}
	if out.Message == "" {
		t.Fatal("expected error message")
	}
}

func TestVkPasswordLogin_MissingFields(t *testing.T) {
	st, err := getVKLoginState("test-missing-fields")
	if err != nil {
		t.Fatal(err)
	}
	defer clearVKLoginState("test-missing-fields")
	out, err := vkPasswordLogin(st, vkPasswordLoginInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "error" || out.Message == "" {
		t.Fatalf("unexpected outcome: %+v", out)
	}
}

func TestParseVKProxyTarget_DefaultLogin(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.com/wdtt/panel/vk/login/", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := parseVKProxyTarget(req, "/wdtt/panel/vk/login/")
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "login.vk.com" || u.Query().Get("act") != "login" {
		t.Fatalf("got %s", u.String())
	}
}
