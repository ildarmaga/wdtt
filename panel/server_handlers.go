package panel

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func (a *App) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, getCachedServerStatus())
}

func (a *App) handleCPUHistory(w http.ResponseWriter, r *http.Request) {
	bucketStr := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/server/cpuHistory/")
	bucket, _ := strconv.Atoi(bucketStr)
	if bucket <= 0 {
		bucket = 2
	}
	jsonOK(w, aggregateCPUHistory(bucket, 120))
}

func (a *App) handleProcessList(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, getProcessList(40))
}

func (a *App) handleKillProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pidStr := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/server/killProcess/")
	pid, err := strconv.Atoi(strings.Trim(pidStr, "/"))
	if err != nil || pid <= 0 {
		jsonMsg(w, "invalid pid", false)
		return
	}
	if err := killProcess(pid); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonMsg(w, "process terminated", true)
}

func (a *App) handleGetXrayVersion(w http.ResponseWriter, r *http.Request) {
	versions, err := xrayVersionTagList()
	if err != nil {
		jsonError(w, err.Error(), http.StatusOK)
		return
	}
	jsonOK(w, versions)
}

func (a *App) handleGetPanelVersion(w http.ResponseWriter, r *http.Request) {
	tags, err := fetchWdttPanelReleases()
	if err != nil {
		jsonError(w, err.Error(), http.StatusOK)
		return
	}
	jsonOK(w, tags)
}

func (a *App) handleInstallPanel(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/server/installPanel/")
	tag = strings.Trim(tag, "/")
	if tag == "" {
		jsonError(w, "version required", http.StatusOK)
		return
	}
	if err := installPanelVersion(tag); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonMsg(w, "Panel "+tag+" installed", true)
}

func (a *App) handleGetConfigJSON(w http.ResponseWriter, r *http.Request) {
	cfg, err := getXrayConfigJSON()
	if err != nil {
		jsonError(w, err.Error(), http.StatusOK)
		return
	}
	jsonOK(w, cfg)
}

func (a *App) handleStopXrayService(w http.ResponseWriter, r *http.Request) {
	if err := controlService("xray", "stop"); err != nil {
		jsonMsg(w, i18nWeb("pages.xray.stopError")+": "+err.Error(), false)
		return
	}
	jsonMsg(w, i18nWeb("pages.xray.stopSuccess"), true)
}

func (a *App) handleStopWdttService(w http.ResponseWriter, r *http.Request) {
	if isUnifiedDeployment() {
		jsonMsg(w, "используйте выключение VPN в Подключениях", false)
		return
	}
	if err := controlService("wdtt", "stop"); err != nil {
		jsonMsg(w, "WDTT stop error: "+err.Error(), false)
		return
	}
	jsonMsg(w, "WDTT stopped", true)
}

func (a *App) handleRestartWdttService(w http.ResponseWriter, r *http.Request) {
	if err := controlService("wdtt", "restart"); err != nil {
		jsonMsg(w, "WDTT restart error: "+err.Error(), false)
		return
	}
	if isUnifiedDeployment() {
		jsonMsg(w, "VPN-сервер перезапущен", true)
		return
	}
	jsonMsg(w, "WDTT restarted", true)
}

func (a *App) handleInstallXray(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/server/installXray/")
	if tag == "" {
		jsonError(w, "version required", http.StatusOK)
		return
	}
	if err := installXrayVersion(tag); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	jsonMsg(w, "Xray "+tag+" installed", true)
}

func (a *App) handleUpdateGeofile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/server/updateGeofile/")
	updated, restartXray, err := updateGeofilesOp(name)
	if err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	if restartXray {
		serviceRestart(xrayServiceUnit)
	}
	if len(updated) == 1 {
		jsonMsg(w, updated[0]+" updated", true)
		return
	}
	jsonMsg(w, "Updated: "+strings.Join(updated, ", "), true)
}

