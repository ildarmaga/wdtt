package main

import (
	"net/http"
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

func (a *App) handleGetXrayVersion(w http.ResponseWriter, r *http.Request) {
	releases, err := fetchXrayReleases()
	if err != nil {
		jsonError(w, err.Error(), http.StatusOK)
		return
	}
	versions := make([]string, 0, len(releases))
	for _, rel := range releases {
		versions = append(versions, rel.TagName)
	}
	jsonOK(w, versions)
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
	markXrayManuallyStopped()
	if err := serviceStop(xrayServiceUnit); err != nil {
		jsonMsg(w, i18nWeb("pages.xray.stopError")+": "+err.Error(), false)
		return
	}
	jsonMsg(w, i18nWeb("pages.xray.stopSuccess"), true)
}

func (a *App) handleStopWdttService(w http.ResponseWriter, r *http.Request) {
	if err := serviceStop(wdttServiceUnit); err != nil {
		jsonMsg(w, "WDTT stop error: "+err.Error(), false)
		return
	}
	jsonMsg(w, "WDTT stopped", true)
}

func (a *App) handleRestartWdttService(w http.ResponseWriter, r *http.Request) {
	if err := restartWdttWithDeps(); err != nil {
		jsonMsg(w, "WDTT restart error: "+err.Error(), false)
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
	if name == "" || name == "/" {
		updated, err := updateAllGeofiles()
		if err != nil {
			jsonMsg(w, err.Error(), false)
			return
		}
		jsonMsg(w, "Updated: "+strings.Join(updated, ", "), true)
		return
	}
	name = strings.Trim(name, "/")
	if err := updateGeofile(name); err != nil {
		jsonMsg(w, err.Error(), false)
		return
	}
	serviceRestart(xrayServiceUnit)
	jsonMsg(w, name+" updated", true)
}

func (a *App) handleServerLogs(w http.ResponseWriter, r *http.Request) {
	countStr := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/server/logs/")
	count, _ := strconv.Atoi(countStr)
	if count <= 0 {
		count = 20
	}
	form, _ := parsePostForm(r)
	unit := panelServiceUnit
	switch form.Get("service") {
	case "wdtt":
		unit = wdttServiceUnit
	case "xray":
		unit = xrayServiceUnit
	case "panel":
		unit = panelServiceUnit
	}
	syslog := form.Get("syslog") == "true"
	level := form.Get("level")
	if syslog {
		unit = ""
	}
	lines := fetchFormattedServiceLogs(unit, count, level, syslog)
	if lines == nil {
		lines = []string{}
	}
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
	jsonMsg(w, "not supported", false)
}

func (a *App) handleDefaultSettings(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]interface{}{"ipLimitEnable": false})
}

func (a *App) handleCustomGeoList(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, []interface{}{})
}

func (a *App) handleGetDb(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not supported", http.StatusNotFound)
}

func (a *App) handleServerAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, a.cfg.basePath()+"panel/api/server/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	switch parts[0] {
	case "status":
		a.handleServerStatus(w, r)
	case "cpuHistory":
		a.handleCPUHistory(w, r)
	case "getXrayVersion":
		a.handleGetXrayVersion(w, r)
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
