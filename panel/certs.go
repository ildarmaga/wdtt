package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	acmeCertRoot       = "/root/cert"
	letsencryptLiveDir = "/etc/letsencrypt/live"
	wdttDtlsCertFile   = "/etc/wdtt/dtls-cert.pem"
	wdttDtlsKeyFile    = "/etc/wdtt/dtls-key.pem"
)

type CertInfo struct {
	ID          string   `json:"id"`
	Source      string   `json:"source"`
	Domain      string   `json:"domain"`
	Domains     []string `json:"domains"`
	CertFile    string   `json:"certFile"`
	KeyFile     string   `json:"keyFile"`
	NotBefore   string   `json:"notBefore"`
	NotAfter    string   `json:"notAfter"`
	Issuer      string   `json:"issuer"`
	DaysLeft    int      `json:"daysLeft"`
	PanelActive bool     `json:"panelActive"`
	AcmeManaged bool     `json:"acmeManaged"`
}

type AcmeEntry struct {
	Domain    string `json:"domain"`
	KeyLength string `json:"keyLength"`
	SAN       string `json:"san"`
	CA        string `json:"ca"`
	Created   string `json:"created"`
	Renew     string `json:"renew"`
}

type AcmeStatus struct {
	Installed     bool        `json:"installed"`
	Path          string      `json:"path"`
	CronInstalled bool        `json:"cronInstalled"`
	CronPath      string      `json:"cronPath"`
	CronHour      int         `json:"cronHour"`
	CronMinute    int         `json:"cronMinute"`
	CronSchedule  string      `json:"cronSchedule"`
	Entries       []AcmeEntry `json:"entries"`
}

