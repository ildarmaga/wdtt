package panel

import (
	"strings"
	"sync"
)

const logBufferCapacity = 4096

var (
	logBufferMu   sync.RWMutex
	logBuffer     []string
	logBufferPos  int
	logBufferFull bool
)

func appendLogBuffer(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	logBufferMu.Lock()
	defer logBufferMu.Unlock()
	if len(logBuffer) < logBufferCapacity {
		logBuffer = append(logBuffer, line)
		return
	}
	logBufferFull = true
	logBuffer[logBufferPos] = line
	logBufferPos = (logBufferPos + 1) % logBufferCapacity
}

func snapshotLogBuffer() []string {
	logBufferMu.RLock()
	defer logBufferMu.RUnlock()
	if len(logBuffer) == 0 {
		return nil
	}
	if !logBufferFull {
		out := make([]string, len(logBuffer))
		copy(out, logBuffer)
		return out
	}
	out := make([]string, 0, logBufferCapacity)
	out = append(out, logBuffer[logBufferPos:]...)
	out = append(out, logBuffer[:logBufferPos]...)
	return out
}

// fetchUnifiedBufferLogLines returns formatted unified-process log lines, newest first.
func fetchUnifiedBufferLogLines(scanCount int, level, serviceKey string) []string {
	raw := snapshotLogBuffer()
	if len(raw) == 0 {
		return nil
	}
	if scanCount <= 0 {
		scanCount = 100
	}
	if scanCount > len(raw) {
		scanCount = len(raw)
	}
	level = normalizeLogLevelFilter(level)
	out := make([]string, 0, scanCount)
	for i := len(raw) - 1; i >= 0 && len(out) < scanCount; i-- {
		line := raw[i]
		body := logMessageBody(line)
		source := "WDTT"
		if isPanelLogMessage(body) {
			source = "WDTT Panel"
		}
		switch serviceKey {
		case "panel":
			if source != "WDTT Panel" {
				continue
			}
		case "wdtt":
			if source == "WDTT Panel" {
				continue
			}
		}
		formatted := formatJournalLine(line, 6, "", source)
		if formatted == "" {
			continue
		}
		if !passesLogLevelFilter(level, formattedLevel(formatted)) {
			continue
		}
		out = append(out, formatted)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
