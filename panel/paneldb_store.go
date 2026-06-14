package main

import (
	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

func storeFromPasswordsDB(db *PasswordsDB) *paneldb.Store {
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
		s.Users[pass] = userEntryToPaneldb(e)
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

func passwordsDBFromStore(s *paneldb.Store) *PasswordsDB {
	if s == nil {
		return &PasswordsDB{
			Passwords: map[string]*PasswordEntry{},
			Devices:   map[string]*DeviceEntry{},
		}
	}
	out := &PasswordsDB{
		MainPassword: s.MainPassword,
		AdminID:      s.AdminID,
		BotToken:     s.BotToken,
		Passwords:    make(map[string]*PasswordEntry),
		Devices:      make(map[string]*DeviceEntry),
	}
	for pass, u := range s.Users {
		if u == nil {
			continue
		}
		out.Passwords[pass] = userEntryFromPaneldb(u)
	}
	for id, d := range s.Devices {
		if d == nil {
			continue
		}
		out.Devices[id] = &DeviceEntry{
			DeviceID: d.DeviceID,
			IP:       d.IP,
			PrivKey:  d.PrivKey,
			PubKey:   d.PubKey,
		}
	}
	return out
}

func userEntryToPaneldb(e *PasswordEntry) *paneldb.User {
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

func userEntryFromPaneldb(u *paneldb.User) *PasswordEntry {
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
	u := userEntryToPaneldb(entry)
	paneldb.NormalizeUser(u)
	syncDeviceFields(entry, u)
}

func allEntryDeviceIDsPanel(entry *PasswordEntry) []string {
	if entry == nil {
		return nil
	}
	u := userEntryToPaneldb(entry)
	ids := paneldb.AllDeviceIDs(u)
	syncDeviceFields(entry, u)
	return ids
}

func wdttInboundToPaneldb(cfg WdttInboundConfig) *paneldb.Inbound {
	return &paneldb.Inbound{
		Tag:                 cfg.Tag,
		Remark:              cfg.Remark,
		Enable:              cfg.Enable,
		ListenHost:          cfg.ListenHost,
		ServerHost:          cfg.ServerHost,
		DtlsPort:            cfg.DtlsPort,
		WgPort:              cfg.WgPort,
		ClientPort:          cfg.ClientPort,
		DNS:                 cfg.DNS,
		MTU:                 cfg.MTU,
		MaxUsers:            cfg.MaxUsers,
		HandshakeTimeoutSec: cfg.HandshakeTimeoutSec,
		MaxDtlsPerDevice:    cfg.MaxDtlsPerDevice,
		OnlineTimeoutSec:    cfg.OnlineTimeoutSec,
		AdminAddr:           cfg.AdminAddr,
	}
}

func wdttInboundFromPaneldb(in *paneldb.Inbound) WdttInboundConfig {
	if in == nil {
		return defaultWdttInbound()
	}
	return WdttInboundConfig{
		Tag:                 in.Tag,
		Remark:              in.Remark,
		Enable:              in.Enable,
		ListenHost:          in.ListenHost,
		ServerHost:          in.ServerHost,
		DtlsPort:            in.DtlsPort,
		WgPort:              in.WgPort,
		ClientPort:          in.ClientPort,
		DNS:                 in.DNS,
		MTU:                 in.MTU,
		MaxUsers:            in.MaxUsers,
		HandshakeTimeoutSec: in.HandshakeTimeoutSec,
		MaxDtlsPerDevice:    in.MaxDtlsPerDevice,
		OnlineTimeoutSec:    in.OnlineTimeoutSec,
		AdminAddr:           in.AdminAddr,
	}
}
