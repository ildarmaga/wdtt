package panel

import (
	"fmt"
	"strings"
)

func xrayVersionTagList() ([]string, error) {
	releases, err := fetchXrayReleases()
	if err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(releases))
	for _, rel := range releases {
		tags = append(tags, rel.TagName)
	}
	return tags, nil
}

func controlService(name, action string) error {
	svcMap := map[string]string{"wdtt": wdttServiceUnit, "xray": xrayServiceUnit}
	svc, ok := svcMap[name]
	if !ok {
		return fmt.Errorf("неизвестный сервис")
	}
	switch action {
	case "restart":
		if svc == wdttServiceUnit {
			return restartWdttWithDeps()
		}
		if svc == xrayServiceUnit {
			markXrayAutoManaged()
		}
		return serviceRestart(svc)
	case "stop":
		if svc == xrayServiceUnit {
			markXrayManuallyStopped()
		}
		return serviceStop(svc)
	case "start":
		if svc == xrayServiceUnit {
			markXrayAutoManaged()
		}
		return serviceStart(svc)
	default:
		return fmt.Errorf("неизвестное действие")
	}
}

func updateGeofilesOp(name string) (updated []string, restartXray bool, err error) {
	if name == "" || name == "all" || name == "/" {
		updated, err = updateAllGeofiles()
		return updated, false, err
	}
	name = strings.Trim(name, "/")
	if err = updateGeofile(name); err != nil {
		return nil, false, err
	}
	return []string{name}, true, nil
}

func fetchServiceLogLines(count int, serviceKey, level string, syslog bool) []string {
	unit := resolveLogUnit(serviceKey, syslog)
	fetchCount := count
	if needsUnifiedLogFilter(serviceKey, unit) {
		fetchCount = count * 20
		if fetchCount < 200 {
			fetchCount = 200
		}
	}
	lines := fetchFormattedServiceLogs(unit, fetchCount, level, syslog, serviceKey)
	if needsUnifiedLogFilter(serviceKey, unit) {
		lines = filterUnifiedLogLines(lines, serviceKey, count)
	} else if len(lines) > count {
		lines = lines[:count]
	}
	if lines == nil {
		return []string{}
	}
	return lines
}

func needsUnifiedLogFilter(serviceKey, unit string) bool {
	if serviceKey != "panel" && serviceKey != "wdtt" {
		return false
	}
	return unit == wdttServiceUnit && !serviceUnitExists(panelServiceUnit)
}

func filterUnifiedLogLines(lines []string, serviceKey string, limit int) []string {
	filtered := make([]string, 0, limit)
	for _, line := range lines {
		body := logLineBody(line)
		switch serviceKey {
		case "panel":
			if !isPanelLogMessage(body) {
				continue
			}
		case "wdtt":
			if isPanelLogMessage(body) {
				continue
			}
		}
		filtered = append(filtered, line)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

func logLineBody(line string) string {
	_, body, ok := strings.Cut(line, " - ")
	if !ok {
		return line
	}
	_, msg, ok := strings.Cut(body, ": ")
	if ok {
		return msg
	}
	return body
}

func isPanelLogMessage(body string) bool {
	b := strings.TrimSpace(body)
	if b == "" {
		return false
	}
	lower := strings.ToLower(b)
	panelPrefixes := []string{
		"[panel]", "[watchdog]",
		"panel db:", "panel inbound",
		"subscription listen", "subscription server:",
		"subscription tls:", "xray hot-", "xray restart", "xray stats api",
		"wdtt-xray.service обновлён",
		"логин:", "error loading certificates",
		"warning: ssl", "export db:", "restart panel:",
	}
	for _, p := range panelPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	for _, p := range []string{"[PANEL]", "WDTT Panel:", "WDTT Subscription:"} {
		if strings.HasPrefix(b, p) {
			return true
		}
	}
	return false
}

func resolveLogUnit(serviceKey string, syslog bool) string {
	if syslog {
		return ""
	}
	switch serviceKey {
	case "wdtt":
		return wdttServiceUnit
	case "xray":
		return xrayServiceUnit
	case "panel", "":
		if serviceUnitExists(panelServiceUnit) {
			return panelServiceUnit
		}
		return wdttServiceUnit
	default:
		return wdttServiceUnit
	}
}

func serviceUnitExists(unit string) bool {
	_, err := runCmd("systemctl", "status", unit, "--no-pager")
	return err == nil
}
