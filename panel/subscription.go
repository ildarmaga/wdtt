package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"

	"wdtt-panel/network"
)

const subIDChars = "abcdefghijklmnopqrstuvwxyz0123456789"

func genSubID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 16)
	for i := range out {
		out[i] = subIDChars[int(b[i])%len(subIDChars)]
	}
	return string(out), nil
}

func normalizeSubPath(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		p = "/sub/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return strings.ReplaceAll(p, "//", "/")
}

type subUserInfo struct {
	Password string
	Entry    *PasswordEntry
	Email    string
}

func lookupUserBySubID(subID string) (*subUserInfo, error) {
	subID = strings.TrimSpace(subID)
	if subID == "" {
		return nil, fmt.Errorf("empty sub id")
	}
	if !panelDBEnabled() {
		return nil, fmt.Errorf("database unavailable")
	}
	var pass, subCol string
	var e PasswordEntry
	var deactivated int
	err := panelDB.QueryRow(`SELECT password, sub_id, device_id, max_devices, expires_at, down_bytes, up_bytes,
		total_bytes, max_down_mbps, max_up_mbps, is_deactivated, comment, ports, vk_hash
		FROM wdtt_users WHERE sub_id = ?`, subID).Scan(
		&pass, &subCol, &e.DeviceID, &e.MaxDevices, &e.ExpiresAt, &e.DownBytes, &e.UpBytes,
		&e.TotalBytes, &e.MaxDownMBps, &e.MaxUpMBps, &deactivated, &e.Comment, &e.Ports, &e.VkHash,
	)
	if err != nil {
		return nil, fmt.Errorf("subscription not found")
	}
	e.SubID = subCol
	e.IsDeactivated = deactivated != 0
	if e.IsDeactivated || isPasswordExpired(&e) || trafficExceeded(&e) {
		return nil, fmt.Errorf("subscription inactive")
	}
	email := strings.TrimSpace(e.Comment)
	if email == "" {
		email = pass
	}
	return &subUserInfo{Password: pass, Entry: &e, Email: email}, nil
}

func (a *App) buildSubURL(subID string) string {
	subID = strings.TrimSpace(subID)
	if subID == "" {
		return ""
	}
	path := normalizeSubPath(a.cfg.SubPath)
	if uri := strings.TrimSpace(a.cfg.SubURI); uri != "" {
		return strings.TrimRight(uri, "/") + "/" + subID
	}
	scheme, host := a.subPublicEndpoint()
	return fmt.Sprintf("%s://%s%s%s", scheme, host, path, subID)
}

func (a *App) subPublicEndpoint() (scheme, hostWithPort string) {
	scheme = "http"
	certFile := strings.TrimSpace(a.cfg.SubCertFile)
	keyFile := strings.TrimSpace(a.cfg.SubKeyFile)
	if certFile == "" && keyFile == "" {
		certFile = strings.TrimSpace(a.cfg.WebCertFile)
		keyFile = strings.TrimSpace(a.cfg.WebKeyFile)
	}
	if certFile != "" && keyFile != "" {
		scheme = "https"
	}
	host := strings.TrimSpace(a.cfg.SubDomain)
	if host == "" {
		host = strings.TrimSpace(a.cfg.WebDomain)
	}
	if host == "" {
		inbound, _ := loadWdttInbound()
		host = a.resolveLinkHost(inbound)
	}
	port := a.cfg.SubPort
	if port <= 0 {
		port = 2096
	}
	if (scheme == "https" && port == 443) || (scheme == "http" && port == 80) {
		return scheme, host
	}
	return scheme, net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

func subListenAddr(cfg *PanelConfig) string {
	port := cfg.SubPort
	if port <= 0 {
		port = 2096
	}
	listen := strings.TrimSpace(cfg.SubListen)
	if listen == "" {
		return fmt.Sprintf(":%d", port)
	}
	return net.JoinHostPort(listen, fmt.Sprintf("%d", port))
}

func startSubscriptionServer(app *App) {
	cfg := app.cfg
	if cfg == nil || !cfg.SubEnable {
		return
	}
	go func() {
		mux := http.NewServeMux()
		path := normalizeSubPath(cfg.SubPath)
		mux.HandleFunc(path, app.handleSubscription)
		mux.HandleFunc(strings.TrimSuffix(path, "/"), func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, path, http.StatusMovedPermanently)
		})

		addr := subListenAddr(cfg)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			log.Printf("subscription listen %s: %v", addr, err)
			return
		}

		certFile := strings.TrimSpace(cfg.SubCertFile)
		keyFile := strings.TrimSpace(cfg.SubKeyFile)
		scheme := "http"
		if certFile != "" && keyFile != "" {
			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				log.Printf("subscription TLS: %v", err)
				return
			}
			listener = network.NewMuxListener(listener, &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			})
			scheme = "https"
		}
		log.Printf("WDTT Subscription: %s://%s%s", scheme, listener.Addr().String(), path)
		if err := http.Serve(listener, mux); err != nil {
			log.Printf("subscription server: %v", err)
		}
	}()
}

