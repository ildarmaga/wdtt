package main

import "strings"

const (
	defaultMaxDevices = 1
	maxDevicesLimit   = 20
)

func normalizeEntryDevices(entry *PasswordEntry) {
	if entry == nil {
		return
	}
	if entry.DeviceID != "" {
		found := false
		for _, id := range entry.DeviceIDs {
			if id == entry.DeviceID {
				found = true
				break
			}
		}
		if !found {
			entry.DeviceIDs = append([]string{entry.DeviceID}, entry.DeviceIDs...)
		}
	}
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(entry.DeviceIDs))
	for _, id := range entry.DeviceIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		cleaned = append(cleaned, id)
	}
	entry.DeviceIDs = cleaned
	if len(entry.DeviceIDs) > 0 {
		entry.DeviceID = entry.DeviceIDs[0]
	} else {
		entry.DeviceID = ""
	}
	if entry.MaxDevices <= 0 {
		entry.MaxDevices = defaultMaxDevices
	}
	if entry.MaxDevices > maxDevicesLimit {
		entry.MaxDevices = maxDevicesLimit
	}
}

func entryMaxDevices(entry *PasswordEntry) int {
	if entry == nil || entry.MaxDevices <= 0 {
		return defaultMaxDevices
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

func allEntryDeviceIDsPanel(entry *PasswordEntry) []string {
	if entry == nil {
		return nil
	}
	normalizeEntryDevices(entry)
	return append([]string(nil), entry.DeviceIDs...)
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
