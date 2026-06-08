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

func migrateDatabaseDevices() {
	for _, entry := range db.Passwords {
		normalizeEntryDevices(entry)
	}
}

func entryMaxDevices(entry *PasswordEntry) int {
	if entry == nil || entry.MaxDevices <= 0 {
		return defaultMaxDevices
	}
	return entry.MaxDevices
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

func entryDeviceSlotsLeft(entry *PasswordEntry) int {
	if entry == nil {
		return 0
	}
	normalizeEntryDevices(entry)
	left := entryMaxDevices(entry) - len(entry.DeviceIDs)
	if left < 0 {
		return 0
	}
	return left
}

func entryCanAcceptDevice(entry *PasswordEntry, deviceID string) bool {
	if entryHasDevice(entry, deviceID) {
		return true
	}
	return entryDeviceSlotsLeft(entry) > 0
}

func bindDeviceToEntry(entry *PasswordEntry, deviceID string) bool {
	if entry == nil || deviceID == "" {
		return false
	}
	normalizeEntryDevices(entry)
	if entryHasDevice(entry, deviceID) {
		return true
	}
	if entryDeviceSlotsLeft(entry) <= 0 {
		return false
	}
	entry.DeviceIDs = append(entry.DeviceIDs, deviceID)
	entry.DeviceID = entry.DeviceIDs[0]
	return true
}

func removeDeviceFromEntry(entry *PasswordEntry, deviceID string) bool {
	if entry == nil || deviceID == "" {
		return false
	}
	normalizeEntryDevices(entry)
	out := entry.DeviceIDs[:0]
	removed := false
	for _, id := range entry.DeviceIDs {
		if id == deviceID {
			removed = true
			continue
		}
		out = append(out, id)
	}
	if !removed {
		return false
	}
	entry.DeviceIDs = out
	if len(entry.DeviceIDs) > 0 {
		entry.DeviceID = entry.DeviceIDs[0]
	} else {
		entry.DeviceID = ""
	}
	return true
}

func clearEntryDevices(entry *PasswordEntry) {
	if entry == nil {
		return
	}
	entry.DeviceIDs = nil
	entry.DeviceID = ""
}

func allEntryDeviceIDs(entry *PasswordEntry) []string {
	if entry == nil {
		return nil
	}
	normalizeEntryDevices(entry)
	return append([]string(nil), entry.DeviceIDs...)
}
