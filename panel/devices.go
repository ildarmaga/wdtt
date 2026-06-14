package main

import (
	"strings"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

func entryMaxDevices(entry *PasswordEntry) int {
	if entry == nil || entry.MaxDevices <= 0 {
		return paneldb.DefaultMaxDevices
	}
	return entry.MaxDevices
}

func deviceIDsDisplay(entry *PasswordEntry) string {
	if entry == nil {
		return ""
	}
	normalizeEntryDevices(entry)
	if len(entry.DeviceIDs) == 0 {
		return ""
	}
	return strings.Join(entry.DeviceIDs, ", ")
}

func entryHasDevice(entry *PasswordEntry, deviceID string) bool {
	if entry == nil || deviceID == "" {
		return false
	}
	normalizeEntryDevices(entry)
	for _, id := range entry.DeviceIDs {
		if id == deviceID {
			return true
		}
	}
	return false
}

func deviceIDsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
