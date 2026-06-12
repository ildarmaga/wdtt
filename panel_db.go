package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const panelDBPath = "/etc/wdtt/panel.db"

var (
	serverPanelDB     *sql.DB
	serverPanelDBErr  error
	serverPanelDBOnce sync.Once
)

func openServerPanelDB() (*sql.DB, error) {
	serverPanelDBOnce.Do(func() {
		if _, err := os.Stat(panelDBPath); err != nil {
			serverPanelDBErr = err
			return
		}
		dsn := panelDBPath + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			serverPanelDBErr = err
			return
		}
		db.SetMaxOpenConns(1)
		if err := db.Ping(); err != nil {
			db.Close()
			serverPanelDBErr = err
			return
		}
		if err := ensureServerDBSchema(db); err != nil {
			db.Close()
			serverPanelDBErr = err
			return
		}
		serverPanelDB = db
	})
	return serverPanelDB, serverPanelDBErr
}

func serverPanelDBReady() bool {
	db, err := openServerPanelDB()
	return err == nil && db != nil
}

func ensureServerDBSchema(db *sql.DB) error {
	for _, stmt := range []string{
		`ALTER TABLE wdtt_global ADD COLUMN admin_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE wdtt_global ADD COLUMN bot_token TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE wdtt_users ADD COLUMN sub_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE wdtt_users ADD COLUMN last_seen_at INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}

func sqliteTableCount(db *sql.DB, table string) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
	return n, err
}

func loadDatabaseFromSQLite() (*Database, bool, error) {
	db, err := openServerPanelDB()
	if err != nil {
		return nil, false, err
	}
	n, err := sqliteTableCount(db, "wdtt_users")
	if err != nil || n == 0 {
		return nil, false, err
	}

	out := &Database{
		Passwords: make(map[string]*PasswordEntry),
		Devices:   make(map[string]*ClientDevice),
	}
	_ = db.QueryRow(`SELECT main_password, admin_id, bot_token FROM wdtt_global WHERE id = 1`).Scan(
		&out.MainPassword, &out.AdminID, &out.BotToken,
	)
	rows, err := db.Query(`SELECT password, device_id, max_devices, expires_at, down_bytes, up_bytes,
		total_bytes, max_down_mbps, max_up_mbps, is_deactivated, comment, ports, vk_hash, sub_id, last_seen_at FROM wdtt_users`)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		e := &PasswordEntry{}
		var pass string
		var deactivated int
		if err := rows.Scan(&pass, &e.DeviceID, &e.MaxDevices, &e.ExpiresAt, &e.DownBytes, &e.UpBytes,
			&e.TotalBytes, &e.MaxDownMBps, &e.MaxUpMBps, &deactivated, &e.Comment, &e.Ports, &e.VkHash, &e.SubID, &e.LastSeenAt); err != nil {
			return nil, false, err
		}
		e.IsDeactivated = deactivated != 0
		out.Passwords[pass] = e
	}
	drows, err := db.Query(`SELECT password, device_id FROM wdtt_user_devices ORDER BY password, sort_order`)
	if err != nil {
		return nil, false, err
	}
	defer drows.Close()
	for drows.Next() {
		var pass, did string
		if err := drows.Scan(&pass, &did); err != nil {
			return nil, false, err
		}
		if e := out.Passwords[pass]; e != nil {
			e.DeviceIDs = append(e.DeviceIDs, did)
		}
	}
	devRows, err := db.Query(`SELECT device_id, ip, priv_key, pub_key FROM wdtt_devices`)
	if err != nil {
		return nil, false, err
	}
	defer devRows.Close()
	for devRows.Next() {
		d := &ClientDevice{}
		if err := devRows.Scan(&d.DeviceID, &d.IP, &d.PrivKey, &d.PubKey); err != nil {
			return nil, false, err
		}
		out.Devices[d.DeviceID] = d
	}
	for _, e := range out.Passwords {
		normalizeEntryDevices(e)
	}
	return out, true, nil
}

func saveDatabaseToSQLite(src *Database) error {
	if src == nil {
		return fmt.Errorf("nil database")
	}
	db, err := openServerPanelDB()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO wdtt_global (id, main_password, admin_id, bot_token) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			main_password=excluded.main_password,
			admin_id=excluded.admin_id,
			bot_token=excluded.bot_token`,
		src.MainPassword, src.AdminID, src.BotToken); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wdtt_user_devices`); err != nil {
		return err
	}
	existingSubIDs := map[string]string{}
	subRows, err := tx.Query(`SELECT password, sub_id FROM wdtt_users WHERE sub_id != ''`)
	if err != nil {
		return err
	}
	for subRows.Next() {
		var pass, sid string
		if err := subRows.Scan(&pass, &sid); err != nil {
			subRows.Close()
			return err
		}
		existingSubIDs[pass] = sid
	}
	if err := subRows.Close(); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wdtt_users`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wdtt_devices`); err != nil {
		return err
	}
	for pass, entry := range src.Passwords {
		if entry == nil {
			continue
		}
		normalizeEntryDevices(entry)
		deact := 0
		if entry.IsDeactivated {
			deact = 1
		}
		subID := strings.TrimSpace(entry.SubID)
		if subID == "" {
			subID = existingSubIDs[pass]
		}
		if _, err := tx.Exec(`INSERT INTO wdtt_users (
			password, device_id, max_devices, expires_at, down_bytes, up_bytes, total_bytes,
			max_down_mbps, max_up_mbps, is_deactivated, comment, ports, vk_hash, sub_id, last_seen_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			pass, entry.DeviceID, entry.MaxDevices, entry.ExpiresAt, entry.DownBytes, entry.UpBytes,
			entry.TotalBytes, entry.MaxDownMBps, entry.MaxUpMBps, deact, entry.Comment, entry.Ports, entry.VkHash, subID, entry.LastSeenAt,
		); err != nil {
			return err
		}
		for i, did := range allEntryDeviceIDs(entry) {
			if _, err := tx.Exec(`INSERT INTO wdtt_user_devices (password, device_id, sort_order) VALUES (?,?,?)`,
				pass, did, i); err != nil {
				return err
			}
		}
	}
	for id, dev := range src.Devices {
		if dev == nil {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO wdtt_devices (device_id, ip, priv_key, pub_key) VALUES (?,?,?,?)`,
			id, dev.IP, dev.PrivKey, dev.PubKey); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func updateLastSeenInSQLite(password string, ts int64) error {
	if password == "" || ts <= 0 {
		return nil
	}
	db, err := openServerPanelDB()
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE wdtt_users SET last_seen_at = ? WHERE password = ? AND last_seen_at < ?`, ts, password, ts)
	return err
}

