package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

const ufwBeforeRulesPath = "/etc/ufw/before.rules"

var reICMPRuleTarget = regexp.MustCompile(` -j (ACCEPT|DROP)$`)

func ufwPingBlocked() bool {
	data, err := os.ReadFile(ufwBeforeRulesPath)
	if err != nil {
		return false
	}
	inSection := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if isUFWICMPSectionHeader(trimmed) {
			inSection = true
			continue
		}
		if trimmed == "" {
			inSection = false
			continue
		}
		if !inSection || !strings.Contains(trimmed, "-p icmp") {
			if inSection && !strings.Contains(trimmed, "-p icmp") {
				inSection = false
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
	return strings.Contains(line, "# ok icmp codes for INPUT") ||
		strings.Contains(line, "# ok icmp code for FORWARD")
}

func setUFWBlockPing(block bool) error {
	if !ufwInstalled() {
		return fmt.Errorf("ufw не установлен")
	}
	data, err := os.ReadFile(ufwBeforeRulesPath)
	if err != nil {
		return fmt.Errorf("чтение %s: %w", ufwBeforeRulesPath, err)
	}
	target := "ACCEPT"
	if block {
		target = "DROP"
	}
	lines := strings.Split(string(data), "\n")
	inSection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isUFWICMPSectionHeader(trimmed) {
			inSection = true
			continue
		}
		if trimmed == "" {
			inSection = false
			continue
		}
		if !inSection {
			continue
		}
		if !strings.Contains(trimmed, "-p icmp") {
			inSection = false
			continue
		}
		if reICMPRuleTarget.MatchString(trimmed) {
			lines[i] = reICMPRuleTarget.ReplaceAllString(trimmed, " -j "+target)
		}
	}
	if err := os.WriteFile(ufwBeforeRulesPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("запись %s: %w", ufwBeforeRulesPath, err)
	}
	if ufwActive() {
		if _, err := runCmd("ufw", "reload"); err != nil {
			return fmt.Errorf("ufw reload: %w", err)
		}
	}
	return nil
}
