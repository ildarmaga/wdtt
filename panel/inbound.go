package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	wdttInboundPath     = "/etc/wdtt/inbound.json"
	wdttServicePath     = "/etc/systemd/system/wdtt.service"
	wdttInboundTag      = "wdtt-in"
	wdttIptComment      = "WDTT_MANAGED"
	defaultDtlsPort     = 56000
	defaultWgPort       = 56001
	defaultClientPort   = 9000
	defaultClientDNS    = "1.1.1.1"
	defaultMaxUsers     = 10
	maxUsersSubnetLimit = 249
)

type WdttInboundConfig struct {
	Tag        string `json:"tag"`
	Remark     string `json:"remark"`
	ListenHost string `json:"listen_host"`
	ServerHost string `json:"server_host"`
	DtlsPort   int    `json:"dtls_port"`
	WgPort     int    `json:"wg_port"`
	ClientPort int    `json:"client_port"`
	DNS        string `json:"dns"`
	MaxUsers   int    `json:"max_users"`
}

type WdttInboundStatus struct {
	ServiceActive bool   `json:"service_active"`
	IfaceUp       bool   `json:"iface_up"`
	IfaceAddr     string `json:"iface_addr"`
	DtlsFirewall  bool   `json:"dtls_firewall"`
	WgFirewall    bool   `json:"wg_firewall"`
	DtlsListening bool   `json:"dtls_listening"`
	WgListening   bool   `json:"wg_listening"`
	ActiveUsers   int    `json:"active_users"`
	TotalUsers    int    `json:"total_users"`
	OnlineUsers   int    `json:"online_users"`
	MaxUsers      int    `json:"max_users"`
	XrayActive    bool   `json:"xray_active"`
}

func defaultWdttInbound() WdttInboundConfig {
	return WdttInboundConfig{
		Tag:        wdttInboundTag,
		Remark:     "WDTT",
		ListenHost: "0.0.0.0",
		DtlsPort:   defaultDtlsPort,
		WgPort:     defaultWgPort,
		ClientPort: defaultClientPort,
		DNS:        defaultClientDNS,
		MaxUsers:   defaultMaxUsers,
	}
}

func (c *WdttInboundConfig) normalize() {
	def := defaultWdttInbound()
	if c.Tag == "" {
		c.Tag = def.Tag
	}
	if c.Remark == "" {
		c.Remark = def.Remark
	}
	if c.ListenHost == "" {
		c.ListenHost = def.ListenHost
	}
	if c.DtlsPort <= 0 {
		c.DtlsPort = def.DtlsPort
	}
	if c.WgPort <= 0 {
		c.WgPort = def.WgPort
	}
	if c.ClientPort <= 0 {
		c.ClientPort = def.ClientPort
	}
	if strings.TrimSpace(c.DNS) == "" {
		c.DNS = def.DNS
	}
	if c.MaxUsers <= 0 {
		c.MaxUsers = def.MaxUsers
	}
}

func (c WdttInboundConfig) validate() error {
	c.normalize()
	if c.DtlsPort < 1 || c.DtlsPort > 65535 {
		return fmt.Errorf("DTLS порт должен быть от 1 до 65535")
	}
	if c.WgPort < 1 || c.WgPort > 65535 {
		return fmt.Errorf("WireGuard порт должен быть от 1 до 65535")
	}
	if c.ClientPort < 1 || c.ClientPort > 65535 {
		return fmt.Errorf("порт клиента должен быть от 1 до 65535")
	}
	if c.DtlsPort == c.WgPort {
		return fmt.Errorf("DTLS и WireGuard порты должны отличаться")
	}
	host := strings.TrimSpace(c.ListenHost)
	if host == "" {
		return fmt.Errorf("укажите адрес прослушивания")
	}
	dns := strings.TrimSpace(c.DNS)
	if dns == "" {
		return fmt.Errorf("укажите DNS для клиентов")
	}
	if !validDNSHost(dns) {
		return fmt.Errorf("неверный DNS: %s", dns)
	}
	if c.MaxUsers < 1 || c.MaxUsers > maxUsersSubnetLimit {
		return fmt.Errorf("лимит пользователей: от 1 до %d", maxUsersSubnetLimit)
	}
	return nil
}

func validDNSHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, part := range strings.Split(host, ".") {
		if part == "" || len(part) > 63 {
			return false
		}
		for _, ch := range part {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' {
				continue
			}
			return false
		}
	}
	return strings.Contains(host, ".")
}

func inboundMaxUsers() int {
	cfg, _ := loadWdttInbound()
	cfg.normalize()
	return cfg.MaxUsers
}

func collectWdttInboundStatus(cfg WdttInboundConfig) WdttInboundStatus {
	cfg.normalize()
	st := WdttInboundStatus{
		ServiceActive: serviceActive(wdttServiceUnit),
		IfaceAddr:     getWdttIface(),
		MaxUsers:      cfg.MaxUsers,
		XrayActive:    serviceActive(xrayServiceUnit),
	}
	st.IfaceUp = st.IfaceAddr != ""
	st.DtlsFirewall = iptablesUDPPortOpen(cfg.DtlsPort)
	st.WgFirewall = iptablesUDPPortOpen(cfg.WgPort)
	st.DtlsListening = udpPortListening(cfg.DtlsPort)
	st.WgListening = udpPortListening(cfg.WgPort)
	if db, _ := loadPasswords(); db != nil {
		st.TotalUsers = len(db.Passwords)
		st.ActiveUsers = countActivePasswords(db)
	}
	if stats := loadServerStats(); stats != nil {
		st.OnlineUsers = stats.ActiveUsers
	}
	return st
}

func iptablesUDPPortOpen(port int) bool {
	if port <= 0 {
		return false
	}
	portStr := strconv.Itoa(port)
	_, err := runCmd("iptables", "-C", "INPUT", "-p", "udp", "--dport", portStr,
		"-m", "comment", "--comment", wdttIptComment, "-j", "ACCEPT")
	return err == nil
}

func udpPortListening(port int) bool {
	if port <= 0 {
		return false
	}
	portStr := strconv.Itoa(port)
	out, _ := runCmd("ss", "-H", "-ulnp", "sport", "=", ":"+portStr)
	return strings.Contains(out, ":"+portStr)
}

func (c WdttInboundConfig) portsString() string {
	c.normalize()
	return fmt.Sprintf("%d,%d,%d", c.DtlsPort, c.WgPort, c.ClientPort)
}

func (c WdttInboundConfig) listenAddr() string {
	c.normalize()
	return fmt.Sprintf("%s:%d", strings.TrimSpace(c.ListenHost), c.DtlsPort)
}

func loadWdttInbound() (WdttInboundConfig, error) {
	cfg := defaultWdttInbound()
	data, err := os.ReadFile(wdttInboundPath)
	if err == nil {
		if json.Unmarshal(data, &cfg) == nil {
			cfg.normalize()
			return cfg, nil
		}
	}
	if svc, err := parseWdttInboundFromService(); err == nil {
		svc.normalize()
		return svc, nil
	}
	return cfg, nil
}

func saveWdttInbound(cfg WdttInboundConfig) error {
	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(wdttInboundPath), 0700)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(wdttInboundPath, data, 0600)
}

var (
	reListen   = regexp.MustCompile(`-listen\s+(\S+)`)
	reWgPort   = regexp.MustCompile(`-wg-port\s+(\d+)`)
	rePassword = regexp.MustCompile(`-password\s+(\S+)`)
)

func parseWdttInboundFromService() (WdttInboundConfig, error) {
	cfg := defaultWdttInbound()
	data, err := os.ReadFile(wdttServicePath)
	if err != nil {
		return cfg, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "ExecStart=") || strings.Contains(line, "ExecStartPre") {
			continue
		}
		if m := reListen.FindStringSubmatch(line); len(m) == 2 {
			host, portStr, err := netSplitHostPortSafe(m[1])
			if err == nil {
				if host != "" {
					cfg.ListenHost = host
				}
				if p, err := strconv.Atoi(portStr); err == nil {
					cfg.DtlsPort = p
				}
			}
		}
		if m := reWgPort.FindStringSubmatch(line); len(m) == 2 {
			if p, err := strconv.Atoi(m[1]); err == nil {
				cfg.WgPort = p
			}
		}
		break
	}
	cfg.normalize()
	return cfg, nil
}

