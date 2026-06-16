package panel

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
)

const vkProxyUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"

var vkProxyHosts = []string{
	"vk.com", "m.vk.com", "login.vk.com", "id.vk.com", "oauth.vk.com",
	"vk.ru", "m.vk.ru", "api.vk.com", "static.vk.com", "static.vk.ru",
	"st.vk.com", "vkuser.net", "userapi.com", "static.vkontakte.com",
	"ads.vk.com", "persiq.vk.com", "persiq.vk.ru",
}

var vkAssetRootPrefixes = []string{
	"/js/", "/dist/", "/css/", "/fonts/", "/images/", "/video/", "/audio/", "/mu/", "/mrong/",
}

var (
	vkURLRewrite = regexp.MustCompile(`(?i)(https?:)?//([a-z0-9.-]+\.(?:vk\.com|vk\.ru|vkuser\.net|userapi\.com|vkontakte\.com))(/[^"'>\s\\]*)?`)
	vkBodyTagRe  = regexp.MustCompile(`(?i)<body[^>]*>`)
	vkHeadTagRe  = regexp.MustCompile(`(?i)<head[^>]*>`)
)

type vkLoginState struct {
	mu       sync.Mutex
	jar      *cookiejar.Jar
	client   *http.Client
	lastHost string
}

var (
	vkLoginMu       sync.Mutex
	vkLoginSessions = map[string]*vkLoginState{}
)

func vkLoginPrefix(base string) string {
	return strings.TrimSuffix(base, "/") + "/panel/vk/login/"
}

func getVKLoginState(user string) (*vkLoginState, error) {
	vkLoginMu.Lock()
	defer vkLoginMu.Unlock()
	if st, ok := vkLoginSessions[user]; ok {
		return st, nil
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	st := &vkLoginState{
		jar: jar,
		client: &http.Client{
			Jar:     jar,
			Timeout: 0,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		lastHost: "vk.com",
	}
	vkLoginSessions[user] = st
	return st, nil
}

func clearVKLoginState(user string) {
	vkLoginMu.Lock()
	delete(vkLoginSessions, user)
	vkLoginMu.Unlock()
}

func vkAuthStatus(user string) map[string]interface{} {
	st, err := getVKLoginState(user)
	if err != nil {
		return map[string]interface{}{"logged_in": false, "has_remixsid": false}
	}
	ok := vkJarHasRemixsid(st.jar)
	return map[string]interface{}{
		"logged_in":    ok,
		"has_remixsid": ok,
	}
}

func vkJarHasRemixsid(jar *cookiejar.Jar) bool {
	if jar == nil {
		return false
	}
	for _, raw := range []string{"https://vk.com/", "https://vk.ru/", "https://login.vk.com/"} {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, c := range jar.Cookies(u) {
			if c.Name == vkAuthCookieName && strings.TrimSpace(c.Value) != "" {
				return true
			}
		}
	}
	return false
}

func exportVKCookiesFromJar(jar *cookiejar.Jar) ([]byte, error) {
	if jar == nil {
		return nil, fmt.Errorf("сессия VK login пуста")
	}
	seen := map[string]vkCookieEntry{}
	for _, raw := range []string{
		"https://vk.com/", "https://vk.ru/", "https://login.vk.com/", "https://id.vk.com/",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, c := range jar.Cookies(u) {
			if strings.TrimSpace(c.Value) == "" {
				continue
			}
			seen[c.Name] = vkCookieEntry{Name: c.Name, Value: c.Value}
		}
	}
	if _, ok := seen[vkAuthCookieName]; !ok {
		return nil, fmt.Errorf("remixsid не найден — завершите вход в VK")
	}
	out := make([]vkCookieEntry, 0, len(seen))
	for _, c := range seen {
		out = append(out, c)
	}
	return json.Marshal(out)
}

func saveVKCookiesFromLogin(user string) error {
	st, err := getVKLoginState(user)
	if err != nil {
		return err
	}
	data, err := exportVKCookiesFromJar(st.jar)
	if err != nil {
		return err
	}
	return saveVKCookies(data)
}

func parseVKProxyTarget(r *http.Request, prefix string) (*url.URL, error) {
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		u, err := url.Parse("https://login.vk.com/?act=login")
		if err != nil {
			return nil, err
		}
		if q := r.URL.RawQuery; q != "" {
			u.RawQuery = q
		}
		return u, nil
	}
	if strings.HasPrefix(rest, "h/") {
		rest = rest[2:]
		slash := strings.Index(rest, "/")
		host := rest
		p := "/"
		if slash >= 0 {
			host = rest[:slash]
			p = rest[slash:]
			if p == "" {
				p = "/"
			}
		}
		if !vkHostAllowed(host) {
			return nil, fmt.Errorf("host not allowed: %s", host)
		}
		u, err := url.Parse("https://" + host + p)
		if err != nil {
			return nil, err
		}
		u.RawQuery = r.URL.RawQuery
		return u, nil
	}
	u, err := url.Parse("https://vk.com/" + rest)
	if err != nil {
		return nil, err
	}
	u.RawQuery = r.URL.RawQuery
	return u, nil
}

func vkHostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, h := range vkProxyHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

func vkHostFromReferer(referer, base string) string {
	if referer == "" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	prefix := vkLoginPrefix(base)
	marker := prefix + "h/"
	idx := strings.Index(u.Path, marker)
	if idx < 0 {
		return ""
	}
	rest := u.Path[idx+len(marker):]
	slash := strings.Index(rest, "/")
	host := rest
	if slash >= 0 {
		host = rest[:slash]
	}
	if vkHostAllowed(host) {
		return host
	}
	return ""
}

func vkResolveAssetHost(r *http.Request, base string, st *vkLoginState) string {
	if host := vkHostFromReferer(r.Header.Get("Referer"), base); host != "" {
		return vkCDNHost(host)
	}
	st.mu.Lock()
	host := st.lastHost
	st.mu.Unlock()
	return vkCDNHost(host)
}

func rewriteVKURL(raw, proxyPrefix string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	if !vkHostAllowed(u.Host) {
		return raw
	}
	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		p += "#" + u.Fragment
	}
	return proxyPrefix + "h/" + u.Host + p
}