func (a *App) handleServerLogs(w http.ResponseWriter, r *http.Request) {
	countStr := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/server/logs/")
	count, _ := strconv.Atoi(countStr)
	if count <= 0 {
		count = 20
	}
	var req struct {
		Level   string      `json:"level"`
		Service string      `json:"service"`
		Syslog  interface{} `json:"syslog"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonMsg(w, "invalid request", false)
		return
	}
	syslog := false
	switch v := req.Syslog.(type) {
	case bool:
		syslog = v
	case string:
		syslog = v == "true" || v == "1"
	}
	lines := fetchServiceLogLines(count, req.Service, req.Level, syslog)
	jsonOK(w, lines)
}

func (a *App) handleXrayLogs(w http.ResponseWriter, r *http.Request) {
	count := parseXrayLogsCount(r, a.cfg.basePath())
	var req struct {
		Filter      string      `json:"filter"`
		ShowDirect  interface{} `json:"showDirect"`
		ShowBlocked interface{} `json:"showBlocked"`
		ShowProxy   interface{} `json:"showProxy"`
	}
	if err := readJSON(r, &req); err != nil {
		form, ferr := parsePostForm(r)
		if ferr == nil {
			req.Filter = form.Get("filter")
			if form.Get("showDirect") != "" {
				req.ShowDirect = form.Get("showDirect")
			}
			if form.Get("showBlocked") != "" {
				req.ShowBlocked = form.Get("showBlocked")
			}
			if form.Get("showProxy") != "" {
				req.ShowProxy = form.Get("showProxy")
			}
		}
	}
	entries := getXrayLogs(count, req.Filter, req.ShowDirect, req.ShowBlocked, req.ShowProxy)
	if entries == nil {
		entries = []XrayLogEntry{}
	}
	jsonOK(w, entries)
}

func (a *App) handleImportDB(w http.ResponseWriter, r *http.Request) {
	if !panelDBEnabled() {
		jsonMsg(w, "SQLite не инициализирован", false)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		jsonMsg(w, "файл не передан", false)
		return
	}
	defer file.Close()
	if err := importPanelDBFromReader(file); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	if cfg, err := loadPanelConfig(); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	} else {
		a.cfg = cfg
	}
	jsonOK(w, map[string]interface{}{
		"message": "База импортирована. Перезапустите панель для полного применения.",
	})
}

func (a *App) handleDefaultSettings(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]interface{}{"ipLimitEnable": false})
}

func (a *App) handleCustomGeoList(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, []interface{}{})
}

func (a *App) handleGetDb(w http.ResponseWriter, r *http.Request) {
	if !panelDBEnabled() {
		http.Error(w, "database not available", http.StatusNotFound)
		return
	}
	if _, err := os.Stat(panelDBPath); err != nil {
		http.Error(w, "database not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="panel.db"`)
	if err := exportPanelDB(w); err != nil {
		log.Printf("export db: %v", err)
	}
}

func (a *App) handleServerAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/server/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	switch parts[0] {
	case "status":
		a.handleServerStatus(w, r)
	case "events":
		a.handleServerEvents(w, r)
	case "metrics":
		a.handleServerMetrics(w, r)
	case "cpuHistory":
		a.handleCPUHistory(w, r)
	case "processes":
		a.handleProcessList(w, r)
	case "killProcess":
		if len(parts) > 1 {
			r.URL.Path = a.cfg.basePath() + "panel/api/server/killProcess/" + parts[1]
		}
		a.handleKillProcess(w, r)
	case "getXrayVersion":
		a.handleGetXrayVersion(w, r)
	case "getPanelVersion":
		a.handleGetPanelVersion(w, r)
	case "getConfigJson":
		a.handleGetConfigJSON(w, r)
	case "getDb":
		a.handleGetDb(w, r)
	case "stopXrayService":
		a.handleStopXrayService(w, r)
	case "restartXrayService":
		a.handleRestartXrayService(w, r)
	case "stopWdttService":
		a.handleStopWdttService(w, r)
	case "restartWdttService":
		a.handleRestartWdttService(w, r)
	case "installXray":
		if len(parts) > 1 {
			r.URL.Path = a.cfg.basePath() + "panel/api/server/installXray/" + parts[1]
		}
		a.handleInstallXray(w, r)
	case "installPanel":
		if len(parts) > 1 {
			r.URL.Path = a.cfg.basePath() + "panel/api/server/installPanel/" + parts[1]
		}
		a.handleInstallPanel(w, r)
	case "updateGeofile":
		r.URL.Path = a.cfg.basePath() + "panel/api/server/updateGeofile"
		if len(parts) > 1 {
			r.URL.Path += "/" + parts[1]
		}
		a.handleUpdateGeofile(w, r)
	case "logs":
		if len(parts) > 1 {
			r.URL.Path = a.cfg.basePath() + "panel/api/server/logs/" + parts[1]
		}
		a.handleServerLogs(w, r)
	case "xraylogs":
		if len(parts) > 1 {
			r.URL.Path = a.cfg.basePath() + "panel/api/server/xraylogs/" + parts[1]
		}
		a.handleXrayLogs(w, r)
	case "importDB":
		a.handleImportDB(w, r)
	default:
		http.NotFound(w, r)
	}
}
