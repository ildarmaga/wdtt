package panel

import (
	"os"
	"strconv"
	"strings"
)

const deployInboundEnvPath = wdttConfigDir + "/install-inbound.env"

// applyDeployInboundDefaults подставляет порты из deploy.sh (install-inbound.env) при первом seed panel.db.
func applyDeployInboundDefaults(cfg *WdttInboundConfig) {
	if cfg == nil {
		return
	}
	data, err := os.ReadFile(deployInboundEnvPath)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		p, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil || p <= 0 {
			continue
		}
		switch strings.TrimSpace(key) {
		case "DTLS_PORT":
			cfg.DtlsPort = p
		case "WG_PORT":
			cfg.WgPort = p
		}
	}
}
