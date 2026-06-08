package main

import (
	"os/exec"
	"strings"
)

func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func serviceActive(name string) bool {
	out, _ := runCmd("systemctl", "is-active", name)
	return out == "active"
}

func serviceRestart(name string) error {
	_, err := runCmd("systemctl", "restart", name)
	return err
}

func serviceStop(name string) error {
	_, err := runCmd("systemctl", "stop", name)
	return err
}

func serviceStart(name string) error {
	_, err := runCmd("systemctl", "start", name)
	return err
}