func rewriteVKAbsoluteURLs(body, proxyPrefix string) string {
	return vkURLRewrite.ReplaceAllStringFunc(body, func(m string) string {
		sub := vkURLRewrite.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		host := sub[2]
		p := sub[3]
		if p == "" {
			p = "/"
		}
		return proxyPrefix + "h/" + host + p
	})
}

func vkCDNHost(pageHost string) string {
	pageHost = strings.ToLower(strings.TrimSpace(pageHost))
	if strings.HasPrefix(pageHost, "st") && strings.HasSuffix(pageHost, ".vk.com") {
		return pageHost
	}
	return "st1-15.vk.com"
}

func rewriteVKRootPaths(body, proxyPrefix, host string) string {
	cdn := vkCDNHost(host)
	base := proxyPrefix + "h/" + cdn
	for _, p := range vkAssetRootPrefixes {
		repl := base + p
		body = strings.ReplaceAll(body, `"`+p, `"`+repl)
		body = strings.ReplaceAll(body, "'"+p, "'"+repl)
		body = strings.ReplaceAll(body, "("+p, "("+repl)
		body = strings.ReplaceAll(body, " url("+p, " url("+repl)
	}
	return body
}

func rewriteVKText(body, proxyPrefix, host string) string {
	body = rewriteVKAbsoluteURLs(body, proxyPrefix)
	body = rewriteVKRootPaths(body, proxyPrefix, host)
	return body
}

func injectVKProxyHooks(html, proxyPrefix, host string) string {
	escPrefix, escHost := jsonEscape(proxyPrefix), jsonEscape(host)
	script := `<script>(function(){var P=` + escPrefix + `,H=` + escHost + `;window.__wdtt_vk_host=H;function map(u){if(!u)return u;if(typeof u!=="string")return u;if(u.charAt(0)==="/"&&u.charAt(1)!=="/")return P+"h/"+H+u;try{var x=new URL(u,location.href);if(/\.(vk\.com|vk\.ru|vkuser\.net|userapi\.com|vkontakte\.com)$/i.test(x.hostname))return P+"h/"+x.hostname+x.pathname+x.search+x.hash;}catch(e){}return u;}var f=window.fetch;window.fetch=function(i,o){if(typeof i==="string")i=map(i);else if(i&&i.url)i=new Request(map(i.url),i);return f.call(this,i,o);};var xo=XMLHttpRequest.prototype.open;XMLHttpRequest.prototype.open=function(m,u){arguments[1]=map(u);return xo.apply(this,arguments);};try{var d=Object.getOwnPropertyDescriptor(Document.prototype,"domain");if(!d||d.configurable)Object.defineProperty(document,"domain",{configurable:true,get:function(){return location.hostname;},set:function(){}});}catch(e){}})();</script>`
	loc := vkHeadTagRe.FindStringIndex(html)
	if loc == nil {
		return script + html
	}
	pos := loc[1]
	if pos < 0 || pos > len(html) {
		return script + html
	}
	return html[:pos] + script + html[pos:]
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func injectVKLoginBanner(html string) string {
	banner := `<div id="wdtt-vk-login-bar" style="position:sticky;top:0;z-index:999999;background:#1677ff;color:#fff;padding:8px 12px;font:14px/1.4 sans-serif;text-align:center">WDTT VK login: classic vk.com form (VK ID does not work on panel domain). After sign-in click Save cookies in Settings.</div>`
	loc := vkBodyTagRe.FindStringIndex(html)
	if loc == nil {
		return banner + html
	}
	pos := loc[1]
	if pos < 0 || pos > len(html) {
		return banner + html
	}
	return html[:pos] + banner + html[pos:]
}

func rewriteVKBody(body []byte, proxyPrefix, host, contentType string) []byte {
	s := string(body)
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "text/html"):
		s = rewriteVKText(s, proxyPrefix, host)
		s = injectVKProxyHooks(s, proxyPrefix, host)
		s = injectVKLoginBanner(s)
	case strings.Contains(ct, "javascript"), strings.Contains(ct, "json"), strings.Contains(ct, "text/css"):
		s = rewriteVKText(s, proxyPrefix, host)
	default:
		return body
	}
	return []byte(s)
}

