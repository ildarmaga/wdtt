package panel

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	logTimePrefixRe = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2})\s+(\d{2}:\d{2}:\d{2})(?:\.\d+)?\s*`)
	xrayLevelTagRe  = regexp.MustCompile(`^\[(Info|Warning|Error|Debug)\]\s*`)
)

var logLevelRank = map[string]int{
	"ERROR": 3, "WARNING": 4, "NOTICE": 5, "INFO": 6, "DEBUG": 7,
}

func fetchFormattedServiceLogs(unit string, count int, level string, syslog bool, serviceKey string) []string {
	level = normalizeLogLevelFilter(level)
	args := []string{"--no-pager", "-n", strconv.Itoa(count), "-r", "-o", "cat", "-p", level}
	if !syslog && unit != "" {
		args = append([]string{"-u", unit}, args...)
	}
	out, _ := runCmd("journalctl", args...)
	source := logSourceForService(unit, serviceKey)
	lines := make([]string, 0, count)
	for _, raw := range strings.Split(out, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if serviceKey == "panel" && needsUnifiedLogFilter(serviceKey, unit) {
			source = "WDTT Panel"
		}
		formatted := formatJournalLine(raw, 6, "", source)
		if formatted == "" {
			continue
		}
		if passesLogLevelFilter(level, formattedLevel(formatted)) {
			lines = append(lines, formatted)
		}
	}
	return lines
}

func normalizeLogLevelFilter(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "err", "error":
		return "err"
	case "warning":
		return "warning"
	case "notice":
		return "notice"
	case "debug":
		return "debug"
	default:
		return "info"
	}
}

func logSourceForUnit(unit string) string {
	return logSourceForService(unit, "")
}

func logSourceForService(unit, serviceKey string) string {
	if serviceKey == "panel" {
		return "WDTT Panel"
	}
	switch unit {
	case panelServiceUnit:
		return "WDTT Panel"
	case wdttServiceUnit:
		return "WDTT"
	case xrayServiceUnit:
		return "XRAY"
	default:
		return "SYS"
	}
}

func normalizeLogSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "wdtt-app", "wdtt":
		return "WDTT"
	case "wdtt-xray", "xray":
		return "XRAY"
	case "wdtt-panel":
		return "WDTT Panel"
	default:
		if source == "" {
			return "SYS"
		}
		upper := strings.ToUpper(source)
		if upper == "WDTT-APP" {
			return "WDTT"
		}
		return upper
	}
}

func stripServiceLogPrefix(body string) string {
	for {
		trimmed := strings.TrimSpace(body)
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "wdtt-app:"):
			body = strings.TrimSpace(trimmed[len("wdtt-app:"):])
		case strings.HasPrefix(lower, "wdtt:"):
			body = strings.TrimSpace(trimmed[len("wdtt:"):])
		case strings.HasPrefix(trimmed, "WDTT-APP:"):
			body = strings.TrimSpace(trimmed[len("WDTT-APP:"):])
		case strings.HasPrefix(trimmed, "WDTT:"):
			body = strings.TrimSpace(trimmed[len("WDTT:"):])
		default:
			return trimmed
		}
	}
}

func formatJournalLine(message string, priority int, realtimeUS, source string) string {
	date, clock, body := splitLogTimestamp(message, realtimeUS)
	body = strings.TrimSpace(body)
	body = xrayLevelTagRe.ReplaceAllString(body, "")
	body = stripServiceLogPrefix(body)

	level := inferLogLevel(body, priority)
	if body == "" {
		body = strings.TrimSpace(xrayLevelTagRe.ReplaceAllString(message, ""))
	}
	if date == "" {
		return ""
	}
	return date + " " + clock + " " + level + " - " + source + ": " + body
}

func splitLogTimestamp(message, realtimeUS string) (date, clock, body string) {
	if m := logTimePrefixRe.FindStringSubmatch(message); len(m) > 2 {
		body = strings.TrimSpace(message[len(m[0]):])
		return m[1], m[2], body
	}
	if realtimeUS != "" {
		us, err := strconv.ParseInt(realtimeUS, 10, 64)
		if err == nil {
			t := unixMicroToLocal(us)
			return t.Format("2006/01/02"), t.Format("15:04:05"), strings.TrimSpace(message)
		}
	}
	return "", "", strings.TrimSpace(message)
}

func inferLogLevel(message string, priority int) string {
	if m := xrayLevelTagRe.FindStringSubmatch(message); len(m) > 1 {
		switch strings.ToLower(m[1]) {
		case "error":
			return "ERROR"
		case "warning":
			return "WARNING"
		case "debug":
			return "DEBUG"
		default:
			return "INFO"
		}
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(message, "http: TLS handshake error"),
		strings.Contains(lower, "warning"),
		strings.Contains(message, "[UFW BLOCK]"):
		return "WARNING"
	case strings.Contains(lower, "error"),
		strings.Contains(lower, "failed"),
		strings.Contains(lower, "fatal"):
		return "ERROR"
	default:
		return journalPriorityToLevel(priority)
	}
}

func journalPriorityToLevel(priority int) string {
	switch {
	case priority <= 3:
		return "ERROR"
	case priority == 4:
		return "WARNING"
	case priority == 5:
		return "NOTICE"
	case priority == 6:
		return "INFO"
	default:
		return "DEBUG"
	}
}

func formattedLevel(line string) string {
	parts := strings.SplitN(line, " - ", 2)
	if len(parts) == 0 {
		return "INFO"
	}
	fields := strings.Fields(parts[0])
	if len(fields) >= 3 {
		return strings.ToUpper(fields[2])
	}
	return "INFO"
}

func logFilterRank(filter string) int {
	switch normalizeLogLevelFilter(filter) {
	case "err":
		return 3
	case "warning":
		return 4
	case "notice":
		return 5
	case "debug":
		return 7
	default:
		return 6
	}
}

func passesLogLevelFilter(filter, entryLevel string) bool {
	entryRank, ok := logLevelRank[strings.ToUpper(entryLevel)]
	if !ok {
		entryRank = 6
	}
	return entryRank <= logFilterRank(filter)
}

func unixMicroToLocal(us int64) time.Time {
	return time.Unix(us/1_000_000, (us%1_000_000)*1000).Local()
}
