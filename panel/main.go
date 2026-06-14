package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

type App struct {
	cfg *PanelConfig
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(formatPanelVersion())
		os.Exit(0)
	}

	if err := initI18n(); err != nil {
		log.Fatal("i18n: ", err)
	}
	if err := initTemplates(); err != nil {
		log.Fatal("templates: ", err)
	}
	if err := initPanelDB(); err != nil {
		log.Fatal("panel db: ", err)
	}

	cfg, err := loadPanelConfig()
	if err != nil {
		log.Fatal(err)
	}
	ensureLegacySettingsImported()
	ensureDefaultWdttData()
	if _, err := loadWdttInbound(); err != nil {
		log.Printf("panel inbound load: %v", err)
	}
	initAssetsVer()
	if acmeCronInstalled() {
		_ = ensureAcmeCron()
	}
	ensureWdttXrayDirs()
	patchWdttXrayService()
	if err := patchXrayStatsAPIOnDisk(); err != nil {
		log.Printf("xray stats API patch: %v", err)
	}
	startStatusCollector()

	app := &App{cfg: cfg}
	startSubscriptionServer(app)
	base := cfg.basePath()
	mux := http.NewServeMux()

	// Public
	mux.HandleFunc(base, app.serveLogin)
	mux.HandleFunc(strings.TrimSuffix(base, "/"), app.serveLogin)
	mux.HandleFunc(base+"login", app.handleLogin)
	mux.HandleFunc(base+"getTwoFactorEnable", app.handleTwoFactorEnable)
	mux.HandleFunc(base+"logout/", app.handleLogout)
	mux.HandleFunc(base+"logout", app.handleLogout)

	// 3x-ui assets (как в 3x-ui: долгий кэш + gzip на уровне сервера)
	assetsH := withCacheControl(assetsHandler(), "max-age=31536000, public")
	mux.Handle(base+"assets/", http.StripPrefix(base+"assets/", assetsH))

	// Panel pages
	mux.HandleFunc(base+"panel/", app.serveDashboard)
	mux.HandleFunc(base+"panel/users", app.serveUsersPage)
	mux.HandleFunc(base+"panel/connections", app.serveConnectionsPage)
	mux.HandleFunc(base+"panel/xray", app.serveXrayPage)
	mux.HandleFunc(base+"panel/settings", app.serveSettingsPage)

	// 3x-ui compatible xray API
	mux.HandleFunc(base+"panel/xray/", app.requireAuth(app.handleXrayAPI))
	mux.HandleFunc(base+"panel/setting/getDefaultJsonConfig", app.requireAuth(app.handleDefaultJsonConfig))
	mux.HandleFunc(base+"panel/api/custom-geo/aliases", app.requireAuth(app.handleCustomGeoAliases))
	mux.HandleFunc(base+"panel/api/server/", app.requireAuth(app.handleServerAPI))
	mux.HandleFunc(base+"panel/setting/defaultSettings", app.requireAuth(app.handleDefaultSettings))
	mux.HandleFunc(base+"panel/setting/all", app.requireAuth(app.handleSettingAll))
	mux.HandleFunc(base+"panel/setting/update", app.requireAuth(app.handleSettingUpdate))
	mux.HandleFunc(base+"panel/setting/ssh-port", app.requireAuth(app.handleSettingSSHPort))
	mux.HandleFunc(base+"panel/setting/updateUser", app.requireAuth(app.handleSettingUpdateUser))
	mux.HandleFunc(base+"panel/setting/restartPanel", app.requireAuth(app.handleSettingRestartPanel))
	mux.HandleFunc(base+"panel/api/custom-geo/list", app.requireAuth(app.handleCustomGeoList))

	// API (auth required)
	api := base + "panel/api/"
	mux.HandleFunc(api+"status", app.requireAuth(app.handleStatus))
	mux.HandleFunc(api+"inbound", app.requireAuth(app.handleInboundGet))
	mux.HandleFunc(api+"inbound/save", app.requireAuth(app.handleInboundSave))
	mux.HandleFunc(api+"firewall/ports", app.requireAuth(app.handleFirewallPortsList))
	mux.HandleFunc(api+"firewall/open", app.requireAuth(app.handleFirewallPortOpen))
	mux.HandleFunc(api+"firewall/update", app.requireAuth(app.handleFirewallPortUpdate))
	mux.HandleFunc(api+"firewall/close", app.requireAuth(app.handleFirewallPortClose))
	mux.HandleFunc(api+"firewall/ufw-enable", app.requireAuth(app.handleFirewallUFWEnable))
	mux.HandleFunc(api+"certs/list", app.requireAuth(app.handleCertsList))
	mux.HandleFunc(api+"certs/issue", app.requireAuth(app.handleCertsIssue))
	mux.HandleFunc(api+"certs/issue-ip", app.requireAuth(app.handleCertsIssueIP))
	mux.HandleFunc(api+"certs/renew", app.requireAuth(app.handleCertsRenew))
	mux.HandleFunc(api+"certs/acme-log", app.requireAuth(app.handleCertsAcmeLog))
	mux.HandleFunc(api+"certs/renew-dtls", app.requireAuth(app.handleCertsRenewDtls))
	mux.HandleFunc(api+"certs/revoke", app.requireAuth(app.handleCertsRevoke))
	mux.HandleFunc(api+"certs/delete-certbot", app.requireAuth(app.handleCertsDeleteCertbot))
	mux.HandleFunc(api+"certs/apply", app.requireAuth(app.handleCertsApply))
	mux.HandleFunc(api+"certs/install-acme", app.requireAuth(app.handleCertsInstallAcme))
	mux.HandleFunc(api+"certs/acme-cron", app.requireAuth(app.handleCertsAcmeCron))
	mux.HandleFunc(api+"xray-inbounds", app.requireAuth(app.handleXrayInboundsList))
	mux.HandleFunc(api+"xray-inbounds/save", app.requireAuth(app.handleXrayInboundSave))
	mux.HandleFunc(api+"xray-inbounds/delete", app.requireAuth(app.handleXrayInboundDelete))
	mux.HandleFunc(api+"users", app.requireAuth(app.handleUsersList))
	mux.HandleFunc(api+"users/add", app.requireAuth(app.handleUserAdd))
	mux.HandleFunc(api+"users/update", app.requireAuth(app.handleUserUpdate))
	mux.HandleFunc(api+"users/reset-traffic", app.requireAuth(app.handleUserResetTraffic))
	mux.HandleFunc(api+"users/delete", app.requireAuth(app.handleUserDelete))
	mux.HandleFunc(api+"password/main", app.requireAuth(app.handleMainPassword))
	mux.HandleFunc(api+"password/panel", app.requireAuth(app.handlePanelPassword))
	mux.HandleFunc(api+"service/", app.requireAuth(app.handleService))
	mux.HandleFunc(api+"xray/versions", app.requireAuth(app.handleXrayVersions))
	mux.HandleFunc(api+"xray/install/", app.requireAuth(app.handleXrayInstall))
	mux.HandleFunc(api+"xray/geofiles/", app.requireAuth(app.handleXrayGeofiles))
	mux.HandleFunc(api+"xray/config", app.requireAuth(app.handleXrayConfig))
	mux.HandleFunc(api+"logs", app.requireAuth(app.handleLogs))

	log.Fatal(startPanelServer(cfg, gzipMiddleware(mux)))
}

