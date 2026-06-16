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

func TestSkipJournalNoise(t *testing.T) {
	if !skipJournalNoise("2026/06/08 15:00:00 [СТАТ] traffic", "wdtt") {
		t.Fatal("expected [СТАТ] to be skipped for wdtt")
	}
	if skipJournalNoise("2026/06/08 15:00:00 normal line", "wdtt") {
		t.Fatal("normal line should not be skipped")
	}
	if skipJournalNoise("2026/06/08 15:00:00 [СТАТ] traffic", "xray") {
		t.Fatal("[СТАТ] should only be skipped for wdtt service")
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