func loadDatabaseFromDiskSource() (*Database, error) {
	if incoming, ok, err := loadDatabaseFromSQLite(); err != nil {
		return nil, err
	} else if ok {
		return incoming, nil
	}
	// Одноразовый fallback: panel.db ещё пуст, passwords.json остался до миграции панели.
	if data, err := os.ReadFile(dbFile); err == nil {
		var incoming Database
		if err := json.Unmarshal(data, &incoming); err != nil {
			return nil, err
		}
		if incoming.Passwords == nil {
			incoming.Passwords = make(map[string]*PasswordEntry)
		}
		if incoming.Devices == nil {
			incoming.Devices = make(map[string]*ClientDevice)
		}
		if serverPanelDBReady() {
			if err := saveDatabaseToSQLite(&incoming); err != nil {
				log.Printf("[DB] migrate passwords.json to sqlite: %v", err)
			} else {
				_ = os.Remove(dbFile)
				log.Printf("[DB] migrated passwords.json → %s", panelDBPath)
			}
		}
		return &incoming, nil
	}
	return &Database{
		Passwords: make(map[string]*PasswordEntry),
		Devices:   make(map[string]*ClientDevice),
	}, nil
}

func saveDatabaseDual() error {
	if !serverPanelDBReady() {
		return fmt.Errorf("panel.db not available at %s", panelDBPath)
	}
	return saveDatabaseToSQLite(db)
}

