package panel

import (
	"os/exec"
	"strings"
)

func getRawIface() string {
	out, err := exec.Command("ip", "-o", "link", "show", "wdtt-raw").Output()
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return ""
	}
	return "wdtt-raw"
}
