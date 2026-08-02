package server

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

// mbpsToTcRate converts stored MB/s (megabytes/sec) to a tc rate string.
// UI shows Мбит/с and stores value/8 as MB/s.
func mbpsToTcRate(mbPerSec float64) string {
	if mbPerSec <= 0 {
		return "8kbit"
	}
	// MB/s → kbit/s: * 8 (bits) * 1000
	kbit := int(mbPerSec*8000.0 + 0.5)
	if kbit < 8 {
		kbit = 8 // ~1 KB/s floor
	}
	if kbit%1000 == 0 {
		return fmt.Sprintf("%dmbit", kbit/1000)
	}
	return fmt.Sprintf("%dkbit", kbit)
}

func ipLastOctet(ip string) (int, error) {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return 0, fmt.Errorf("invalid ip: %s", ip)
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil || n < 2 || n > 250 {
		return 0, fmt.Errorf("invalid ip octet: %s", ip)
	}
	return n, nil
}

func tcAvailable() bool {
	_, err := runCmd("tc", "-Version")
	return err == nil
}

func resetTcOnIface(iface string) {
	runCmdSilent("tc", "qdisc", "del", "dev", iface, "root")
	runCmdSilent("tc", "qdisc", "del", "dev", iface, "ingress")
}

// tcNeedsHTBReset — true if root is not clean HTB 1: (e.g. fq from BBR default_qdisc).
func tcNeedsHTBReset(qdiscShow string) bool {
	hasHTB := false
	hasOtherRoot := false
	for _, line := range strings.Split(qdiscShow, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, " root ") {
			continue
		}
		if strings.Contains(line, "qdisc htb 1:") {
			hasHTB = true
			continue
		}
		if strings.HasPrefix(line, "qdisc ") {
			hasOtherRoot = true
		}
	}
	return !hasHTB || hasOtherRoot
}

func ensureTcHTB(iface string) {
	out, _ := runCmd("tc", "qdisc", "show", "dev", iface)
	if tcNeedsHTBReset(out) {
		runCmdSilent("tc", "qdisc", "del", "dev", iface, "root")
		if msg := runCmdSilent("tc", "qdisc", "add", "dev", iface, "root", "handle", "1:", "htb", "default", "999"); msg != "" {
			log.Printf("[TC] qdisc htb add %s: %s", iface, msg)
		}
	}
	runCmdSilent("tc", "class", "add", "dev", iface, "parent", "1:", "classid", "1:1", "htb", "rate", "10gbit")
	runCmdSilent("tc", "class", "add", "dev", iface, "parent", "1:1", "classid", "1:999", "htb", "rate", "10gbit", "ceil", "10gbit")
}

func ensureTcIngress(iface string) {
	out, _ := runCmd("tc", "qdisc", "show", "dev", iface)
	if strings.Contains(out, "ingress") {
		return
	}
	if msg := runCmdSilent("tc", "qdisc", "add", "dev", iface, "handle", "ffff:", "ingress"); msg != "" {
		log.Printf("[TC] ingress add %s: %s", iface, msg)
	}
}

func removeClientSpeedLimits(iface, ip string) {
	octet, err := ipLastOctet(ip)
	if err != nil {
		return
	}
	classID := fmt.Sprintf("1:%d", octet)
	runCmdSilent("tc", "filter", "del", "dev", iface, "protocol", "ip", "parent", "1:0", "prio", strconv.Itoa(octet))
	runCmdSilent("tc", "class", "del", "dev", iface, "classid", classID)
	runCmdSilent("tc", "filter", "del", "dev", iface, "protocol", "ip", "parent", "ffff:", "prio", strconv.Itoa(octet+1000))
}

func tcFilterExists(iface, parent string, prio int) bool {
	out, _ := runCmd("tc", "filter", "show", "dev", iface, "parent", parent)
	pref := fmt.Sprintf("pref %d ", prio)
	pref2 := fmt.Sprintf("pref %d\n", prio)
	return strings.Contains(out, pref) || strings.Contains(out, pref2) ||
		strings.Contains(out, fmt.Sprintf("prio %d", prio))
}

