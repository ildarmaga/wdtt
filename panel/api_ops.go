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
	unit := panelServiceUnit
	if !syslog {
		switch serviceKey {
		case "wdtt":
			unit = wdttServiceUnit
		case "xray":
			unit = xrayServiceUnit
		case "panel", "":
			unit = panelServiceUnit
		}
	} else {
		unit = ""
	}
	lines := fetchFormattedServiceLogs(unit, count, level, syslog)
	if lines == nil {
		return []string{}
	}
	return lines
}
