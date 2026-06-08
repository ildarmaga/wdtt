package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

func mbpsToTcRate(mbps float64) string {
	mbit := int(mbps*8.0 + 0.5)
	if mbit < 1 {
		mbit = 1
	}
	return fmt.Sprintf("%dmbit", mbit)
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

func ensureTcHTB(iface string) {
	out, _ := runCmd("tc", "qdisc", "show", "dev", iface)
	if strings.Contains(out, "htb 1:") {
		return
	}
	runCmdSilent("tc", "qdisc", "add", "dev", iface, "root", "handle", "1:", "htb", "default", "999")
	runCmdSilent("tc", "class", "add", "dev", iface, "parent", "1:", "classid", "1:1", "htb", "rate", "10gbit")
	runCmdSilent("tc", "class", "add", "dev", iface, "parent", "1:1", "classid", "1:999", "htb", "rate", "10gbit", "ceil", "10gbit")
}

func ensureTcIngress(iface string) {
	out, _ := runCmd("tc", "qdisc", "show", "dev", iface)
	if strings.Contains(out, "ingress") {
		return
	}
	runCmdSilent("tc", "qdisc", "add", "dev", iface, "handle", "ffff:", "ingress")
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

func applyClientSpeedLimits(iface, ip string, downMBps, upMBps float64) {
	if !tcAvailable() {
		log.Printf("[TC] tc не найден — лимит скорости для %s не применён", ip)
		return
	}
	removeClientSpeedLimits(iface, ip)
	if downMBps <= 0 && upMBps <= 0 {
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
		if out := runCmdSilent("tc", "class", "add", "dev", iface, "parent", "1:1", "classid", classID, "htb", "rate", rate, "ceil", rate); out != "" {
			log.Printf("[TC] class add %s: %s", ip, out)
		}
		if out := runCmdSilent("tc", "filter", "add", "dev", iface, "protocol", "ip", "parent", "1:0", "prio", strconv.Itoa(octet),
			"u32", "match", "ip", "dst", ip+"/32", "flowid", classID); out != "" {
			log.Printf("[TC] filter dst %s: %s", ip, out)
		} else {
			log.Printf("[TC] ↓ %s: %.1f MB/s (%s)", ip, downMBps, rate)
		}
	}
	if upMBps > 0 {
		ensureTcIngress(iface)
		rate := mbpsToTcRate(upMBps)
		if out := runCmdSilent("tc", "filter", "add", "dev", iface, "parent", "ffff:", "protocol", "ip", "prio", strconv.Itoa(octet+1000),
			"u32", "match", "ip", "src", ip+"/32", "police", "rate", rate, "burst", "1mb", "drop"); out != "" {
			log.Printf("[TC] filter src %s: %s", ip, out)
		} else {
			log.Printf("[TC] ↑ %s: %.1f MB/s (%s)", ip, upMBps, rate)
		}
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
