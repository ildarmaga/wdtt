package panel

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestExportVKCookiesFromJar(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	u := mustParseURL(t, "https://vk.com/")
	jar.SetCookies(u, []*http.Cookie{{Name: "remixsid", Value: "abc123"}, {Name: "remixlang", Value: "0"}})
	data, err := exportVKCookiesFromJar(jar)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateVKCookiesJSON(data); err != nil {
		t.Fatal(err)
	}
}

func TestVKJarHasRemixsid(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	if vkJarHasRemixsid(jar) {
		t.Fatal("expected false on empty jar")
	}
	jar.SetCookies(mustParseURL(t, "https://vk.com/"), []*http.Cookie{{Name: "remixsid", Value: "x"}})
	if !vkJarHasRemixsid(jar) {
		t.Fatal("expected remixsid")
	}
}

func TestRewriteVKURL(t *testing.T) {
	prefix := "/wdtt/panel/vk/login/"
	got := rewriteVKURL("https://login.vk.com/?act=login", prefix)
	want := prefix + "h/login.vk.com/?act=login"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestInjectVKLoginBanner_LargeHTML(t *testing.T) {
	html := "<!DOCTYPE html><html><head>" + strings.Repeat("<!-- İ comment -->\n", 5000) + "</head><BODY>\n" + strings.Repeat("a", 50000) + "</body></html>"
	out := injectVKLoginBanner(html)
	if !strings.Contains(out, "wdtt-vk-login-bar") {
		t.Fatal("banner missing")
	}
	if len(out) <= len(html) {
		t.Fatal("expected banner to increase size")
	}
}

func TestVKCDNHost(t *testing.T) {
	if got := vkCDNHost("id.vk.com"); got != "st1-15.vk.com" {
		t.Fatalf("got %q", got)
	}
	if got := vkCDNHost("st1-21.vk.com"); got != "st1-21.vk.com" {
		t.Fatalf("got %q", got)
	}
}

func TestRewriteVKRootPaths(t *testing.T) {
	prefix := "/wdtt/panel/vk/login/"
	host := "st1-15.vk.com"
	in := `<script src="/js/loader.js"></script><link href="/dist/web/x.css">`
	out := rewriteVKRootPaths(in, prefix, host)
	wantJS := prefix + "h/st1-15.vk.com/js/loader.js"
	wantCSS := prefix + "h/st1-15.vk.com/dist/web/x.css"
	if !strings.Contains(out, wantJS) {
		t.Fatalf("js not rewritten: %s", out)
	}
	if !strings.Contains(out, wantCSS) {
		t.Fatalf("css not rewritten: %s", out)
	}
}

func TestVKHostFromReferer(t *testing.T) {
	base := "/wdtt/"
	ref := "https://dev.example.com:2860/wdtt/panel/vk/login/h/st1-15.vk.com/dist/web/x.js"
	got := vkHostFromReferer(ref, base)
	if got != "st1-15.vk.com" {
		t.Fatalf("got %q", got)
	}
}

func TestInjectVKProxyHooks(t *testing.T) {
	html := "<html><head></head><body></body></html>"
	out := injectVKProxyHooks(html, "/wdtt/panel/vk/login/", "vk.com")
	if !strings.Contains(out, "window.fetch") {
		t.Fatal("fetch hook missing")
	}
	if !strings.Contains(out, "__wdtt_vk_host") {
		t.Fatal("host var missing")
	}
}

func TestRewriteVKBody_NoPanic(t *testing.T) {
	prefix := "/wdtt/panel/vk/login/"
	html := "<html><head><script>var hint = \"use //vk.com/path\";</script></head><BODY><a href=\"https://login.vk.com/?act=login\">login</a></BODY></html>"
	out := rewriteVKBody([]byte(html), prefix, "vk.com", "text/html; charset=utf-8")
	s := string(out)
	if !strings.Contains(s, "wdtt-vk-login-bar") {
		t.Fatal("banner missing")
	}
	if !strings.Contains(s, prefix+"h/login.vk.com/?act=login") {
		t.Fatalf("login url not rewritten: %s", s)
	}
}

func TestRewriteVKBody_JavaScript(t *testing.T) {
	prefix := "/wdtt/panel/vk/login/"
	src := `fetch("https://api.vk.com/method/users.get?v=5.131");`
	out := string(rewriteVKBody([]byte(src), prefix, "st1-15.vk.com", "application/javascript"))
	if !strings.Contains(out, prefix+"h/api.vk.com/method/users.get?v=5.131") {
		t.Fatalf("js url not rewritten: %s", out)
	}
}

func TestProxyVKLoginIntegration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<html><head></head><body><form action="https://login.vk.com/?act=login" method="post">ok</form></body></html>`)
	}))
	defer upstream.Close()

	clearVKLoginState("admin")
	cfg := &PanelConfig{WebBasePath: "/wdtt/", SessionKey: "test-session-key"}
	app := &App{cfg: cfg}

	upURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	st, err := getVKLoginState("admin")
	if err != nil {
		t.Fatal(err)
	}
	st.client = &http.Client{
		Jar: st.jar,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "vk.com" {
				req.URL.Scheme = upURL.Scheme
				req.URL.Host = upURL.Host
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	mux := http.NewServeMux()
	base := cfg.basePath()
	mux.HandleFunc(base+"panel/vk/login/", app.handleVKLoginProxy)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	reqURL := strings.TrimSuffix(srv.URL, "/") + base + "panel/vk/login/h/vk.com/"
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: makeTestSessionToken(t, cfg, "admin")})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
	s := string(body)
	if !strings.Contains(s, "wdtt-vk-login-bar") {
		t.Fatalf("missing banner: %s", s)
	}
	want := vkLoginPrefix(base) + "h/login.vk.com/?act=login"
	if !strings.Contains(s, want) {
		t.Fatalf("form action not rewritten, want %q in %s", want, s)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func makeTestSessionToken(t *testing.T, cfg *PanelConfig, user string) string {
	t.Helper()
	app := &App{cfg: cfg}
	rec := httptest.NewRecorder()
	app.createSession(rec, user)
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			return c.Value
		}
	}
	t.Fatal("session cookie not created")
	return ""
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
