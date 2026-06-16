package panel

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func deployInboundEnvPath() string {
	return filepath.Join(wdttConfigDir, "install-inbound.env")
}

func deployMainPasswordPath() string {
	return filepath.Join(wdttConfigDir, "install-main-password.env")
}

func readDeployEnvValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

// readDeployMainPassword — пароль из install-main-password.env (install.sh / deploy).
func readDeployMainPassword() string {
	return readDeployEnvValue(deployMainPasswordPath(), "MAIN_PASSWORD")
}

// applyDeployInboundDefaults подставляет порты из deploy.sh (install-inbound.env) при первом seed panel.db.
func applyDeployInboundDefaults(cfg *WdttInboundConfig) {
	if cfg == nil {
		return
	}
	data, err := os.ReadFile(deployInboundEnvPath())
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
