package panel

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

const ufwBeforeRulesPath = "/etc/ufw/before.rules"

var reICMPRuleTarget = regexp.MustCompile(` -j (ACCEPT|DROP)$`)

type ufwICMPSection int

const (
	ufwICMPNone ufwICMPSection = iota
	ufwICMPInput
	ufwICMPForward
)

func ufwPingBlocked() bool {
	return ufwPingBlockedIn(ufwBeforeRulesPath)
}

func ufwPingBlockedIn(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	section := ufwICMPNone
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if sec := ufwICMPHeaderSection(trimmed); sec != ufwICMPNone {
			section = sec
			continue
		}
		if trimmed == "" {
			section = ufwICMPNone
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !ufwICMPRuleMatchesSection(trimmed, section) {
			if section != ufwICMPNone && strings.Contains(trimmed, "-p icmp") {
				section = ufwICMPNone
			}
			continue
		}
		if strings.Contains(trimmed, "echo-request") {
			return strings.HasSuffix(trimmed, "-j DROP")
		}
	}
	return false
}

func isUFWICMPSectionHeader(line string) bool {
	return ufwICMPHeaderSection(line) != ufwICMPNone
}

func ufwICMPHeaderSection(line string) ufwICMPSection {
	switch {
	case strings.Contains(line, "# ok icmp codes for INPUT"):
		return ufwICMPInput
	case strings.Contains(line, "# ok icmp code for FORWARD"):
		return ufwICMPForward
	default:
		return ufwICMPNone
	}
}

func ufwICMPRuleMatchesSection(line string, section ufwICMPSection) bool {
	if !strings.Contains(line, "-p icmp") {
		return false
	}
	switch section {
	case ufwICMPInput:
		return strings.Contains(line, "-A ufw-before-input ")
	case ufwICMPForward:
		return strings.Contains(line, "-A ufw-before-forward ")
	default:
		return false
	}
}

func setUFWBlockPing(block bool) error {
	if !ufwInstalled() {
		return fmt.Errorf("ufw не установлен")
	}
	if err := setUFWBlockPingIn(ufwBeforeRulesPath, block); err != nil {
		return err
	}
	if ufwActive() {
		if _, err := runCmd("ufw", "reload"); err != nil {
			return fmt.Errorf("ufw reload: %w", err)
		}
	}
	return nil
}

func setUFWBlockPingIn(path string, block bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("чтение %s: %w", path, err)
	}
	target := "ACCEPT"
	if block {
		target = "DROP"
	}
	lines := strings.Split(string(data), "\n")
	section := ufwICMPNone
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if sec := ufwICMPHeaderSection(trimmed); sec != ufwICMPNone {
			section = sec
			continue
		}
		if trimmed == "" {
			section = ufwICMPNone
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !ufwICMPRuleMatchesSection(trimmed, section) {
			if section != ufwICMPNone && strings.Contains(trimmed, "-p icmp") {
				section = ufwICMPNone
			}
			continue
		}
		if reICMPRuleTarget.MatchString(trimmed) {
			lines[i] = reICMPRuleTarget.ReplaceAllString(trimmed, " -j "+target)
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("запись %s: %w", path, err)
	}
	return nil
}
