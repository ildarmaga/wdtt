package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
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
		total_bytes, max_down_mbps, max_up_mbps, is_deactivated, comment, ports, vk_hash, last_seen_at
		FROM wdtt_users WHERE sub_id = ?`, subID).Scan(
		&pass, &subCol, &e.DeviceID, &e.MaxDevices, &e.ExpiresAt, &e.DownBytes, &e.UpBytes,
		&e.TotalBytes, &e.MaxDownMBps, &e.MaxUpMBps, &deactivated, &e.Comment, &e.Ports, &e.VkHash, &e.LastSeenAt,
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
	cfg := app.cfg
	if cfg == nil || !cfg.SubEnable {
		return
	}
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/assets/", http.StripPrefix("/assets/", withCacheControl(assetsHandler(), "max-age=31536000, public")))
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
	link, err := buildWdttShareLink(linkHost, info.Password, info.Email, a.cfg.SubTitle, "", info.Entry.VkHash, info.Entry, inbound, a.buildSubURL(info.Entry.SubID))
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

	if subscriptionWantsHTML(r) {
		a.serveSubInfoPage(w, r, subID, info, links)
		return
	}

	a.applySubHeaders(w, header)

	body := strings.Join(links, "\n")
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

func (a *App) serveSubInfoPage(w http.ResponseWriter, r *http.Request, subID string, info *subUserInfo, links []string) {
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
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := htmlTemplates.ExecuteTemplate(w, "subscription.html", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