func (a *App) serveLogin(w http.ResponseWriter, r *http.Request) {
	if a.parseSession(r) != nil {
		http.Redirect(w, r, a.cfg.basePath()+"panel/", http.StatusFound)
		return
	}
	a.renderHTML(w, r, "login.html", "pages.login.title", nil)
}

func (a *App) serveDashboard(w http.ResponseWriter, r *http.Request) {
	if a.parseSession(r) == nil {
		http.Redirect(w, r, a.cfg.basePath(), http.StatusFound)
		return
	}
	if r.URL.Path != a.cfg.basePath()+"panel/" && r.URL.Path != strings.TrimSuffix(a.cfg.basePath(), "/")+"panel/" {
		http.NotFound(w, r)
		return
	}
	a.renderHTML(w, r, "index.html", "pages.index.title", nil)
}

func (a *App) serveUsersPage(w http.ResponseWriter, r *http.Request) {
	if a.parseSession(r) == nil {
		http.Redirect(w, r, a.cfg.basePath(), http.StatusFound)
		return
	}
	a.renderHTML(w, r, "users.html", "Пользователи", nil)
}

func (a *App) serveSettingsPage(w http.ResponseWriter, r *http.Request) {
	if a.parseSession(r) == nil {
		http.Redirect(w, r, a.cfg.basePath(), http.StatusFound)
		return
	}
	a.renderHTML(w, r, "settings.html", "pages.settings.title", nil)
}
