package server

import (
	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

func storeFromDatabase(db *Database) *paneldb.Store {
	if db == nil {
		return paneldb.NewStore()
	}
	s := paneldb.NewStore()
	s.MainPassword = db.MainPassword
	s.AdminID = db.AdminID
	s.BotToken = db.BotToken
	for pass, e := range db.Passwords {
		if e == nil {
			continue
		}
		s.Users[pass] = userToPaneldb(e)
	}
	for id, d := range db.Devices {
		if d == nil {
			continue
		}
		s.Devices[id] = &paneldb.Device{
			DeviceID: d.DeviceID,
			IP:       d.IP,
			PrivKey:  d.PrivKey,
			PubKey:   d.PubKey,
		}
	}
	return s
}

func databaseFromStore(s *paneldb.Store) *Database {
	if s == nil {
		return &Database{
			Passwords: make(map[string]*PasswordEntry),
			Devices:   make(map[string]*ClientDevice),
		}
	}
	out := &Database{
		MainPassword: s.MainPassword,
		AdminID:      s.AdminID,
		BotToken:     s.BotToken,
		Passwords:    make(map[string]*PasswordEntry),
		Devices:      make(map[string]*ClientDevice),
	}
	for pass, u := range s.Users {
		if u == nil {
			continue
		}
		out.Passwords[pass] = userFromPaneldb(u)
	}
	for id, d := range s.Devices {
		if d == nil {
			continue
		}
		out.Devices[id] = &ClientDevice{
			DeviceID: d.DeviceID,
			IP:       d.IP,
			PrivKey:  d.PrivKey,
			PubKey:   d.PubKey,
		}
	}
	return out
}

func userToPaneldb(e *PasswordEntry) *paneldb.User {
	return &paneldb.User{
		DeviceID:      e.DeviceID,
		DeviceIDs:     append([]string(nil), e.DeviceIDs...),
		MaxDevices:    e.MaxDevices,
		ExpiresAt:     e.ExpiresAt,
		DownBytes:     e.DownBytes,
		UpBytes:       e.UpBytes,
		TotalBytes:    e.TotalBytes,
		MaxDownMBps:   e.MaxDownMBps,
		MaxUpMBps:     e.MaxUpMBps,
		IsDeactivated: e.IsDeactivated,
		Comment:       e.Comment,
		Ports:         e.Ports,
		VkHash:        e.VkHash,
		SubID:         e.SubID,
		LastSeenAt:    e.LastSeenAt,
	}
}

func userFromPaneldb(u *paneldb.User) *PasswordEntry {
	return &PasswordEntry{
		DeviceID:      u.DeviceID,
		DeviceIDs:     append([]string(nil), u.DeviceIDs...),
		MaxDevices:    u.MaxDevices,
		ExpiresAt:     u.ExpiresAt,
		DownBytes:     u.DownBytes,
		UpBytes:       u.UpBytes,
		TotalBytes:    u.TotalBytes,
		MaxDownMBps:   u.MaxDownMBps,
		MaxUpMBps:     u.MaxUpMBps,
		IsDeactivated: u.IsDeactivated,
		Comment:       u.Comment,
		Ports:         u.Ports,
		VkHash:        u.VkHash,
		SubID:         u.SubID,
		LastSeenAt:    u.LastSeenAt,
	}
}

func syncDeviceFields(entry *PasswordEntry, u *paneldb.User) {
	if entry == nil || u == nil {
		return
	}
	entry.DeviceID = u.DeviceID
	entry.DeviceIDs = append([]string(nil), u.DeviceIDs...)
	entry.MaxDevices = u.MaxDevices
}

func normalizeEntryDevices(entry *PasswordEntry) {
	if entry == nil {
		return
	}
	u := userToPaneldb(entry)
	paneldb.NormalizeUser(u)
	syncDeviceFields(entry, u)
}

func allEntryDeviceIDs(entry *PasswordEntry) []string {
	if entry == nil {
		return nil
	}
	u := userToPaneldb(entry)
	ids := paneldb.AllDeviceIDs(u)
	syncDeviceFields(entry, u)
	return ids
}
