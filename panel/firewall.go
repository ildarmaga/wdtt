package panel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const panelIptComment = "WDTT_PANEL"
const serverUFWComment = "WDTT_SERVER"
const sshUFWComment = "WDTT_SSH"
const sshConfigPath = "/etc/ssh/sshd_config"

type FirewallPort struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Comment  string `json:"comment"`
	Action   string `json:"action"`
	Backend  string `json:"backend"`
	IPv6     bool   `json:"ipv6"`
	RuleIDs  []int  `json:"rule_ids"`
	Managed  bool   `json:"managed"`
}

var (
	reIptInputAccept = regexp.MustCompile(`^-A INPUT -p (tcp|udp) -m (?:tcp|udp) --dport (\d+)(?: -m comment --comment ([^ ]+))? -j ACCEPT`)
	reUfwRuleLine    = regexp.MustCompile(`^\[\s*(\d+)\]\s+(\d+)/(tcp|udp)(\s+\(v6\))?\s+ALLOW\s+IN\b`)
)

func ufwActive() bool {
	out, err := runCmd("ufw", "status")
	if err != nil {
		return false
	}
	first := strings.TrimSpace(strings.Split(out, "\n")[0])
	return strings.EqualFold(first, "Status: active")
}

func ufwInstalled() bool {
	_, err := exec.LookPath("ufw")
	return err == nil
}

func detectSSHPort() int {
	port := 22
	paths := []string{sshConfigPath}
	if entries, err := os.ReadDir("/etc/ssh/sshd_config.d"); err == nil {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
				continue
			}
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			paths = append(paths, filepath.Join("/etc/ssh/sshd_config.d", name))
		}
	}
	for _, path := range paths {
		parseSSHPortFile(path, &port)
	}
	return port
}

func parseSSHPortFile(path string, port *int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "Port") {
			if p, err := strconv.Atoi(fields[1]); err == nil && p >= 1 && p <= 65535 {
				*port = p
			}
		}
	}
}

func enableUFW(cfg *PanelConfig) (map[string]interface{}, error) {
	if !ufwInstalled() {
		return nil, fmt.Errorf("ufw не установлен (apt install ufw)")
	}
	if ufwActive() {
		return nil, fmt.Errorf("UFW уже активен")
	}
	sshPort := detectSSHPort()
	rule := fmt.Sprintf("%d/tcp", sshPort)
	if _, err := runCmd("ufw", "allow", rule, "comment", sshUFWComment); err != nil {
		return nil, fmt.Errorf("не удалось открыть SSH %s: %w", rule, err)
	}
	if cfg != nil && cfg.Port >= 1 && cfg.Port <= 65535 {
		panelRule := fmt.Sprintf("%d/tcp", cfg.Port)
		_, _ = runCmd("ufw", "allow", panelRule, "comment", panelIptComment)
	}
	if inb, err := loadInboundNorm(); err == nil {
		for _, rule := range []string{
			fmt.Sprintf("%d/udp", inb.DtlsPort),
			fmt.Sprintf("%d/udp", inb.WgPort),
		} {
			_, _ = runCmd("ufw", "allow", rule, "comment", serverUFWComment)
		}
	} else {
		for _, rule := range []string{"56000/udp", "56001/udp"} {
			_, _ = runCmd("ufw", "allow", rule, "comment", serverUFWComment)
		}
	}
	if _, err := runCmd("ufw", "--force", "enable"); err != nil {
		return nil, fmt.Errorf("ufw enable: %w", err)
	}
	if cfg != nil && cfg.BlockPing {
		_ = setUFWBlockPing(true)
	}
	ports, _ := listFirewallPorts()
	if ports == nil {
		ports = []FirewallPort{}
	}
	return map[string]interface{}{
		"message":    fmt.Sprintf("UFW включён. SSH %d/tcp открыт автоматически.", sshPort),
		"ssh_port":   sshPort,
		"ports":      ports,
		"backend":    firewallBackend(),
		"ufw_active": ufwActive(),
	}, nil
}

