package panel

import (
	"crypto/rand"
	"crypto/tls"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
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
	s, err := paneldb.LoadStore(panelDB)
	if err != nil {
		return nil, fmt.Errorf("subscription not found")
	}
	for pass, u := range s.Users {
		if u == nil || u.SubID != subID {
			continue
		}
		e := userEntryFromPaneldb(u)
		if e.IsDeactivated || isPasswordExpired(e) || trafficExceeded(e) {
			return nil, fmt.Errorf("subscription inactive")
		}
		email := strings.TrimSpace(e.Comment)
		if email == "" {
			email = pass
		}
		return &subUserInfo{Password: pass, Entry: e, Email: email}, nil
	}
	return nil, fmt.Errorf("subscription not found")
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

func subscriptionWantsHTML(r *http.Request) bool {
	q := r.URL.Query()
	if f := strings.ToLower(strings.TrimSpace(q.Get("format"))); f == "raw" || f == "sub" || f == "base64" {
		return false
	}
	if q.Get("html") == "1" || strings.EqualFold(q.Get("view"), "html") {
		return true
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "text/html") {
		return true
	}
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	if ua == "" {
		return false
	}
	if strings.Contains(ua, "okhttp") || strings.Contains(ua, "dart/") ||
		strings.Contains(ua, "clash") || strings.Contains(ua, "sing-box") ||
		strings.Contains(ua, "hiddify") || strings.Contains(ua, "v2ray") ||
		strings.Contains(ua, "streisand") || strings.Contains(ua, "shadowrocket") {
		return false
	}
	return strings.Contains(ua, "mozilla") || strings.Contains(ua, "safari") ||
		strings.Contains(ua, "chrome") || strings.Contains(ua, "edg/")
}

func startSubscriptionServer(app *App) {
	app.startSubscriptionServer()
}

func (a *App) stopSubscriptionServer() {
	a.subMu.Lock()
	defer a.subMu.Unlock()
	if a.subCancel != nil {
		a.subCancel()
		a.subCancel = nil
	}
	if a.subSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = a.subSrv.Shutdown(ctx)
		cancel()
		a.subSrv = nil
	}
}

func (a *App) restartSubscriptionServer() {
	a.stopSubscriptionServer()
	a.startSubscriptionServer()
}

func (a *App) startSubscriptionServer() {
	a.subMu.Lock()
	if a.cfg == nil || !a.cfg.SubEnable {
		a.subMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.subCancel = cancel
	cfg := a.cfg
	a.subMu.Unlock()

	go a.runSubscriptionServer(ctx, cfg)
}

func (a *App) runSubscriptionServer(ctx context.Context, cfg *PanelConfig) {
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", withCacheControl(assetsHandler(), "max-age=31536000, public")))
	path := normalizeSubPath(cfg.SubPath)
	mux.HandleFunc(path, a.handleSubscription)
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
			listener.Close()
			return
		}
		listener = network.NewMuxListener(listener, &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})
		scheme = "https"
	}

	srv := &http.Server{Handler: mux}
	a.subMu.Lock()
	a.subSrv = srv
	a.subMu.Unlock()

	log.Printf("WDTT Subscription: %s://%s%s", scheme, listener.Addr().String(), path)
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
		listener.Close()
	}()
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Printf("subscription server: %v", err)
	}
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
	subURL := a.buildSubURL(info.Entry.SubID)
	link, err := buildWdttShareLink(linkHost, info.Password, info.Email, a.cfg.SubTitle, "", info.Entry.VkHash, info.Entry, inbound, subURL)
	if err != nil {
		http.Error(w, "failed to build link", http.StatusInternalServerError)
		return
	}
	expireSec := int64(0)
	if info.Entry.ExpiresAt > 0 {
		expireSec = info.Entry.ExpiresAt
	}
	header := fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d",
		info.Entry.UpBytes, info.Entry.DownBytes, info.Entry.TotalBytes, expireSec)

	if subscriptionWantsHTML(r) {
		pageLinks, linkTitles, err := buildAllSubscriptionLinks(linkHost, info.Password, info.Email, a.cfg.SubTitle, info.Entry, inbound, subURL)
		if err != nil || len(pageLinks) == 0 {
			pageLinks = []string{link}
			linkTitles = []string{"WDTT JSON"}
		}
		a.serveSubInfoPage(w, r, subID, info, pageLinks, linkTitles)
		return
	}

	a.applySubHeaders(w, header)

	body := link
	if a.cfg.SubEncrypt {
		body = base64.StdEncoding.EncodeToString([]byte(body))
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(body))
}

func (a *App) applySubHeaders(w http.ResponseWriter, userInfoHeader string) {
	if a.cfg.SubShowInfo && userInfoHeader != "" {
		w.Header().Set("Subscription-Userinfo", userInfoHeader)
	}
	if title := strings.TrimSpace(a.cfg.SubTitle); title != "" {
		w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte(title)))
	}
	support := strings.TrimSpace(a.cfg.SubSupportURL)
	if support != "" {
		w.Header().Set("Profile-Web-Page-Url", support)
	}
	if profile := strings.TrimSpace(a.cfg.SubProfileURL); profile != "" && profile != support {
		w.Header().Set("Profile-Home-Page-Url", profile)
	}
	if announce := strings.TrimSpace(a.cfg.SubAnnounce); announce != "" {
		w.Header().Set("Announce", "base64:"+base64.StdEncoding.EncodeToString([]byte(announce)))
	}
	w.Header().Set("Content-Disposition", "attachment; filename=wdtt")
}

func (a *App) serveSubInfoPage(w http.ResponseWriter, r *http.Request, subID string, info *subUserInfo, links, linkTitles []string) {
	if htmlTemplates == nil {
		http.Error(w, "templates not loaded", http.StatusInternalServerError)
		return
	}
	used := trafficUsed(info.Entry)
	total := info.Entry.TotalBytes
	remained := ""
	totalStr := "∞"
	if total > 0 {
		totalStr = formatBytes(total)
		left := total - used
		if left < 0 {
			left = 0
		}
		remained = formatBytes(left)
	}
	expireSec := int64(0)
	if info.Entry.ExpiresAt > 0 {
		expireSec = info.Entry.ExpiresAt
	}
	lastOnline := info.Entry.LastSeenAt
	if stats := loadServerStats(); stats != nil && stats.Timestamp > 0 {
		db, err := loadPasswords()
		isMain := err == nil && db != nil && info.Password == db.MainPassword
		if userOnlineFromStats(info.Password, deviceIDsDisplay(info.Entry), isMain, stats) && stats.Timestamp > lastOnline {
			lastOnline = stats.Timestamp
		}
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	data := pageData{
		"title":        "subscription.title",
		"host":         host,
		"base_path":    "/",
		"cur_ver":      panelVersion,
		"assets_ver":   assetsVer,
		"sId":          subID,
		"subUrl":       a.buildSubURL(subID),
		"subJsonUrl":   "",
		"subClashUrl":  "",
		"download":     formatBytes(info.Entry.DownBytes),
		"upload":       formatBytes(info.Entry.UpBytes),
		"used":         formatBytes(used),
		"total":        totalStr,
		"remained":     remained,
		"expire":       expireSec,
		"lastOnline":   lastOnline,
		"downloadByte": info.Entry.DownBytes,
		"uploadByte":   info.Entry.UpBytes,
		"totalByte":    total,
		"datepicker":   "gregorian",
		"result":       links,
		"linkTitles":   linkTitles,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := htmlTemplates.ExecuteTemplate(w, "subscription.html", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
