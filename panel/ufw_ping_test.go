package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleBeforeRules = `# ok icmp codes for INPUT
-A ufw-before-input -p icmp --icmp-type destination-unreachable -j ACCEPT
-A ufw-before-input -p icmp --icmp-type time-exceeded -j ACCEPT
-A ufw-before-input -p icmp --icmp-type parameter-problem -j ACCEPT
-A ufw-before-input -p icmp --icmp-type echo-request -j ACCEPT

# ok icmp code for FORWARD
-A ufw-before-forward -p icmp --icmp-type destination-unreachable -j ACCEPT
-A ufw-before-forward -p icmp --icmp-type time-exceeded -j ACCEPT
-A ufw-before-forward -p icmp --icmp-type parameter-problem -j ACCEPT
-A ufw-before-forward -p icmp --icmp-type echo-request -j ACCEPT
-A ufw-before-input -p icmp --icmp-type source-quench -j DROP

# allow dhcp client to work
-A ufw-before-input -p udp --sport 67 --dport 68 -j ACCEPT
`

func TestUFWPingBlockedDetectsEchoRequest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "before.rules")
	if err := os.WriteFile(path, []byte(sampleBeforeRules), 0644); err != nil {
		t.Fatal(err)
	}
	if ufwPingBlockedIn(path) {
		t.Fatal("expected ping allowed")
	}
	if err := setUFWBlockPingIn(path, true); err != nil {
		t.Fatal(err)
	}
	if !ufwPingBlockedIn(path) {
		t.Fatal("expected ping blocked after setUFWBlockPing(true)")
	}
	data, _ := os.ReadFile(path)
	body := string(data)
	if strings.Contains(body, "echo-request -j ACCEPT") {
		t.Fatal("echo-request should be DROP when blocked")
	}
	if !strings.Contains(body, "ufw-before-forward -p icmp --icmp-type echo-request -j DROP") {
		t.Fatal("forward echo-request should be DROP")
	}
	// Wrong-chain rule under FORWARD header must stay untouched.
	if !strings.Contains(body, "ufw-before-input -p icmp --icmp-type source-quench -j DROP") {
		t.Fatal("misplaced ufw-before-input rule must not be rewritten in FORWARD section")
	}
}

func TestUFWPingUnblockRestoresAccept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "before.rules")
	if err := os.WriteFile(path, []byte(sampleBeforeRules), 0644); err != nil {
		t.Fatal(err)
	}
	if err := setUFWBlockPingIn(path, true); err != nil {
		t.Fatal(err)
	}
	if err := setUFWBlockPingIn(path, false); err != nil {
		t.Fatal(err)
	}
	if ufwPingBlockedIn(path) {
		t.Fatal("expected ping allowed after unblock")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "echo-request -j ACCEPT") {
		t.Fatal("echo-request should be ACCEPT when unblocked")
	}
}
