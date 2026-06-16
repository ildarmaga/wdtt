package panel

import (
	"strings"
	"testing"
)

func TestCollapseRepeatedLogLines(t *testing.T) {
	lines := []string{
		"2026/06/16 07:00:00 ERROR - WDTT: [DTLS] same",
		"2026/06/16 07:00:01 ERROR - WDTT: [DTLS] same",
		"2026/06/16 07:00:02 ERROR - WDTT: [DTLS] same",
		"2026/06/16 07:00:03 INFO - WDTT: [СТАТ] ok",
	}
	out := collapseRepeatedLogLines(lines)
	if len(out) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(out), out)
	}
	if !strings.Contains(out[0], "(×3)") {
		t.Fatalf("expected repeat marker: %q", out[0])
	}
}

func TestFormatPanelFileLogLine(t *testing.T) {
	line := "2026/06/16 09:00:00 WDTT Panel: subscription listen :2096/sub"
	got := formatPanelFileLogLine(line)
	if got == "" || !strings.Contains(got, "WDTT Panel:") {
		t.Fatalf("unexpected: %q", got)
	}
}
