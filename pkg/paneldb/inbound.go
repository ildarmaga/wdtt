package paneldb

import (
	"database/sql"
	"fmt"
)

// Inbound — строка wdtt_inbound (id=1).
type Inbound struct {
	Tag                 string
	Remark              string
	Enable              bool
	ListenHost          string
	ServerHost          string
	DtlsPort            int
	WgPort              int
	ClientPort          int
	DNS                 string
	MTU                 int
	MaxUsers            int
	HandshakeTimeoutSec int
	MaxDtlsPerDevice    int
	OnlineTimeoutSec    int
	AdminAddr           string
}

// RuntimeSettings — поля inbound, которые wdtt-server может применить без rebind.
type RuntimeSettings struct {
	DNS                 string `json:"dns"`
	MTU                 int    `json:"mtu"`
	MaxUsers            int    `json:"max_users"`
	HandshakeTimeoutSec int    `json:"handshake_timeout_sec"`
	MaxDtlsPerDevice    int    `json:"max_dtls_per_device"`
	OnlineTimeoutSec    int    `json:"online_timeout_sec"`
}

// StartupSettings — bind-параметры + runtime (читаются при старте / in-process restart).
type StartupSettings struct {
	ListenHost string
	DtlsPort   int
	WgPort     int
	AdminAddr  string
	Enable     bool
	RuntimeSettings
}

// HasInbound — есть ли строка в wdtt_inbound.
func HasInbound(db *sql.DB) (bool, error) {
	n, err := tableCount(db, "wdtt_inbound")
	return n > 0, err
}

// LoadInbound читает wdtt_inbound WHERE id = 1.
func LoadInbound(db *sql.DB) (*Inbound, error) {
	if db == nil {
		return nil, fmt.Errorf("nil db")
	}
	var in Inbound
	var enable int
	err := db.QueryRow(`SELECT tag, remark, enable, listen_host, server_host, dtls_port, wg_port,
		client_port, dns, mtu, max_users, handshake_timeout_sec, max_dtls_per_device, online_timeout_sec, admin_addr
		FROM wdtt_inbound WHERE id = 1`).Scan(
		&in.Tag, &in.Remark, &enable, &in.ListenHost, &in.ServerHost, &in.DtlsPort, &in.WgPort,
		&in.ClientPort, &in.DNS, &in.MTU, &in.MaxUsers, &in.HandshakeTimeoutSec, &in.MaxDtlsPerDevice, &in.OnlineTimeoutSec, &in.AdminAddr,
	)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	in.Enable = enable != 0
	return &in, nil
}

// SaveInbound upsert wdtt_inbound (id=1).
func SaveInbound(db *sql.DB, in *Inbound) error {
	if db == nil || in == nil {
		return fmt.Errorf("nil db or inbound")
	}
	en := 0
	if in.Enable {
		en = 1
	}
	_, err := db.Exec(`INSERT INTO wdtt_inbound (
		id, tag, remark, enable, listen_host, server_host, dtls_port, wg_port, client_port, dns, mtu,
		max_users, handshake_timeout_sec, max_dtls_per_device, online_timeout_sec, admin_addr
	) VALUES (1,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET
		tag=excluded.tag, remark=excluded.remark, enable=excluded.enable,
		listen_host=excluded.listen_host, server_host=excluded.server_host,
		dtls_port=excluded.dtls_port, wg_port=excluded.wg_port, client_port=excluded.client_port,
		dns=excluded.dns, mtu=excluded.mtu, max_users=excluded.max_users,
		handshake_timeout_sec=excluded.handshake_timeout_sec,
		max_dtls_per_device=excluded.max_dtls_per_device,
		online_timeout_sec=excluded.online_timeout_sec, admin_addr=excluded.admin_addr`,
		in.Tag, in.Remark, en, in.ListenHost, in.ServerHost, in.DtlsPort, in.WgPort,
		in.ClientPort, in.DNS, in.MTU, in.MaxUsers, in.HandshakeTimeoutSec, in.MaxDtlsPerDevice, in.OnlineTimeoutSec, in.AdminAddr,
	)
	return err
}

// LoadRuntimeSettings — runtime-поля для wdtt-server.
func LoadRuntimeSettings(db *sql.DB) (RuntimeSettings, bool, error) {
	if db == nil {
		return RuntimeSettings{}, false, fmt.Errorf("nil db")
	}
	has, err := HasInbound(db)
	if err != nil || !has {
		return RuntimeSettings{}, false, err
	}
	var s RuntimeSettings
	err = db.QueryRow(`SELECT dns, mtu, max_users, handshake_timeout_sec, max_dtls_per_device, online_timeout_sec
		FROM wdtt_inbound WHERE id = 1`).Scan(
		&s.DNS, &s.MTU, &s.MaxUsers, &s.HandshakeTimeoutSec, &s.MaxDtlsPerDevice, &s.OnlineTimeoutSec,
	)
	if err == sql.ErrNoRows {
		return RuntimeSettings{}, false, nil
	}
	if err != nil {
		return RuntimeSettings{}, false, err
	}
	return s, true, nil
}

// LoadStartupSettings — bind + runtime для старта VPN-сервера (из panel.db).
func LoadStartupSettings(db *sql.DB) (StartupSettings, bool, error) {
	if db == nil {
		return StartupSettings{}, false, fmt.Errorf("nil db")
	}
	in, err := LoadInbound(db)
	if err == sql.ErrNoRows {
		return StartupSettings{}, false, nil
	}
	if err != nil {
		return StartupSettings{}, false, err
	}
	return StartupSettings{
		ListenHost: in.ListenHost,
		DtlsPort:   in.DtlsPort,
		WgPort:     in.WgPort,
		AdminAddr:  in.AdminAddr,
		Enable:     in.Enable,
		RuntimeSettings: RuntimeSettings{
			DNS:                 in.DNS,
			MTU:                 in.MTU,
			MaxUsers:            in.MaxUsers,
			HandshakeTimeoutSec: in.HandshakeTimeoutSec,
			MaxDtlsPerDevice:    in.MaxDtlsPerDevice,
			OnlineTimeoutSec:    in.OnlineTimeoutSec,
		},
	}, true, nil
}