func (a *App) handleSubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := normalizeSubPath(a.cfg.SubPath)
	if !strings.HasPrefix(r.URL.Path, path) {
		http.NotFound(w, r)
		return
	}
	subID := strings.Trim(strings.TrimPrefix(r.URL.Path, path), "/")
	if subID == "" || strings.Contains(subID, "/") {
		http.NotFound(w, r)
		return
	}

	info, err := lookupUserBySubID(subID)
	if err != nil {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}
	inbound, _ := loadWdttInbound()
	linkHost := a.resolveLinkHost(inbound)
	link, err := buildWdttShareLink(linkHost, info.Password, info.Email, inbound.Tag, "", info.Entry.VkHash, info.Entry, inbound)
	if err != nil {
		http.Error(w, "failed to build link", http.StatusInternalServerError)
		return
	}
	links := []string{link}
	expireSec := int64(0)
	if info.Entry.ExpiresAt > 0 {
		expireSec = info.Entry.ExpiresAt
	}
	header := fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d",
		info.Entry.UpBytes, info.Entry.DownBytes, info.Entry.TotalBytes, expireSec)
	profileURL := strings.TrimSpace(a.cfg.SubProfileURL)
	if profileURL == "" {
		profileURL = a.buildSubURL(subID)
	}
	a.applySubHeaders(w, header)

	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "text/html") || r.URL.Query().Get("html") == "1" || strings.EqualFold(r.URL.Query().Get("view"), "html") {
		a.serveSubInfoPage(w, subID, info, links)
		return
	}

	body := strings.Join(links, "\n")
	if a.cfg.SubEncrypt {
		body = base64.StdEncoding.EncodeToString([]byte(body))
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(body))
	_ = profileURL
}

func (a *App) applySubHeaders(w http.ResponseWriter, userInfoHeader string) {
	if a.cfg.SubShowInfo && userInfoHeader != "" {
		w.Header().Set("Subscription-Userinfo", userInfoHeader)
	}
	if title := strings.TrimSpace(a.cfg.SubTitle); title != "" {
		w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte(title)))
	}
	if support := strings.TrimSpace(a.cfg.SubSupportURL); support != "" {
		w.Header().Set("Profile-Web-Page-Url", support)
	}
	if announce := strings.TrimSpace(a.cfg.SubAnnounce); announce != "" {
		w.Header().Set("Announce", "base64:"+base64.StdEncoding.EncodeToString([]byte(announce)))
	}
	if a.cfg.SubUpdates > 0 {
		w.Header().Set("Profile-Update-Interval", fmt.Sprintf("%d", a.cfg.SubUpdates))
	}
	w.Header().Set("Content-Disposition", "attachment; filename=wdtt")
}

type subPageData struct {
	SubID    string
	SubURL   string
	Email    string
	Upload   string
	Download string
	Total    string
	Used     string
	Expire   string
	Links    []string
	Title    string
}

func (a *App) serveSubInfoPage(w http.ResponseWriter, subID string, info *subUserInfo, links []string) {
	used := trafficUsed(info.Entry)
	page := subPageData{
		SubID:    subID,
		SubURL:   a.buildSubURL(subID),
		Email:    info.Email,
		Upload:   formatBytes(info.Entry.UpBytes),
		Download: formatBytes(info.Entry.DownBytes),
		Total:    formatBytes(info.Entry.TotalBytes),
		Used:     formatBytes(used),
		Expire:   passwordExpiry(info.Entry),
		Links:    links,
		Title:    strings.TrimSpace(a.cfg.SubTitle),
	}
	if page.Title == "" {
		page.Title = "WDTT Subscription"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := subPageTemplate.Execute(w, page); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

var subPageTemplate = template.Must(template.New("sub").Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
body{font-family:system-ui,sans-serif;background:#0a1222;color:rgba(255,255,255,.85);margin:0;padding:24px}
.card{max-width:720px;margin:0 auto;background:#151f31;border-radius:12px;padding:24px}
h1{font-size:1.25rem;margin:0 0 16px}
.row{display:flex;justify-content:space-between;gap:12px;padding:8px 0;border-bottom:1px solid #2c3950}
label{opacity:.7;flex:0 0 auto}
span,code{flex:1;text-align:right}
code{word-break:break-all;font-size:12px}
.btn{display:inline-block;margin-top:12px;padding:8px 14px;background:#008771;color:#fff;border-radius:8px;text-decoration:none}
.qr{margin-top:16px;text-align:center}
</style>
</head>
<body>
<div class="card">
<h1>{{.Title}}</h1>
<div class="row"><label>ID подписки</label><span>{{.SubID}}</span></div>
<div class="row"><label>Email / имя</label><span>{{.Email}}</span></div>
<div class="row"><label>↑ Upload</label><span>{{.Upload}}</span></div>
<div class="row"><label>↓ Download</label><span>{{.Download}}</span></div>
<div class="row"><label>Использовано</label><span>{{.Used}}</span></div>
<div class="row"><label>Лимит</label><span>{{.Total}}</span></div>
<div class="row"><label>Срок</label><span>{{.Expire}}</span></div>
<div class="row"><label>URL подписки</label><code>{{.SubURL}}</code></div>
{{if .Links}}<div class="row"><label>Конфиг</label><code>{{index .Links 0}}</code></div>{{end}}
<a class="btn" href="{{.SubURL}}">Обновить подписку</a>
<div class="qr"><canvas id="qr"></canvas></div>
</div>
<script src="https://cdn.jsdelivr.net/npm/qrious@4.0.2/dist/qrious.min.js"></script>
<script>
try{new QRious({element:document.getElementById('qr'),value:{{printf "%q" .SubURL}},size:220});}catch(e){}
</script>
</body>
</html>`))
