package panel

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

type App struct {
	cfg       *PanelConfig
	subMu     sync.Mutex
	subCancel context.CancelFunc
	subSrv    *http.Server
}

// Run запускает HTTP-панель (блокирует до ошибки).
func Run() error {
	if err := initI18n(); err != nil {
		return fmt.Errorf("i18n: %w", err)
	}
	if err := initTemplates(); err != nil {
		return fmt.Errorf("templates: %w", err)
	}
	if err := initPanelDB(); err != nil {
		return fmt.Errorf("panel db: %w", err)
	}

	cfg, err := loadPanelConfig()
	if err != nil {
		return err
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
	app.startSubscriptionServer()
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
	mux.HandleFunc(base+"panel/xray/", app.requireAuthCSRF(app.handleXrayAPI))
	mux.HandleFunc(base+"panel/setting/getDefaultJsonConfig", app.requireAuthCSRF(app.handleDefaultJsonConfig))
	mux.HandleFunc(base+"panel/api/custom-geo/aliases", app.requireAuthCSRF(app.handleCustomGeoAliases))
	mux.HandleFunc(base+"panel/api/server/", app.requireAuthCSRF(app.handleServerAPI))
	mux.HandleFunc(base+"panel/setting/defaultSettings", app.requireAuthCSRF(app.handleDefaultSettings))
	mux.HandleFunc(base+"panel/setting/all", app.requireAuthCSRF(app.handleSettingAll))
	mux.HandleFunc(base+"panel/setting/update", app.requireAuthCSRF(app.handleSettingUpdate))
	mux.HandleFunc(base+"panel/setting/ssh-port", app.requireAuthCSRF(app.handleSettingSSHPort))
	mux.HandleFunc(base+"panel/setting/updateUser", app.requireAuthCSRF(app.handleSettingUpdateUser))
	mux.HandleFunc(base+"panel/setting/restartPanel", app.requireAuthCSRF(app.handleSettingRestartPanel))
	mux.HandleFunc(base+"panel/api/custom-geo/list", app.requireAuthCSRF(app.handleCustomGeoList))

	// API (auth required)
	api := base + "panel/api/"
	mux.HandleFunc(api+"status", app.requireAuthCSRF(app.handleStatus))
	mux.HandleFunc(api+"inbound", app.requireAuthCSRF(app.handleInboundGet))
	mux.HandleFunc(api+"inbound/save", app.requireAuthCSRF(app.handleInboundSave))
	mux.HandleFunc(api+"firewall/ports", app.requireAuthCSRF(app.handleFirewallPortsList))
	mux.HandleFunc(api+"firewall/open", app.requireAuthCSRF(app.handleFirewallPortOpen))
	mux.HandleFunc(api+"firewall/update", app.requireAuthCSRF(app.handleFirewallPortUpdate))
	mux.HandleFunc(api+"firewall/close", app.requireAuthCSRF(app.handleFirewallPortClose))
	mux.HandleFunc(api+"firewall/ufw-enable", app.requireAuthCSRF(app.handleFirewallUFWEnable))
	mux.HandleFunc(api+"certs/list", app.requireAuthCSRF(app.handleCertsList))
	mux.HandleFunc(api+"certs/issue", app.requireAuthCSRF(app.handleCertsIssue))
	mux.HandleFunc(api+"certs/issue-ip", app.requireAuthCSRF(app.handleCertsIssueIP))
	mux.HandleFunc(api+"certs/renew", app.requireAuthCSRF(app.handleCertsRenew))
	mux.HandleFunc(api+"certs/acme-log", app.requireAuthCSRF(app.handleCertsAcmeLog))
	mux.HandleFunc(api+"certs/renew-dtls", app.requireAuthCSRF(app.handleCertsRenewDtls))
	mux.HandleFunc(api+"certs/revoke", app.requireAuthCSRF(app.handleCertsRevoke))
	mux.HandleFunc(api+"certs/delete-certbot", app.requireAuthCSRF(app.handleCertsDeleteCertbot))
	mux.HandleFunc(api+"certs/apply", app.requireAuthCSRF(app.handleCertsApply))
	mux.HandleFunc(api+"certs/install-acme", app.requireAuthCSRF(app.handleCertsInstallAcme))
	mux.HandleFunc(api+"certs/acme-cron", app.requireAuthCSRF(app.handleCertsAcmeCron))
	mux.HandleFunc(api+"xray-inbounds", app.requireAuthCSRF(app.handleXrayInboundsList))
	mux.HandleFunc(api+"xray-inbounds/save", app.requireAuthCSRF(app.handleXrayInboundSave))
	mux.HandleFunc(api+"xray-inbounds/delete", app.requireAuthCSRF(app.handleXrayInboundDelete))
	mux.HandleFunc(api+"users", app.requireAuthCSRF(app.handleUsersList))
	mux.HandleFunc(api+"users/add", app.requireAuthCSRF(app.handleUserAdd))
	mux.HandleFunc(api+"users/update", app.requireAuthCSRF(app.handleUserUpdate))
	mux.HandleFunc(api+"users/reset-traffic", app.requireAuthCSRF(app.handleUserResetTraffic))
	mux.HandleFunc(api+"users/delete", app.requireAuthCSRF(app.handleUserDelete))
	mux.HandleFunc(api+"password/main", app.requireAuthCSRF(app.handleMainPassword))
	mux.HandleFunc(api+"password/panel", app.requireAuthCSRF(app.handlePanelPassword))
	mux.HandleFunc(api+"service/", app.requireAuthCSRF(app.handleService))
	mux.HandleFunc(api+"xray/versions", app.requireAuthCSRF(app.handleXrayVersions))
	mux.HandleFunc(api+"xray/install/", app.requireAuthCSRF(app.handleXrayInstall))
	mux.HandleFunc(api+"xray/geofiles/", app.requireAuthCSRF(app.handleXrayGeofiles))
	mux.HandleFunc(api+"xray/config", app.requireAuthCSRF(app.handleXrayConfig))
	mux.HandleFunc(api+"logs", app.requireAuthCSRF(app.handleLogs))

	return startPanelServer(cfg, gzipMiddleware(mux))
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
