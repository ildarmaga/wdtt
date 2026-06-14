package main

import (
	"log"
	"net"
	"os/exec"
	"strings"
	"sync"
)

// ==================== Пул буферов ====================

var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 1600)
		return &b
	},
}

func getBuf() *[]byte  { return bufPool.Get().(*[]byte) }
func putBuf(b *[]byte) { bufPool.Put(b) }

// ==================== Оптимизация ====================

func enableBBR() {
	log.Println("[SYS] Оптимизация TCP...")
	out, _ := runCmd("bash", "-c", "sysctl net.ipv4.tcp_congestion_control")
	if strings.Contains(out, "bbr") {
		log.Println("[SYS] BBR уже активен ✓")
		return
	}
	cmds := [][]string{
		{"sysctl", "-w", "net.core.default_qdisc=fq"},
		{"sysctl", "-w", "net.ipv4.tcp_congestion_control=bbr"},
		{"sysctl", "-w", "net.core.rmem_max=25165824"},
		{"sysctl", "-w", "net.core.wmem_max=25165824"},
		{"sysctl", "-w", "net.ipv4.tcp_rmem=4096 87380 25165824"},
		{"sysctl", "-w", "net.ipv4.tcp_wmem=4096 65536 25165824"},
	}
	for _, cmd := range cmds {
		runCmd(cmd[0], cmd[1:]...)
	}
	log.Println("[SYS] BBR включен ✓")
}
func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runCmdSilent(name string, args ...string) string {
	out, _ := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out))
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func isNetTimeout(err error) bool {
	ne, ok := err.(net.Error)
	return ok && ne.Timeout()
}

func getDefaultInterface() string {
	out := runCmdSilent("bash", "-c", "ip route show default | awk '/default/ {print $5}' | head -1")
	if out != "" {
		return strings.TrimSpace(out)
	}
	out = runCmdSilent("bash", "-c", "ip -o link show | awk -F': ' '{print $2}' | grep -v -E 'lo|wg|tun|wdtt' | head -1")
	if out != "" {
		return strings.TrimSpace(out)
	}
	return "eth0"
}
