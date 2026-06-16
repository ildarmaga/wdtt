//go:build integration

package panel

import (
	"strings"
	"testing"
)

func TestFetchFormattedServiceLogsWDTT(t *testing.T) {
	lines := fetchFormattedServiceLogs(wdttServiceUnit, 30, "info", false, "wdtt")
	if len(lines) == 0 {
		t.Fatal("expected wdtt service logs, got none")
	}
	for _, line := range lines {
		if strings.Contains(line, "[СТАТ]") {
			t.Fatalf("unexpected [СТАТ] in output: %q", line)
		}
	}
}

func TestFetchFormattedServiceLogsXray(t *testing.T) {
	lines := fetchFormattedServiceLogs(xrayServiceUnit, 30, "info", false, "xray")
	if len(lines) == 0 {
		t.Fatal("expected xray service logs, got none")
	}
}

func TestGetXrayLogsNonEmpty(t *testing.T) {
	entries := getXrayLogs(20, "", true, true, true)
	if len(entries) == 0 {
		t.Fatal("expected xray access log entries, got none")
	}
	for _, e := range entries {
		if e.Inbound == "api" && e.Outbound == "api" {
			t.Fatalf("internal api traffic must be filtered: %+v", e)
		}
	}
}
