package panel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	acmeAccountConfPath = "/root/.acme.sh/account.conf"
	acmeLECADir         = "/root/.acme.sh/ca/acme-v02.api.letsencrypt.org/directory"
	acmeLECAConfPath    = acmeLECADir + "/ca.conf"
)

func resolveAcmeContactEmail(cfg *PanelConfig, override string) string {
	if e := strings.TrimSpace(override); e != "" && isValidAcmeContactEmail(e) {
		return e
	}
	if cfg != nil {
		if d := strings.TrimSpace(strings.ToLower(cfg.panelDomain())); d != "" && !isValidIP(d) && isValidDomain(d) {
			e := "admin@" + d
			if isValidAcmeContactEmail(e) {
				return e
			}
		}
	}
	if e := detectAcmeHostnameEmail(); e != "" {
		return e
	}
	return ""
}

func isValidAcmeContactEmail(email string) bool {
	email = strings.TrimSpace(strings.ToLower(email))
	if !acmeEmailRe.MatchString(email) {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	switch domain {
	case "localhost", "local", "example.com", "example.org", "example.net", "invalid", "test":
		return false
	}
	if strings.HasSuffix(domain, ".localhost") || strings.HasSuffix(domain, ".local") ||
		strings.HasSuffix(domain, ".example") || strings.HasSuffix(domain, ".invalid") ||
		strings.HasSuffix(domain, ".test") {
		return false
	}
	return true
}

func detectAcmeHostnameEmail() string {
	out, _ := runCmd("hostname", "-f")
	host := strings.TrimSpace(strings.ToLower(out))
	if host == "" || host == "localhost" || !strings.Contains(host, ".") || isValidIP(host) {
		return ""
	}
	e := "admin@" + host
	if isValidAcmeContactEmail(e) {
		return e
	}
	return ""
}

func acmeAccountRegistered() bool {
	data, err := os.ReadFile(acmeLECAConfPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ACCOUNT_URL=") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(line, "ACCOUNT_URL="))
		val = strings.Trim(val, `"'`)
		return val != "" && strings.HasPrefix(val, "http")
	}
	return false
}

func setConfKey(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := []string{}
	if err == nil {
		lines = strings.Split(string(data), "\n")
	}
	prefix := key + "="
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = key + "='" + strings.ReplaceAll(value, "'", `'\"'`) + "'"
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, key+"='"+strings.ReplaceAll(value, "'", `'\"'`)+"'")
	}
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func writeAcmeContactEmail(email string) error {
	if err := setConfKey(acmeAccountConfPath, "ACCOUNT_EMAIL", email); err != nil {
		return fmt.Errorf("account.conf: %w", err)
	}
	if err := setConfKey(acmeLECAConfPath, "CA_EMAIL", email); err != nil {
		return fmt.Errorf("ca.conf: %w", err)
	}
	return nil
}

func prepareAcmeAccount(cfg *PanelConfig, overrideEmail string) error {
	if !acmeInstalled() {
		return fmt.Errorf("acme.sh не установлен")
	}
	email := resolveAcmeContactEmail(cfg, overrideEmail)
	if email == "" {
		return fmt.Errorf("укажите контактный email для Let's Encrypt (поле «ACME email» — реальный адрес, например you@gmail.com)")
	}
	if err := writeAcmeContactEmail(email); err != nil {
		return err
	}
	if acmeAccountRegistered() {
		out, err := runAcmeSh("--update-account", "-m", email)
		if err != nil && !strings.Contains(strings.ToLower(out), "success") {
			return fmt.Errorf("обновление ACME email: %s", tailLines(out, 6))
		}
		return nil
	}
	out, err := runAcmeSh("--register-account", "-m", email, "--server", "letsencrypt")
	if err != nil {
		return fmt.Errorf("регистрация ACME аккаунта: %s", tailLines(out, 8))
	}
	return nil
}
