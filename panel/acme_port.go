package panel

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const acmeFirewallComment = "WDTT_ACME"

type acmePortSession struct {
	port            int
	openedUFW       bool
	stoppedServices []string
}

func ufwTCPPortAllowed(port int) bool {
	if !ufwActive() {
		return true
	}
	out, err := runCmd("ufw", "status", "numbered")
	if err != nil {
		return false
	}
	portStr := strconv.Itoa(port)
	for _, line := range strings.Split(out, "\n") {
		m := reUfwRuleLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil || m[2] != portStr || m[3] != "tcp" {
			continue
		}
		if strings.Contains(strings.ToUpper(line), "ALLOW") {
			return true
		}
	}
	return false
}

func isTCPPortInUse(port int) bool {
	out, _ := runCmd("ss", "-tln")
	portStr := ":" + strconv.Itoa(port)
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, portStr) && strings.Contains(line, "LISTEN") {
			return true
		}
	}
	return false
}

func acmeOpenPort80() (openedByUs bool, err error) {
	if !ufwActive() {
		return false, nil
	}
	if ufwTCPPortAllowed(80) {
		return false, nil
	}
	if err := firewallPortOpen("tcp", 80, acmeFirewallComment); err != nil {
		return false, err
	}
	return true, nil
}

func acmeClosePort80(openedByUs bool) {
	if !openedByUs || !ufwActive() {
		return
	}
	_ = ufwDeleteAllowByComment(80, "tcp", acmeFirewallComment)
	_, _ = runCmd("ufw", "deny", "80/tcp", "comment", acmeFirewallComment)
}

func ufwDeleteAllowByComment(port int, proto, comment string) error {
	out, err := runCmd("ufw", "status", "numbered")
	if err != nil {
		return err
	}
	portStr := strconv.Itoa(port)
	var ids []int
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		m := reUfwRuleLine.FindStringSubmatch(line)
		if m == nil || m[2] != portStr || m[3] != proto {
			continue
		}
		if !strings.Contains(strings.ToUpper(line), "ALLOW") {
			continue
		}
		if c := ufwLineComment(line); comment != "" && c != comment && c != "—" {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	for _, id := range ids {
		_, _ = runCmd("ufw", "--force", "delete", strconv.Itoa(id))
	}
	return nil
}

func freeTCPPort(port int) ([]string, error) {
	if !isTCPPortInUse(port) {
		return nil, nil
	}
	var stopped []string
	for _, unit := range []string{"nginx", "apache2", "caddy"} {
		if !serviceActive(unit) {
			continue
		}
		if err := serviceStop(unit); err != nil {
			continue
		}
		stopped = append(stopped, unit)
		time.Sleep(500 * time.Millisecond)
		if !isTCPPortInUse(port) {
			return stopped, nil
		}
	}
	if serviceActive(xrayServiceUnit) && isTCPPortInUse(port) {
		if err := serviceStop(xrayServiceUnit); err == nil {
			stopped = append(stopped, xrayServiceUnit)
			time.Sleep(500 * time.Millisecond)
		}
	}
	if isTCPPortInUse(port) {
		_, _ = runCmd("fuser", "-k", fmt.Sprintf("%d/tcp", port))
		time.Sleep(800 * time.Millisecond)
	}
	if isTCPPortInUse(port) {
		return stopped, fmt.Errorf("порт %d занят — освободите его вручную (nginx/xray/другой сервис)", port)
	}
	return stopped, nil
}

func restartStoppedServices(units []string) {
	for i := len(units) - 1; i >= 0; i-- {
		_ = serviceStart(units[i])
	}
}

func prepareAcmeHTTPPort(httpPort int) (*acmePortSession, error) {
	if httpPort < 1 || httpPort > 65535 {
		httpPort = 80
	}
	sess := &acmePortSession{port: httpPort}
	opened, err := acmeOpenPort80()
	if err != nil {
		return nil, fmt.Errorf("UFW: не удалось открыть 80/tcp: %w", err)
	}
	sess.openedUFW = opened
	if opened {
		acmeLogWrite("UFW: открыт порт 80/tcp")
	}

	stopped, err := freeTCPPort(httpPort)
	if err != nil {
		sess.cleanup()
		return nil, err
	}
	sess.stoppedServices = stopped
	if len(stopped) > 0 {
		acmeLogWrite("Остановлены сервисы на порту " + strconv.Itoa(httpPort) + ": " + strings.Join(stopped, ", "))
	}
	return sess, nil
}

func (s *acmePortSession) cleanup() {
	restartStoppedServices(s.stoppedServices)
	if len(s.stoppedServices) > 0 {
		acmeLogWrite("Сервисы перезапущены")
	}
	acmeClosePort80(s.openedUFW)
	if s.openedUFW {
		acmeLogWrite("UFW: порт 80/tcp закрыт")
	}
}

func withAcmeHTTPPort(httpPort int, fn func() (map[string]interface{}, error)) (map[string]interface{}, error) {
	sess, err := prepareAcmeHTTPPort(httpPort)
	if err != nil {
		return nil, err
	}
	defer sess.cleanup()
	return fn()
}
