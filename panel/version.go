package panel

// panelVersion задаётся при сборке: -ldflags "-X wdtt-panel.panelVersion=1.4.0"
// или через build.sh / GitHub Actions (тег v*).
	var panelVersion = "1.4.67"

func FormatPanelVersion() string {
	return panelVersion
}
