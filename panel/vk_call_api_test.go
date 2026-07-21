package panel

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestVKParseAPIError(t *testing.T) {
	err := vkParseAPIError([]byte(`{"error":{"error_code":5,"error_msg":"auth"}}`))
	if err == nil || !strings.Contains(err.Error(), "VK API 5") {
		t.Fatalf("got %v", err)
	}
	if vkParseAPIError([]byte(`{"response":{"id":1}}`)) != nil {
		t.Fatal("expected nil for ok response")
	}
}

func TestVKCreateCallLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case strings.Contains(r.URL.String(), "web_token"):
			_, _ = w.Write([]byte(`{"data":{"access_token":"tok"}}`))
		case strings.Contains(r.URL.Path, "users.get"):
			_, _ = w.Write([]byte(`{"response":[{"id":12345}]}`))
		case strings.Contains(r.URL.Path, "calls.start"):
			if r.Form.Get("peer_id") != "12345" {
				t.Fatalf("peer_id=%q", r.Form.Get("peer_id"))
			}
			_, _ = w.Write([]byte(`{"response":{"call_id":"c1","join_link":"https://vk.com/call/join/abc123"}}`))
		default:
			t.Fatalf("unexpected %s", r.URL.String())
		}
	}))
	defer srv.Close()

	old := vkCallHTTPClient
	vkCallHTTPClient = srv.Client()
	defer func() { vkCallHTTPClient = old }()

	rewrite := func(u string) string {
		if strings.HasPrefix(u, "https://login.vk.ru") {
			return srv.URL + "/login?" + strings.SplitN(u, "?", 2)[1]
		}
		if strings.HasPrefix(u, "https://api.vk.ru/method/") {
			method := strings.TrimPrefix(u, "https://api.vk.ru/method/")
			if i := strings.Index(method, "?"); i >= 0 {
				return srv.URL + "/" + method[:i] + "?" + method[i+1:]
			}
			return srv.URL + "/" + method
		}
		return u
	}

	oldPost := vkHTTPPostDo
	vkHTTPPostDo = func(endpoint string, form url.Values, headers map[string]string) ([]byte, error) {
		return oldPost(rewrite(endpoint), form, headers)
	}
	defer func() { vkHTTPPostDo = oldPost }()

	got, err := vkCreateCallLink("remixsid=test")
	if err != nil {
		t.Fatal(err)
	}
	if got.CallID != "c1" || got.JoinLink != "https://vk.com/call/join/abc123" {
		t.Fatalf("got %+v", got)
	}
}

func TestVKForceFinishCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case strings.Contains(r.URL.String(), "web_token"):
			_, _ = w.Write([]byte(`{"data":{"access_token":"tok"}}`))
		case strings.Contains(r.URL.Path, "calls.forceFinish"):
			if r.Form.Get("call_id") != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
				t.Fatalf("call_id=%q", r.Form.Get("call_id"))
			}
			_, _ = w.Write([]byte(`{"response":1}`))
		default:
			t.Fatalf("unexpected %s", r.URL.String())
		}
	}))
	defer srv.Close()
	old := vkCallHTTPClient
	vkCallHTTPClient = srv.Client()
	defer func() { vkCallHTTPClient = old }()
	oldPost := vkHTTPPostDo
	vkHTTPPostDo = func(endpoint string, form url.Values, headers map[string]string) ([]byte, error) {
		u := endpoint
		if strings.HasPrefix(u, "https://login.vk.ru") {
			u = srv.URL + "/login?" + strings.SplitN(u, "?", 2)[1]
		} else if strings.HasPrefix(u, "https://api.vk.ru/method/") {
			u = srv.URL + "/" + strings.TrimPrefix(u, "https://api.vk.ru/method/")
		}
		return oldPost(u, form, headers)
	}
	defer func() { vkHTTPPostDo = oldPost }()
	if err := vkForceFinishCall("remixsid=test", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"); err != nil {
		t.Fatal(err)
	}
}

func TestVKCallAlive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.String(), "get_anonym_token") {
			_, _ = w.Write([]byte(`{"data":{"access_token":"anon"}}`))
			return
		}
		if strings.Contains(r.URL.Path, "calls.getCallPreview") {
			_, _ = w.Write([]byte(`{"response":{"ok_join_link":"token"}}`))
			return
		}
		t.Fatalf("unexpected %s", r.URL.String())
	}))
	defer srv.Close()

	old := vkCallHTTPClient
	vkCallHTTPClient = srv.Client()
	defer func() { vkCallHTTPClient = old }()

	oldPost := vkHTTPPostDo
	vkHTTPPostDo = func(endpoint string, form url.Values, headers map[string]string) ([]byte, error) {
		u := endpoint
		if strings.HasPrefix(u, "https://login.vk.ru") {
			u = srv.URL + "/login?" + strings.SplitN(u, "?", 2)[1]
		} else if strings.HasPrefix(u, "https://api.vk.ru/method/") {
			method := strings.TrimPrefix(u, "https://api.vk.ru/method/")
			u = srv.URL + "/" + method
		}
		return oldPost(u, form, headers)
	}
	defer func() { vkHTTPPostDo = oldPost }()

	if !vkCallAlive("https://vk.com/call/join/abc") {
		t.Fatal("expected alive")
	}
}
