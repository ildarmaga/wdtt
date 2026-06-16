package panel

import (
	"strings"
	"testing"
)

func TestFormatJournalLine(t *testing.T) {
	line := formatJournalLine(
		"2026/06/08 15:11:46 http: TLS handshake error from 127.0.0.1: EOF",
		6, "", "WDTT Panel",
	)
	if line != "2026/06/08 15:11:46 WARNING - WDTT Panel: http: TLS handshake error from 127.0.0.1: EOF" {
		t.Fatalf("unexpected: %q", line)
	}

	line = formatJournalLine(
		"2026/06/08 14:07:39.618590 [Info] infra/conf/serial: Reading config",
		6, "", "XRAY",
	)
	if line != "2026/06/08 14:07:39 INFO - XRAY: infra/conf/serial: Reading config" {
		t.Fatalf("unexpected: %q", line)
	}
	line = formatJournalLine(
		"Started wdtt-xray.service - WDTT Xray routing (wdtt0 -> xray).",
		6, "", "XRAY",
	)
	if !strings.Contains(line, " - XRAY: Started wdtt-xray.service") {
		t.Fatalf("unexpected undated line: %q", line)
	}
}

func TestFormatWdttStatsJournalLine(t *testing.T) {
	line := formatWdttStatsJournalLine(&ServerStats{
		ActiveUsers: 1,
		Sessions:    10,
		Total:       42,
		NAT:         "MASQUERADE iptables ✅",
		UpGB:        "0.01",
		DownGB:      "0.03",
	})
	if !strings.Contains(line, "[СТАТ] Пользователей: 1 | Сессий: 10") {
		t.Fatalf("unexpected: %q", line)
	}
}

func TestPrependWdttStatsSummary(t *testing.T) {
	lines := prependWdttStatsSummary([]string{"2026/06/16 10:00:00 INFO - WDTT: [WG] ok"})
	if len(lines) != 2 || !strings.Contains(lines[0], "[СТАТ]") {
		t.Fatalf("expected stats prepended, got %v", lines)
	}
	existing := []string{"2026/06/16 10:00:00 INFO - WDTT: [СТАТ] already"}
	if got := prependWdttStatsSummary(existing); len(got) != 1 {
		t.Fatalf("expected no duplicate stat line, got %v", got)
	}
}

func TestPassesLogLevelFilter(t *testing.T) {
	if !passesLogLevelFilter("info", "WARNING") {
		t.Fatal("warning should pass info filter")
	}
	if passesLogLevelFilter("warning", "INFO") {
		t.Fatal("info should not pass warning filter")
	}
}
