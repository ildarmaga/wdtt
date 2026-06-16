package panel

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type panelSplitWriter struct {
	stderr io.Writer
	panel  *os.File
}

func (w *panelSplitWriter) Write(p []byte) (int, error) {
	if w.panel != nil {
		msg := strings.TrimSpace(string(p))
		if msg != "" {
			body := logMessageBody(msg)
			if isPanelLogMessage(body) {
				_, _ = w.panel.Write(p)
			}
		}
	}
	return w.stderr.Write(p)
}

func logMessageBody(line string) string {
	if i := strings.Index(line, " "); i >= 0 {
		rest := strings.TrimSpace(line[i+1:])
		if len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' && strings.Contains(rest, ":") {
			if j := strings.Index(rest, " "); j >= 0 {
				return strings.TrimSpace(rest[j+1:])
			}
		}
		return rest
	}
	return line
}

// InitUnifiedLogSink дублирует panel-сообщения в /etc/wdtt/panel.log (unified).
func InitUnifiedLogSink() {
	if !isUnifiedDeployment() {
		return
	}
	path := filepath.Join(wdttConfigDir, "panel.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		log.Printf("panel log file: %v", err)
		return
	}
	log.SetOutput(&panelSplitWriter{stderr: os.Stderr, panel: f})
}

func panelLogPath() string {
	return filepath.Join(wdttConfigDir, "panel.log")
}

func fetchPanelLogLines(count int, level string) []string {
	if count <= 0 {
		count = 20
	}
	tailN := count * 3
	if tailN < 100 {
		tailN = 100
	}
	if tailN > 500 {
		tailN = 500
	}
	raw, err := readTailLines(panelLogPath(), tailN)
	if err != nil || len(raw) == 0 {
		return nil
	}
	level = normalizeLogLevelFilter(level)
	lines := make([]string, 0, count)
	for i := len(raw) - 1; i >= 0; i-- {
		line := strings.TrimSpace(raw[i])
		if line == "" {
			continue
		}
		formatted := formatPanelFileLogLine(line)
		if formatted == "" {
			continue
		}
		if !passesLogLevelFilter(level, formattedLevel(formatted)) {
			continue
		}
		lines = append(lines, formatted)
		if len(lines) >= count {
			break
		}
	}
	return lines
}

func formatPanelFileLogLine(line string) string {
	body := logMessageBody(line)
	if body == "" {
		body = line
	}
	date, clock, rest := splitLogTimestamp(body, "")
	if date == "" {
		if m := logTimePrefixRe.FindStringSubmatch(line); len(m) > 2 {
			date, clock = m[1], m[2]
			rest = strings.TrimSpace(line[len(m[0]):])
		}
	}
	if date == "" {
		return ""
	}
	if rest == "" {
		rest = body
	}
	rest = strings.TrimSpace(xrayLevelTagRe.ReplaceAllString(rest, ""))
	rest = stripServiceLogPrefix(rest)
	level := inferLogLevel(rest, 6)
	return date + " " + clock + " " + level + " - WDTT Panel: " + rest
}

func collapseRepeatedLogLines(lines []string) []string {
	if len(lines) <= 1 {
		return lines
	}
	out := make([]string, 0, len(lines))
	prevBody := logLineBody(lines[0])
	repeat := 1
	lastLine := lines[0]
	for i := 1; i < len(lines); i++ {
		body := logLineBody(lines[i])
		if body == prevBody {
			repeat++
			lastLine = lines[i]
			continue
		}
		out = append(out, formatRepeatLine(lastLine, repeat))
		prevBody = body
		repeat = 1
		lastLine = lines[i]
	}
	out = append(out, formatRepeatLine(lastLine, repeat))
	return out
}

func formatRepeatLine(line string, repeat int) string {
	if repeat <= 1 {
		return line
	}
	return line + " (×" + strconv.Itoa(repeat) + ")"
}