func vkResponseNeedsRewrite(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/html") ||
		strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "text/css")
}

func readUpstreamBody(resp *http.Response) ([]byte, error) {
	var r io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		r = gr
	}
	return io.ReadAll(r)
}

func vkGuessContentType(name, upstream string) string {
	if upstream != "" && !strings.HasPrefix(strings.ToLower(upstream), "text/plain") {
		return upstream
	}
	if ext := path.Ext(name); ext != "" {
		if t := mime.TypeByExtension(ext); t != "" {
			return t
		}
	}
	return upstream
}

func (a *App) proxyVKLogin(w http.ResponseWriter, r *http.Request) {
	sess := a.parseSession(r)
	if sess == nil {
		http.Redirect(w, r, a.cfg.basePath(), http.StatusFound)
		return
	}
	st, err := getVKLoginState(sess.User)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	prefix := vkLoginPrefix(a.cfg.basePath())
	target, err := parseVKProxyTarget(r, prefix)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	st.mu.Lock()
	st.lastHost = target.Host
	st.mu.Unlock()

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", vkProxyUserAgent)
	req.Header.Set("Accept", r.Header.Get("Accept"))
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Accept-Encoding", "identity")
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		req.Header.Set("Referer", rewriteVKURL(ref, prefix))
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		req.Header.Set("Origin", "https://"+target.Host)
	}

	resp, err := st.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if loc := resp.Header.Get("Location"); loc != "" {
		if rl := rewriteVKURL(loc, prefix); rl != loc {
			copyHeader(w.Header(), resp.Header)
			w.Header().Set("Location", rl)
			w.WriteHeader(resp.StatusCode)
			return
		}
	}

	ct := resp.Header.Get("Content-Type")
	if !vkResponseNeedsRewrite(ct) {
		copyHeader(w.Header(), resp.Header)
		w.Header().Set("Content-Type", vkGuessContentType(target.Path, ct))
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}

	body, err := readUpstreamBody(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	body = rewriteVKBody(body, prefix, target.Host, ct)

	hdr := w.Header()
	copyHeader(hdr, resp.Header)
	hdr.Del("Content-Encoding")
	hdr.Del("Content-Length")
	hdr.Set("Content-Type", vkGuessContentType(target.Path, ct))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func (a *App) handleVKLoginAssetFallback(w http.ResponseWriter, r *http.Request) {
	sess := a.parseSession(r)
	if sess == nil {
		http.NotFound(w, r)
		return
	}
	st, err := getVKLoginState(sess.User)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	host := vkResolveAssetHost(r, a.cfg.basePath(), st)
	prefix := vkLoginPrefix(a.cfg.basePath())
	r2 := r.Clone(r.Context())
	r2.URL.Path = prefix + "h/" + host + r.URL.Path
	a.proxyVKLogin(w, r2)
}

func copyHeader(dst, src http.Header) {
	skip := map[string]bool{
		"content-encoding":                   true,
		"content-length":                     true,
		"content-security-policy":            true,
		"content-security-policy-report-only": true,
		"x-frame-options":                    true,
		"cross-origin-opener-policy":         true,
		"cross-origin-embedder-policy":       true,
		"cross-origin-resource-policy":       true,
	}
	for k, vv := range src {
		if skip[strings.ToLower(k)] {
			continue
		}
		for _, v := range vv {
			if strings.EqualFold(k, "Location") {
				continue
			}
			dst.Add(k, v)
		}
	}
}

func registerVKLoginAssetFallbacks(mux *http.ServeMux, app *App) {
	for _, p := range vkAssetRootPrefixes {
		prefix := p
		mux.HandleFunc(prefix, app.handleVKLoginAssetFallback)
	}
	mux.HandleFunc("/js/sw.js", app.handleVKLoginAssetFallback)
}