var domainRe = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+)$`)
var acmeEmailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const acmeFallbackEmail = "noreply@example.com"

func acmeShBin() string {
	return "/root/.acme.sh/acme.sh"
}

func runAcmeSh(args ...string) (string, error) {
	if acmeJobActive() {
		return runAcmeShStream(300*time.Second, args...)
	}
	cmd := exec.Command(acmeShBin(), args...)
	cmd.Env = append(os.Environ(), "HOME=/root")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runAcmeShTimeout(timeout time.Duration, args ...string) (string, error) {
	if acmeJobActive() {
		return runAcmeShStream(timeout, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, acmeShBin(), args...)
	cmd.Env = append(os.Environ(), "HOME=/root")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func acmeInstalled() bool {
	st, err := os.Stat(acmeShBin())
	return err == nil && st.Mode().IsRegular() && st.Mode()&0111 != 0
}

func panelReloadCmd() string {
	return "systemctl restart " + panelServiceUnit
}

func inspectCertPair(certFile, keyFile string) (*CertInfo, error) {
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if certFile == "" || keyFile == "" {
		return nil, fmt.Errorf("empty cert paths")
	}
	if _, err := os.Stat(certFile); err != nil {
		return nil, err
	}
	if _, err := os.Stat(keyFile); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("invalid PEM: %s", certFile)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	domains := certDNSNames(cert)
	domain := domains[0]
	if domain == "" {
		domain = filepath.Base(filepath.Dir(certFile))
	}
	info := &CertInfo{
		Domain:    domain,
		Domains:   domains,
		CertFile:  certFile,
		KeyFile:   keyFile,
		NotBefore: cert.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:  cert.NotAfter.UTC().Format(time.RFC3339),
		Issuer:    cert.Issuer.CommonName,
		DaysLeft:  int(time.Until(cert.NotAfter).Hours() / 24),
	}
	if info.Issuer == "" && len(cert.Issuer.Organization) > 0 {
		info.Issuer = cert.Issuer.Organization[0]
	}
	return info, nil
}

func certDNSNames(cert *x509.Certificate) []string {
	seen := map[string]bool{}
	var out []string
	if cert.Subject.CommonName != "" {
		seen[cert.Subject.CommonName] = true
		out = append(out, cert.Subject.CommonName)
	}
	for _, d := range cert.DNSNames {
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, ip := range cert.IPAddresses {
		s := ip.String()
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func listCertificates(cfg *PanelConfig) ([]CertInfo, error) {
	byCert := map[string]CertInfo{}
	add := func(source, id string, certFile, keyFile string, acmeManaged bool) {
		info, err := inspectCertPair(certFile, keyFile)
		if err != nil {
			return
		}
		info.Source = source
		info.ID = id
		info.AcmeManaged = acmeManaged
		if prev, ok := byCert[info.CertFile]; ok {
			if prev.PanelActive {
				info.PanelActive = true
			}
		}
		byCert[info.CertFile] = *info
	}

	entries, _ := filepath.Glob(filepath.Join(acmeCertRoot, "*", "fullchain.pem"))
	for _, certFile := range entries {
		dir := filepath.Dir(certFile)
		keyFile := filepath.Join(dir, "privkey.pem")
		name := filepath.Base(dir)
		add("acme", "acme:"+name, certFile, keyFile, true)
	}

	if dirs, err := os.ReadDir(letsencryptLiveDir); err == nil {
		for _, d := range dirs {
			if !d.IsDir() || d.Name() == "README" {
				continue
			}
			dir := filepath.Join(letsencryptLiveDir, d.Name())
			add("letsencrypt", "le:"+d.Name(),
				filepath.Join(dir, "fullchain.pem"),
				filepath.Join(dir, "privkey.pem"), false)
		}
	}

	if cfg != nil && strings.TrimSpace(cfg.WebCertFile) != "" {
		certFile := strings.TrimSpace(cfg.WebCertFile)
		keyFile := strings.TrimSpace(cfg.WebKeyFile)
		if prev, ok := byCert[certFile]; ok {
			prev.PanelActive = true
			byCert[certFile] = prev
		} else if info, err := inspectCertPair(certFile, keyFile); err == nil {
			info.Source = "panel"
			info.ID = "panel:custom"
			info.PanelActive = true
			byCert[certFile] = *info
		}
	}

	if _, err := os.Stat(wdttDtlsCertFile); err == nil {
		add("wdtt", "wdtt:dtls", wdttDtlsCertFile, wdttDtlsKeyFile, false)
	}

	out := make([]CertInfo, 0, len(byCert))
	for _, c := range byCert {
		if cfg != nil {
			c.PanelActive = samePath(c.CertFile, cfg.WebCertFile) && samePath(c.KeyFile, cfg.WebKeyFile)
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PanelActive != out[j].PanelActive {
			return out[i].PanelActive
		}
		return out[i].Domain < out[j].Domain
	})
	return out, nil
}

func samePath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return aa == bb
}

func acmeStatus() AcmeStatus {
	st := AcmeStatus{
		Path:         acmeShBin(),
		CronPath:     acmeCronFilePath,
		CronHour:     acmeCronDefaultHour,
		CronMinute:   acmeCronDefaultMinute,
		CronSchedule: acmeCronScheduleText(acmeCronDefaultHour, acmeCronDefaultMinute),
		Entries:      []AcmeEntry{},
	}
	if !acmeInstalled() {
		return st
	}
	st.Installed = true
	st.CronInstalled = acmeCronInstalled()
	st.CronPath = acmeCronFilePath
	hour, minute := acmeCronDefaultHour, acmeCronDefaultMinute
	if st.CronInstalled {
		hour, minute, _ = parseAcmeCronSchedule()
	}
	st.CronHour = hour
	st.CronMinute = minute
	st.CronSchedule = acmeCronScheduleText(hour, minute)
	st.Entries = []AcmeEntry{}
	out, err := runAcmeSh("--list")
	if err != nil {
		return st
	}
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		entry := AcmeEntry{Domain: fields[0]}
		if len(fields) > 1 {
			entry.KeyLength = fields[1]
		}
		if len(fields) > 2 {
			entry.SAN = fields[2]
		}
		if len(fields) > 3 {
			entry.CA = fields[3]
		}
		if len(fields) > 4 {
			entry.Created = fields[4]
		}
		if len(fields) > 5 {
			entry.Renew = fields[5]
		}
		st.Entries = append(st.Entries, entry)
	}
	return st
}

func runCmdTimeout(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func installAcme(cfg *PanelConfig) error {
	if acmeInstalled() {
		ensureAcmeContactEmail(cfg)
		return ensureAcmeCron()
	}
	email := acmeContactEmail(cfg)
	script := fmt.Sprintf("export HOME=/root; cd /root && curl -fsSL https://get.acme.sh | sh -s email=%s", shellSingleQuote(email))
	_, err := runCmdTimeout(120*time.Second, "bash", "-c", script)
	if err != nil {
		return fmt.Errorf("curl|sh: %w", err)
	}
	if !acmeInstalled() {
		return fmt.Errorf("acme.sh не найден после установки (%s)", acmeShBin())
	}
	ensureAcmeContactEmail(cfg)
	return ensureAcmeCron()
}

func acmeContactEmail(cfg *PanelConfig) string {
	if cfg != nil {
		d := strings.TrimSpace(strings.ToLower(cfg.WebDomain))
		if d != "" && !isValidIP(d) && isValidDomain(d) {
			return "admin@" + d
		}
	}
	return acmeFallbackEmail
}

func ensureAcmeContactEmail(cfg *PanelConfig) {
	if !acmeInstalled() {
		return
	}
	email := acmeContactEmail(cfg)
	if !acmeEmailRe.MatchString(email) {
		return
	}
	_, _ = runAcmeSh("--update-account", "--accountemail", email)
}

func acmeIssueEmailArgs(cfg *PanelConfig) []string {
	return []string{"--accountemail", acmeContactEmail(cfg)}
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func ensureSocat() {
	_, err := exec.LookPath("socat")
	if err == nil {
		return
	}
	_, _ = runCmd("bash", "-c", "apt-get update >/dev/null 2>&1 && apt-get install -y socat >/dev/null 2>&1")
}

func isValidDomain(domain string) bool {
	domain = strings.TrimSpace(domain)
	if domain == "" || len(domain) > 253 {
		return false
	}
	return domainRe.MatchString(domain)
}

func isValidIP(s string) bool {
	return net.ParseIP(strings.TrimSpace(s)) != nil
}

func acmeListenFlag() string {
	out, _ := runCmd("bash", "-c", "ip -6 route show default 2>/dev/null | head -1")
	if strings.TrimSpace(out) != "" {
		return "--listen-v6"
	}
	return ""
}

func issueAcmeDomain(domain string, httpPort int, applyPanel bool, cfg *PanelConfig) (map[string]interface{}, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if !isValidDomain(domain) {
		return nil, fmt.Errorf("некорректное имя домена")
	}
	if httpPort < 1 || httpPort > 65535 {
		httpPort = 80
	}
	if err := installAcme(cfg); err != nil {
		return nil, fmt.Errorf("не удалось установить acme.sh: %w", err)
	}
	ensureSocat()
	ensureAcmeContactEmail(cfg)

	return withAcmeHTTPPort(httpPort, func() (map[string]interface{}, error) {
		certDir := filepath.Join(acmeCertRoot, domain)
		_ = os.RemoveAll(certDir)
		if err := os.MkdirAll(certDir, 0755); err != nil {
			return nil, err
		}

		_, _ = runAcmeSh("--set-default-ca", "--server", "letsencrypt", "--force")
		issueArgs := append([]string{"--issue", "-d", domain, acmeListenFlag(), "--standalone", "--httpport", fmt.Sprint(httpPort), "--force"}, acmeIssueEmailArgs(cfg)...)
		issueOut, err := runAcmeShTimeout(180*time.Second, issueArgs...)
		if err != nil {
			_ = os.RemoveAll(filepath.Join("/root", ".acme.sh", domain))
			_ = os.RemoveAll(filepath.Join("/root", ".acme.sh", domain+"_ecc"))
			return nil, fmt.Errorf("выпуск сертификата не удался: %s", tailLines(issueOut, 6))
		}

		certFile := filepath.Join(certDir, "fullchain.pem")
		keyFile := filepath.Join(certDir, "privkey.pem")
		installOut, err := runAcmeSh("--installcert", "-d", domain,
			"--key-file", keyFile,
			"--fullchain-file", certFile,
			"--reloadcmd", panelReloadCmd())
		if err != nil && !strings.Contains(installOut, "Installing key to:") {
			return nil, fmt.Errorf("установка сертификата: %s", tailLines(installOut, 6))
		}
		_ = os.Chmod(keyFile, 0600)
		_ = os.Chmod(certFile, 0644)
		_, _ = runAcmeSh("--upgrade", "--auto-upgrade")
		_ = ensureAcmeCron()

		if applyPanel && cfg != nil {
			if err := applyPanelCert(cfg, certFile, keyFile, false); err != nil {
				return nil, err
			}
		}

		certs, _ := listCertificates(cfg)
		fullLog := issueOut
		if installOut != "" {
			fullLog += "\n" + installOut
		}
		return map[string]interface{}{
			"message":  "Сертификат выпущен для " + domain,
			"log":      fullLog,
			"certFile": certFile,
			"keyFile":  keyFile,
			"certs":    certs,
			"acme":     acmeStatus(),
		}, nil
	})
}

func issueAcmeIP(ip, ipv6 string, httpPort int, applyPanel bool, cfg *PanelConfig) (map[string]interface{}, error) {
	if ip == "" {
		ip = detectPublicIPv4()
	}
	ip = strings.TrimSpace(ip)
	if !isValidIP(ip) {
		return nil, fmt.Errorf("некорректный IPv4")
	}
	if httpPort < 1 || httpPort > 65535 {
		httpPort = 80
	}
	if err := installAcme(cfg); err != nil {
		return nil, fmt.Errorf("не удалось установить acme.sh: %w", err)
	}
	ensureSocat()
	ensureAcmeContactEmail(cfg)

	return withAcmeHTTPPort(httpPort, func() (map[string]interface{}, error) {
		certDir := filepath.Join(acmeCertRoot, "ip")
		_ = os.MkdirAll(certDir, 0755)
		args := []string{"--issue", "-d", ip, "--standalone", "--server", "letsencrypt",
			"--certificate-profile", "shortlived", "--days", "6", "--httpport", fmt.Sprint(httpPort), "--force"}
		args = append(args, acmeIssueEmailArgs(cfg)...)
		if strings.TrimSpace(ipv6) != "" && isValidIP(ipv6) {
			args = append(args, "-d", strings.TrimSpace(ipv6))
		}
		_, _ = runAcmeSh("--set-default-ca", "--server", "letsencrypt", "--force")
		issueOut, err := runAcmeShTimeout(180*time.Second, args...)
		if err != nil {
			return nil, fmt.Errorf("выпуск IP-сертификата не удался: %s", tailLines(issueOut, 6))
		}

		certFile := filepath.Join(certDir, "fullchain.pem")
		keyFile := filepath.Join(certDir, "privkey.pem")
		installOut, _ := runAcmeSh("--installcert", "-d", ip,
			"--key-file", keyFile,
			"--fullchain-file", certFile,
			"--reloadcmd", panelReloadCmd())

		if _, err := os.Stat(certFile); err != nil {
			return nil, fmt.Errorf("файлы сертификата не найдены: %s", tailLines(installOut, 6))
		}
		_ = os.Chmod(keyFile, 0600)
		_ = os.Chmod(certFile, 0644)
		_ = ensureAcmeCron()

		if applyPanel && cfg != nil {
			if err := applyPanelCert(cfg, certFile, keyFile, false); err != nil {
				return nil, err
			}
		}

		certs, _ := listCertificates(cfg)
		fullLog := issueOut
		if installOut != "" {
			fullLog += "\n" + installOut
		}
		return map[string]interface{}{
			"message":  "IP-сертификат выпущен для " + ip,
			"log":      fullLog,
			"certFile": certFile,
			"keyFile":  keyFile,
			"certs":    certs,
			"acme":     acmeStatus(),
		}, nil
	})
}

func renewAcmeCert(domain string) (map[string]interface{}, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, fmt.Errorf("укажите домен")
	}
	if !acmeInstalled() {
		return nil, fmt.Errorf("acme.sh не установлен")
	}
	return withAcmeHTTPPort(80, func() (map[string]interface{}, error) {
		out, err := runAcmeShTimeout(180*time.Second, "--renew", "-d", domain, "--force")
		if err != nil {
			return nil, fmt.Errorf("обновление не удалось: %s", tailLines(out, 6))
		}
		return map[string]interface{}{
			"message": "Сертификат обновлён",
			"log":     out,
			"acme":    acmeStatus(),
		}, nil
	})
}

func revokeAcmeCert(domain string, cfg *PanelConfig) (map[string]interface{}, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, fmt.Errorf("укажите домен")
	}
	if !acmeInstalled() {
		return nil, fmt.Errorf("acme.sh не установлен")
	}

	acmeIDs := domain
	if domain == "ip" {
		listOut, _ := runAcmeSh( "--list")
		var ips []string
		for _, line := range strings.Split(listOut, "\n")[1:] {
			f := strings.Fields(line)
			if len(f) == 0 {
				continue
			}
			if isValidIP(f[0]) {
				ips = append(ips, f[0])
			}
		}
		acmeIDs = strings.Join(ips, " ")
	}

	for _, id := range strings.Fields(acmeIDs) {
		_, _ = runAcmeSh( "--revoke", "-d", id)
		_, _ = runAcmeSh( "--remove", "-d", id)
		home, _ := os.UserHomeDir()
		if home == "" {
			home = "/root"
		}
		_ = os.RemoveAll(filepath.Join(home, ".acme.sh", id))
		_ = os.RemoveAll(filepath.Join(home, ".acme.sh", id+"_ecc"))
	}
	_ = os.RemoveAll(filepath.Join(acmeCertRoot, domain))

	if cfg != nil && strings.HasPrefix(strings.TrimSpace(cfg.WebCertFile), filepath.Join(acmeCertRoot, domain)) {
		cfg.WebCertFile = ""
		cfg.WebKeyFile = ""
		_ = savePanelConfig(cfg)
	}

	certs, _ := listCertificates(cfg)
	return map[string]interface{}{
		"message": "Сертификат отозван и удалён",
		"certs":   certs,
		"acme":    acmeStatus(),
	}, nil
}

func deleteLetsEncryptCert(name string, cfg *PanelConfig) (map[string]interface{}, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "README" {
		return nil, fmt.Errorf("укажите имя сертификата")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return nil, fmt.Errorf("некорректное имя сертификата")
	}

	liveDir := filepath.Join(letsencryptLiveDir, name)
	if st, err := os.Stat(liveDir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("сертификат Certbot не найден: %s", name)
	}

	certFile := filepath.Join(liveDir, "fullchain.pem")
	if cfg != nil && samePath(certFile, cfg.WebCertFile) {
		return nil, fmt.Errorf("этот сертификат используется панелью — сначала выберите другой")
	}

	var logOut string
	if _, err := exec.LookPath("certbot"); err == nil {
		out, err := runCmdTimeout(60*time.Second, "certbot", "delete", "--cert-name", name, "-n")
		logOut = out
		if err != nil {
			return nil, fmt.Errorf("certbot delete: %s", tailLines(out, 6))
		}
	} else {
		if err := removeLetsEncryptFiles(name); err != nil {
			return nil, err
		}
		logOut = "certbot не установлен, файлы удалены вручную"
	}

	certs, _ := listCertificates(cfg)
	return map[string]interface{}{
		"message": "Сертификат Certbot удалён",
		"log":     tailLines(logOut, 8),
		"certs":   certs,
	}, nil
}

func removeLetsEncryptFiles(name string) error {
	_ = os.RemoveAll(filepath.Join(letsencryptLiveDir, name))
	_ = os.RemoveAll(filepath.Join("/etc/letsencrypt/archive", name))
	_ = os.Remove(filepath.Join("/etc/letsencrypt/renewal", name+".conf"))
	if _, err := os.Stat(filepath.Join(letsencryptLiveDir, name)); err == nil {
		return fmt.Errorf("не удалось удалить %s", name)
	}
	return nil
}

func applyPanelCert(cfg *PanelConfig, certFile, keyFile string, restart bool) error {
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if err := validatePanelTLS(certFile, keyFile); err != nil {
		return err
	}
	cfg.WebCertFile = certFile
	cfg.WebKeyFile = keyFile
	if cfg.WebDomain == "" {
		if d := domainFromCertPath(certFile); d != "" {
			cfg.WebDomain = d
		}
	}
	if err := savePanelConfig(cfg); err != nil {
		return err
	}
	if restart {
		go func() {
			time.Sleep(400 * time.Millisecond)
			_ = serviceRestart(panelServiceUnit)
		}()
	}
	return nil
}

func detectPublicIPv4() string {
	urls := []string{
		"https://api4.ipify.org",
		"https://ipv4.icanhazip.com",
		"https://v4.api.ipinfo.io/ip",
	}
	for _, u := range urls {
		out, err := runCmdTimeout(5*time.Second, "curl", "-4", "-fsSL", "--max-time", "3", u)
		if err == nil && isValidIP(out) {
			return strings.TrimSpace(out)
		}
	}
	return ""
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) <= n {
		return strings.TrimSpace(s)
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