// applyClientSpeedLimits updates live HTB rates in place (class replace) so
// Save 1→5 Mbit applies without client reconnect. Filters are kept when possible.
func applyClientSpeedLimits(iface, ip string, downMBps, upMBps float64) {
	if !tcAvailable() {
		log.Printf("[TC] tc не найден — лимит скорости для %s не применён", ip)
		return
	}
	if downMBps <= 0 && upMBps <= 0 {
		removeClientSpeedLimits(iface, ip)
		return
	}
	octet, err := ipLastOctet(ip)
	if err != nil {
		log.Printf("[TC] %v", err)
		return
	}
	if downMBps > 0 {
		ensureTcHTB(iface)
		rate := mbpsToTcRate(downMBps)
		classID := fmt.Sprintf("1:%d", octet)
		// replace = create or update rate without dropping the class/flows
		if out := runCmdSilent("tc", "class", "replace", "dev", iface, "parent", "1:1", "classid", classID,
			"htb", "rate", rate, "ceil", rate); out != "" {
			log.Printf("[TC] class replace %s: %s", ip, out)
		}
		prio := octet
		if !tcFilterExists(iface, "1:", prio) {
			if out := runCmdSilent("tc", "filter", "add", "dev", iface, "protocol", "ip", "parent", "1:0", "prio", strconv.Itoa(prio),
				"u32", "match", "ip", "dst", ip+"/32", "flowid", classID); out != "" {
				log.Printf("[TC] filter dst %s: %s", ip, out)
			}
		}
		log.Printf("[TC] ↓ %s: %.4f MB/s (%s)", ip, downMBps, rate)
	} else {
		// clear download only
		prio := octet
		classID := fmt.Sprintf("1:%d", octet)
		runCmdSilent("tc", "filter", "del", "dev", iface, "protocol", "ip", "parent", "1:0", "prio", strconv.Itoa(prio))
		runCmdSilent("tc", "class", "del", "dev", iface, "classid", classID)
	}
	if upMBps > 0 {
		ensureTcIngress(iface)
		rate := mbpsToTcRate(upMBps)
		prio := octet + 1000
		// police rate cannot be changed in place — recreate filter
		runCmdSilent("tc", "filter", "del", "dev", iface, "protocol", "ip", "parent", "ffff:", "prio", strconv.Itoa(prio))
		if out := runCmdSilent("tc", "filter", "add", "dev", iface, "parent", "ffff:", "protocol", "ip", "prio", strconv.Itoa(prio),
			"u32", "match", "ip", "src", ip+"/32", "police", "rate", rate, "burst", "32kb", "drop"); out != "" {
			log.Printf("[TC] filter src %s: %s", ip, out)
		} else {
			log.Printf("[TC] ↑ %s: %.4f MB/s (%s)", ip, upMBps, rate)
		}
	} else {
		runCmdSilent("tc", "filter", "del", "dev", iface, "protocol", "ip", "parent", "ffff:", "prio", strconv.Itoa(octet+1000))
	}
}

func speedLimitIPsForEntry(entry *PasswordEntry) []string {
	ips := make([]string, 0)
	for _, devID := range allEntryDeviceIDs(entry) {
		dev, ok := db.Devices[devID]
		if ok && dev != nil && dev.IP != "" {
			ips = append(ips, dev.IP)
		}
	}
	return ips
}

func applySpeedLimitForEntryUnlocked(entry *PasswordEntry) {
	if entry == nil {
		return
	}
	for _, ip := range speedLimitIPsForEntry(entry) {
		applyClientSpeedLimits(wgIfaceName, ip, entry.MaxDownMBps, entry.MaxUpMBps)
	}
}

func applySpeedLimitForPassword(password string) {
	if password == "" {
		return
	}
	dbMutex.Lock()
	defer dbMutex.Unlock()
	entry, ok := db.Passwords[password]
	if !ok {
		return
	}
	applySpeedLimitForEntryUnlocked(entry)
}

func syncAllSpeedLimits() {
	if !tcAvailable() {
		log.Printf("[TC] tc не найден — установите iproute2 для лимитов скорости")
		return
	}
	dbMutex.Lock()
	defer dbMutex.Unlock()

	hasLimits := false
	for _, entry := range db.Passwords {
		if entry == nil || entry.IsDeactivated {
			continue
		}
		if entry.MaxDownMBps > 0 || entry.MaxUpMBps > 0 {
			hasLimits = true
			break
		}
	}
	resetTcOnIface(wgIfaceName)
	if !hasLimits {
		return
	}
	for _, entry := range db.Passwords {
		if entry == nil || entry.IsDeactivated {
			continue
		}
		if entry.MaxDownMBps <= 0 && entry.MaxUpMBps <= 0 {
			continue
		}
		for _, ip := range speedLimitIPsForEntry(entry) {
			applyClientSpeedLimits(wgIfaceName, ip, entry.MaxDownMBps, entry.MaxUpMBps)
		}
	}
}