func firewallStatusPayload(cfg *PanelConfig) map[string]interface{} {
	ports, err := listFirewallPorts()
	if err != nil {
		ports = []FirewallPort{}
	}
	return map[string]interface{}{
		"ports":         ports,
		"backend":       firewallBackend(),
		"ufw_active":    ufwActive(),
		"ufw_installed": ufwInstalled(),
		"ssh_port":      detectSSHPort(),
	}
}

func listFirewallPorts() ([]FirewallPort, error) {
	if ufwActive() {
		return listUFWPorts()
	}
	return listIptablesPorts()
}

func listUFWPorts() ([]FirewallPort, error) {
	out, err := runCmd("ufw", "status", "numbered")
	if err != nil {
		return nil, fmt.Errorf("ufw status: %w", err)
	}
	type acc struct {
		FirewallPort
		hasV4 bool
		hasV6 bool
	}
	byKey := map[string]*acc{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		m := reUfwRuleLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ruleID, _ := strconv.Atoi(m[1])
		port, _ := strconv.Atoi(m[2])
		proto := m[3]
		isV6 := strings.TrimSpace(m[4]) != ""
		key := proto + "/" + m[2]
		comment := ufwLineComment(line)
		entry, ok := byKey[key]
		if !ok {
			entry = &acc{FirewallPort: FirewallPort{
				Protocol: proto,
				Port:     port,
				Comment:  comment,
				Action:   "ALLOW",
				Backend:  "ufw",
				Managed:  isSensitiveFirewallPort(port, proto),
			}}
			byKey[key] = entry
		}
		entry.RuleIDs = append(entry.RuleIDs, ruleID)
		if isV6 {
			entry.hasV6 = true
		} else {
			entry.hasV4 = true
		}
		if entry.Comment == "" || entry.Comment == "—" {
			entry.Comment = comment
		}
	}
	list := make([]FirewallPort, 0, len(byKey))
	for _, a := range byKey {
		a.IPv6 = a.hasV6
		sort.Ints(a.RuleIDs)
		list = append(list, a.FirewallPort)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Port != list[j].Port {
			return list[i].Port < list[j].Port
		}
		return list[i].Protocol < list[j].Protocol
	})
	return list, nil
}

func ufwLineComment(line string) string {
	if idx := strings.Index(line, "#"); idx >= 0 {
		c := strings.TrimSpace(line[idx+1:])
		if c != "" {
			return c
		}
	}
	return "—"
}

func listIptablesPorts() ([]FirewallPort, error) {
	out, err := runCmd("iptables-save")
	if err != nil {
		return nil, fmt.Errorf("iptables-save: %w", err)
	}
	byKey := map[string]FirewallPort{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-A INPUT") || !strings.Contains(line, "--dport") || !strings.HasSuffix(line, "-j ACCEPT") {
			continue
		}
		m := reIptInputAccept.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		port, _ := strconv.Atoi(m[2])
		if port <= 0 || port > 65535 {
			continue
		}
		comment := strings.TrimSpace(m[3])
		if comment == "" {
			comment = "—"
		}
		key := m[1] + "/" + m[2]
		entry := FirewallPort{
			Protocol: m[1],
			Port:     port,
			Comment:  comment,
			Action:   "ALLOW",
			Backend:  "iptables",
			Managed:  comment == wdttIptComment || isSensitiveFirewallPort(port, m[1]),
		}
		if prev, ok := byKey[key]; ok && prev.Managed {
			entry = prev
		}
		byKey[key] = entry
	}
	list := make([]FirewallPort, 0, len(byKey))
	for _, p := range byKey {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Port != list[j].Port {
			return list[i].Port < list[j].Port
		}
		return list[i].Protocol < list[j].Protocol
	})
	return list, nil
}

func isSensitiveFirewallPort(port int, proto string) bool {
	if proto == "tcp" && port == detectSSHPort() {
		return true
	}
	switch proto {
	case "tcp":
		return port == 22 || port == 4822 || port == 2860 || port == 8443
	case "udp":
		return port == 56000 || port == 56001
	}
	return false
}

