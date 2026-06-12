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
	dedupeDeviceBindings()
}

func dedupeDeviceBindings() {
	type pick struct {
		pass   string
		isMain bool
	}
	chosen := map[string]pick{}
	for pass, entry := range db.Passwords {
		if entry == nil {
			continue
		}
		normalizeEntryDevices(entry)
		isMain := pass == db.MainPassword
		for _, did := range append([]string(nil), entry.DeviceIDs...) {
			if cur, ok := chosen[did]; !ok {
				chosen[did] = pick{pass: pass, isMain: isMain}
			} else if cur.isMain && !isMain {
				chosen[did] = pick{pass: pass, isMain: isMain}
			}
		}
	}
	for pass, entry := range db.Passwords {
		if entry == nil {
			continue
		}
		keep := entry.DeviceIDs[:0]
		for _, did := range entry.DeviceIDs {
			if c, ok := chosen[did]; ok && c.pass == pass {
				keep = append(keep, did)
			}
		}
		entry.DeviceIDs = keep
		if len(keep) > 0 {
			entry.DeviceID = keep[0]
		} else {
			entry.DeviceID = ""
		}
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
	unbindDeviceFromOtherEntries(deviceID, entry)
	entry.DeviceIDs = append(entry.DeviceIDs, deviceID)
	entry.DeviceID = entry.DeviceIDs[0]
	return true
}

func unbindDeviceFromOtherEntries(deviceID string, keep *PasswordEntry) {
	for _, entry := range db.Passwords {
		if entry == nil || entry == keep {
			continue
		}
		removeDeviceFromEntry(entry, deviceID)
	}
	if db.MainPassword != "" {
		if mainEntry, ok := db.Passwords[db.MainPassword]; ok && mainEntry != nil && mainEntry != keep {
			removeDeviceFromEntry(mainEntry, deviceID)
		}
	}
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
