package panel

// panelVersion задаётся при сборке: -ldflags "-X main.panelVersion=1.0.2"
// или через panel/build.sh / GitHub Actions (тег v*).
var panelVersion = "dev"

func FormatPanelVersion() string {
	return panelVersion
}
