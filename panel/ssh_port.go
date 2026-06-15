package panel

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const sshServiceUnit = "ssh.service"

type sshPortChangeResult struct {
	OldPort  int    `json:"old_port"`
	SSHPort  int    `json:"ssh_port"`
	UFWOpen  bool   `json:"ufw_open"`
	Message  string `json:"message"`
}

func setSSHPort(newPort int) (*sshPortChangeResult, error) {
	if err := validateFirewallPort(newPort); err != nil {
		return nil, err
	}
	oldPort := detectSSHPort()
	if newPort == oldPort {
		return nil, fmt.Errorf("SSH уже использует порт %d", newPort)
	}

	mainBackup, err := os.ReadFile(sshConfigPath)
	if err != nil {
		return nil, fmt.Errorf("чтение %s: %w", sshConfigPath, err)
	}
	dropinBackups, err := backupSSHDropins()
	if err != nil {
		return nil, err
	}
	rollback := func() {
		_ = os.WriteFile(sshConfigPath, mainBackup, 0644)
		for path, data := range dropinBackups {
			_ = os.WriteFile(path, data, 0644)
		}
	}

	if err := writeSSHPort(newPort); err != nil {
		return nil, err
	}
	if _, err := runCmd("sshd", "-t"); err != nil {
		rollback()
		return nil, fmt.Errorf("проверка sshd_config: %w", err)
	}

	ufwOpened := false
	if ufwActive() {
		rule := fmt.Sprintf("%d/tcp", newPort)
		if _, err := runCmd("ufw", "allow", rule, "comment", sshUFWComment); err != nil {
			rollback()
			return nil, fmt.Errorf("ufw allow %s: %w", rule, err)
		}
		ufwOpened = true
	}

	if err := serviceRestart(sshServiceUnit); err != nil {
		if ufwOpened {
			_, _ = runCmd("ufw", "delete", "allow", fmt.Sprintf("%d/tcp", newPort))
		}
		rollback()
		return nil, fmt.Errorf("перезапуск SSH: %w", err)
	}

	if ufwActive() && oldPort != newPort {
		_ = firewallPortClose("tcp", oldPort)
	}

	msg := fmt.Sprintf("SSH порт изменён: %d → %d", oldPort, newPort)
	if ufwOpened {
		msg += fmt.Sprintf(". UFW: открыт %d/tcp", newPort)
	}
	return &sshPortChangeResult{
		OldPort: oldPort,
		SSHPort: newPort,
		UFWOpen: ufwOpened,
		Message: msg,
	}, nil
}

func backupSSHDropins() (map[string][]byte, error) {
	backups := map[string][]byte{}
	dir := "/etc/ssh/sshd_config.d"
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return backups, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		backups[path] = data
	}
	return backups, nil
}

func writeSSHPort(newPort int) error {
	dir := "/etc/ssh/sshd_config.d"
	if entries, err := os.ReadDir(dir); err == nil {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
				continue
			}
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			path := filepath.Join(dir, name)
			if err := commentSSHPortLines(path); err != nil {
				return err
			}
		}
	}
	return setSSHPortInFile(sshConfigPath, newPort)
}

func setSSHPortInFile(path string, newPort int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("чтение %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	portLine := fmt.Sprintf("Port %d", newPort)
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 1 && strings.EqualFold(fields[0], "Port") {
			lines[i] = portLine
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, portLine)
	}
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func commentSSHPortLines(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("чтение %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 1 && strings.EqualFold(fields[0], "Port") {
			lines[i] = "# " + strings.TrimLeft(line, " \t")
			changed = true
		}
	}
	if !changed {
		return nil
	}
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0644)
}
