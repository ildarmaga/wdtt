package panel

import (
	"strings"
	"testing"
)

func TestFetchUnifiedBufferLogLines(t *testing.T) {
	appendLogBuffer("2026/06/16 10:00:01 [SERVER] Готов")
	appendLogBuffer("2026/06/16 10:00:02 [watchdog] WDTT активен")
	appendLogBuffer("2026/06/16 10:00:03 WDTT Subscription: https://example/subs/")

	wdtt := fetchUnifiedBufferLogLines(50, "info", "wdtt")
	if len(wdtt) == 0 {
		t.Fatal("expected wdtt buffer lines")
	}
	for _, line := range wdtt {
		if strings.Contains(line, "WDTT Panel:") {
			t.Fatalf("panel line in wdtt filter: %q", line)
		}
	}

	panel := fetchUnifiedBufferLogLines(50, "info", "panel")
	if len(panel) == 0 {
		t.Fatal("expected panel buffer lines")
	}
	for _, line := range panel {
		if !strings.Contains(line, "WDTT Panel:") {
			t.Fatalf("non-panel line in panel filter: %q", line)
		}
	}
}
