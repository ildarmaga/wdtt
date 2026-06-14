package paneldb

import "strings"

// NormalizeUser — device_id ↔ device_ids, лимиты слотов.
func NormalizeUser(u *User) {
	if u == nil {
		return
	}
	if u.DeviceID != "" {
		found := false
		for _, id := range u.DeviceIDs {
			if id == u.DeviceID {
				found = true
				break
			}
		}
		if !found {
			u.DeviceIDs = append([]string{u.DeviceID}, u.DeviceIDs...)
		}
	}
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(u.DeviceIDs))
	for _, id := range u.DeviceIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		cleaned = append(cleaned, id)
	}
	u.DeviceIDs = cleaned
	if len(u.DeviceIDs) > 0 {
		u.DeviceID = u.DeviceIDs[0]
	} else {
		u.DeviceID = ""
	}
	if u.MaxDevices <= 0 {
		u.MaxDevices = DefaultMaxDevices
	}
	if u.MaxDevices > MaxDevicesLimit {
		u.MaxDevices = MaxDevicesLimit
	}
}

// AllDeviceIDs — копия device_ids после NormalizeUser.
func AllDeviceIDs(u *User) []string {
	if u == nil {
		return nil
	}
	NormalizeUser(u)
	return append([]string(nil), u.DeviceIDs...)
}