type inboundRuntimeSettings struct {
	DNS                 string `json:"dns"`
	MaxUsers            int    `json:"max_users"`
	HandshakeTimeoutSec int    `json:"handshake_timeout_sec"`
	MaxDtlsPerDevice    int    `json:"max_dtls_per_device"`
	MTU                 int    `json:"mtu"`
}

func loadInboundFromSQLite() (inboundRuntimeSettings, bool, error) {
	db, err := openServerPanelDB()
	if err != nil {
		return inboundRuntimeSettings{}, false, err
	}
	n, err := sqliteTableCount(db, "wdtt_inbound")
	if err != nil || n == 0 {
		return inboundRuntimeSettings{}, false, err
	}
	var s inboundRuntimeSettings
	err = db.QueryRow(`SELECT dns, mtu, max_users, handshake_timeout_sec, max_dtls_per_device
		FROM wdtt_inbound WHERE id = 1`).Scan(
		&s.DNS, &s.MTU, &s.MaxUsers, &s.HandshakeTimeoutSec, &s.MaxDtlsPerDevice,
	)
	if err == sql.ErrNoRows {
		return inboundRuntimeSettings{}, false, nil
	}
	if err != nil {
		return inboundRuntimeSettings{}, false, err
	}
	return s, true, nil
}

func loadInboundFromJSONFile(configDir string) (inboundRuntimeSettings, error) {
	data, err := os.ReadFile(filepath.Join(configDir, "inbound.json"))
	if err != nil {
		return inboundRuntimeSettings{}, err
	}
	var raw inboundRuntimeSettings
	if err := json.Unmarshal(data, &raw); err != nil {
		return inboundRuntimeSettings{}, err
	}
	return raw, nil
}

func applyInboundRuntimeSettings(raw inboundRuntimeSettings) {
	clientDNS = defaultClientDNS
	maxGeneratedPasswords = defaultMaxUsers
	dtlsHandshakeTimeout = 30 * time.Second
	maxDTLSPerDevice = 0
	wgMTU = defaultWgMTU

	if dns := strings.TrimSpace(raw.DNS); dns != "" {
		clientDNS = dns
	}
	if raw.MaxUsers >= 1 && raw.MaxUsers <= maxUsersSubnetLimit {
		maxGeneratedPasswords = raw.MaxUsers
	}
	if raw.HandshakeTimeoutSec >= 5 && raw.HandshakeTimeoutSec <= 600 {
		dtlsHandshakeTimeout = time.Duration(raw.HandshakeTimeoutSec) * time.Second
	}
	if raw.MaxDtlsPerDevice >= 0 && raw.MaxDtlsPerDevice <= 50 {
		maxDTLSPerDevice = int32(raw.MaxDtlsPerDevice)
	}
	if raw.MTU >= 576 && raw.MTU <= 1500 {
		wgMTU = raw.MTU
	}
	log.Printf("[CFG] inbound: DTLS timeout=%s, max сессий/device=%d (0=без лимита)", dtlsHandshakeTimeout, maxDTLSPerDevice)
	log.Printf("[CFG] DNS клиентов: %s, MTU: %d, лимит активных: %d", clientDNS, wgMTU, maxGeneratedPasswords)
}

func loadInboundSettings(configDir string) {
	if raw, ok, err := loadInboundFromSQLite(); err != nil {
		log.Printf("[DB] inbound sqlite load: %v", err)
	} else if ok {
		applyInboundRuntimeSettings(raw)
		log.Printf("[CFG] inbound loaded from %s", panelDBPath)
		return
	}
	// Legacy fallback до миграции панели (v5).
	raw, err := loadInboundFromJSONFile(configDir)
	if err != nil {
		log.Printf("[CFG] inbound: defaults (empty wdtt_inbound)")
		return
	}
	applyInboundRuntimeSettings(raw)
	if serverPanelDBReady() {
		log.Printf("[CFG] inbound loaded from %s/inbound.json (migrate via panel)", configDir)
	}
}
