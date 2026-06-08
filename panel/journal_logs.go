package main

import (
	"encoding/json"
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

func fetchFormattedServiceLogs(unit string, count int, level string, syslog bool) []string {
	level = normalizeLogLevelFilter(level)
	args := []string{"--no-pager", "-n", strconv.Itoa(count), "-r", "-o", "json", "-p", level}
	if !syslog && unit != "" {
		args = append([]string{"-u", unit}, args...)
	}
	out, _ := runCmd("journalctl", args...)
	source := logSourceForUnit(unit)
	lines := make([]string, 0, count)
	for _, raw := range strings.Split(out, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || !strings.HasPrefix(raw, "{") {
			continue
		}
		var entry struct {
			Message              string `json:"MESSAGE"`
			Priority             string `json:"PRIORITY"`
			RealtimeTimestampUS  string `json:"__REALTIME_TIMESTAMP"`
			SyslogIdentifier     string `json:"SYSLOG_IDENTIFIER"`
		}
		if err := json.Unmarshal([]byte(raw), &entry); err != nil || entry.Message == "" {
			continue
		}
		priority, _ := strconv.Atoi(entry.Priority)
		if syslog && entry.SyslogIdentifier != "" {
			source = strings.ToUpper(entry.SyslogIdentifier)
		}
		formatted := formatJournalLine(entry.Message, priority, entry.RealtimeTimestampUS, source)
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

func formatJournalLine(message string, priority int, realtimeUS, source string) string {
	date, clock, body := splitLogTimestamp(message, realtimeUS)
	body = strings.TrimSpace(body)
	body = xrayLevelTagRe.ReplaceAllString(body, "")

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
