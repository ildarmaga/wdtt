package panel

import (
	"runtime"
	"testing"
)

func TestPanelReleaseBinName(t *testing.T) {
	name := panelReleaseBinName()
	switch runtime.GOARCH {
	case "arm64":
		if name != "wdtt-linux-arm64" {
			t.Fatalf("got %q want wdtt-linux-arm64", name)
		}
	default:
		if name != "wdtt-linux-amd64" {
			t.Fatalf("got %q want wdtt-linux-amd64", name)
		}
	}
}

func TestPanelInstallPathUnifiedFallback(t *testing.T) {
	if serviceUnitExists(panelServiceUnit) {
		t.Skip("legacy wdtt-panel.service present")
	}
	path := panelInstallPath()
	if path != wdttServerBin && path != "/usr/local/bin/wdtt" && path != "/usr/local/bin/wdtt-app" {
		t.Fatalf("unexpected install path: %s", path)
	}
}