func normalizeFirewallProto(proto string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(proto))
	switch p {
	case "tcp", "udp":
		return p, nil
	default:
		return "", fmt.Errorf("протокол: tcp или udp")
	}
}

func validateFirewallPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("порт: 1–65535")
	}
	return nil
}

func firewallPortOpen(proto string, port int, comment string) error {
	proto, err := normalizeFirewallProto(proto)
	if err != nil {
		return err
	}
	if err := validateFirewallPort(port); err != nil {
		return err
	}
	if ufwActive() {
		rule := fmt.Sprintf("%d/%s", port, proto)
		args := []string{"allow", rule}
		if c := normalizeFirewallComment(comment); c != "" {
			args = append(args, "comment", c)
		}
		if _, err := runCmd("ufw", args...); err != nil {
			return fmt.Errorf("ufw allow: %w", err)
		}
		return nil
	}
	portStr := strconv.Itoa(port)
	iptComment := panelIptComment
	if c := normalizeFirewallComment(comment); c != "" {
		iptComment = c
	}
	if _, err := runCmd("iptables", "-C", "INPUT", "-p", proto, "--dport", portStr,
		"-m", "comment", "--comment", iptComment, "-j", "ACCEPT"); err == nil {
		return nil
	}
	_, err = runCmd("iptables", "-I", "INPUT", "-p", proto, "--dport", portStr,
		"-m", "comment", "--comment", iptComment, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("iptables: %w", err)
	}
	return nil
}

func normalizeFirewallComment(comment string) string {
	c := strings.TrimSpace(comment)
	if c == "" || c == "—" {
		return ""
	}
	return c
}

func firewallPortUpdate(oldProto string, oldPort int, newProto string, newPort int, comment string) error {
	oldProto, err := normalizeFirewallProto(oldProto)
	if err != nil {
		return err
	}
	newProto, err = normalizeFirewallProto(newProto)
	if err != nil {
		return err
	}
	if err := validateFirewallPort(oldPort); err != nil {
		return err
	}
	if err := validateFirewallPort(newPort); err != nil {
		return err
	}
	if oldProto == newProto && oldPort == newPort {
		if err := firewallPortClose(oldProto, oldPort); err != nil {
			return err
		}
		return firewallPortOpen(newProto, newPort, comment)
	}
	if err := firewallPortClose(oldProto, oldPort); err != nil {
		return err
	}
	return firewallPortOpen(newProto, newPort, comment)
}

func ufwRuleIDsFor(port int, proto string) ([]int, error) {
	out, err := runCmd("ufw", "status", "numbered")
	if err != nil {
		return nil, err
	}
	portStr := strconv.Itoa(port)
	var ids []int
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		m := reUfwRuleLine.FindStringSubmatch(line)
		if m == nil || m[2] != portStr || m[3] != proto {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		ids = append(ids, id)
	}
	return ids, nil
}

func firewallPortClose(proto string, port int) error {
	proto, err := normalizeFirewallProto(proto)
	if err != nil {
		return err
	}
	if err := validateFirewallPort(port); err != nil {
		return err
	}
	if ufwActive() {
		ids, err := ufwRuleIDsFor(port, proto)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return fmt.Errorf("правило UFW не найдено")
		}
		sort.Sort(sort.Reverse(sort.IntSlice(ids)))
		for _, id := range ids {
			if _, err := runCmd("ufw", "--force", "delete", strconv.Itoa(id)); err != nil {
				return fmt.Errorf("ufw delete: %w", err)
			}
		}
		return nil
	}
	portStr := strconv.Itoa(port)
	removed := false
	for _, comment := range []string{panelIptComment, wdttIptComment} {
		for {
			_, err := runCmd("iptables", "-D", "INPUT", "-p", proto, "--dport", portStr,
				"-m", "comment", "--comment", comment, "-j", "ACCEPT")
			if err != nil {
				break
			}
			removed = true
		}
	}
	if !removed {
		return fmt.Errorf("правило не найдено")
	}
	return nil
}

func firewallBackend() string {
	if ufwActive() {
		return "ufw"
	}
	return "iptables"
}