func netSplitHostPortSafe(addr string) (host, port string, err error) {
	if strings.Count(addr, ":") == 0 {
		return "", addr, nil
	}
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return "", addr, fmt.Errorf("invalid addr")
	}
	return addr[:idx], addr[idx+1:], nil
}

func parseWdttServicePassword() string {
	data, err := os.ReadFile(wdttServicePath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "ExecStart=") && !strings.Contains(line, "ExecStartPre") {
			if m := rePassword.FindStringSubmatch(line); len(m) == 2 {
				return m[1]
			}
		}
	}
	return ""
}

func removeWdttFirewallPort(port int) {
	if port <= 0 {
		return
	}
	portStr := strconv.Itoa(port)
	runCmd("iptables", "-D", "INPUT", "-p", "udp", "--dport", portStr, "-m", "comment", "--comment", wdttIptComment, "-j", "ACCEPT")
}

func writeWdttServiceFile(cfg WdttInboundConfig, password string) error {
	cfg.normalize()
	extra := ""
	if password != "" {
		extra = " -password " + password
	}
	iptPre := fmt.Sprintf(
		`ExecStartPre=-/usr/bin/env bash -c "if command -v iptables >/dev/null 2>&1; then iptables -C INPUT -p udp --dport %d -m comment --comment %s -j ACCEPT 2>/dev/null || iptables -I INPUT -p udp --dport %d -m comment --comment %s -j ACCEPT; iptables -C INPUT -p udp --dport %d -m comment --comment %s -j ACCEPT 2>/dev/null || iptables -I INPUT -p udp --dport %d -m comment --comment %s -j ACCEPT; iptables -C INPUT -p tcp --dport 22 -m comment --comment %s -j ACCEPT 2>/dev/null || iptables -I INPUT -p tcp --dport 22 -m comment --comment %s -j ACCEPT; fi"`,
		cfg.DtlsPort, wdttIptComment, cfg.DtlsPort, wdttIptComment,
		cfg.WgPort, wdttIptComment, cfg.WgPort, wdttIptComment,
		wdttIptComment, wdttIptComment,
	)
	content := fmt.Sprintf(`[Unit]
Description=WDTT VPN Server
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStartPre=-/usr/bin/env bash -c "ip link show wdtt0 >/dev/null 2>&1 && ip link del wdtt0 2>/dev/null || true"
%s
ExecStart=/usr/local/bin/wdtt-server -listen %s -wg-port %d -config-dir %s%s
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
`, iptPre, cfg.listenAddr(), cfg.WgPort, wdttConfigDir, extra)
	return os.WriteFile(wdttServicePath, []byte(content), 0644)
}

func applyWdttInbound(cfg WdttInboundConfig) error {
	old, _ := loadWdttInbound()
	old.normalize()
	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return err
	}
	password := parseWdttServicePassword()
	if err := saveWdttInbound(cfg); err != nil {
		return err
	}
	if err := writeWdttServiceFile(cfg, password); err != nil {
		return err
	}
	if old.DtlsPort != cfg.DtlsPort {
		removeWdttFirewallPort(old.DtlsPort)
	}
	if old.WgPort != cfg.WgPort {
		removeWdttFirewallPort(old.WgPort)
	}
	runCmd("systemctl", "daemon-reload")
	return restartWdttWithDeps()
}

func resolveUserPorts(entry *PasswordEntry, inbound WdttInboundConfig) (dtls, wg, client int) {
	inbound.normalize()
	dtls, wg, client = inbound.DtlsPort, inbound.WgPort, inbound.ClientPort
	if entry == nil || entry.Ports == "" {
		return
	}
	parts := strings.Split(entry.Ports, ",")
	if len(parts) >= 1 {
		if p, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && p > 0 {
			dtls = p
		}
	}
	if len(parts) >= 2 {
		if p, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && p > 0 {
			wg = p
		}
	}
	if len(parts) >= 3 {
		if p, err := strconv.Atoi(strings.TrimSpace(parts[2])); err == nil && p > 0 {
			client = p
		}
	}
	return
}

func buildWdttLink(serverIP, password, vkHash string, entry *PasswordEntry, inbound WdttInboundConfig) string {
	link, err := buildWdttShareLink(serverIP, password, "", "", "", vkHash, entry, inbound)
	if err != nil {
		return ""
	}
	return link
}
