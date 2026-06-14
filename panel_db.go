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

	"github.com/ildarmaga/wdtt/pkg/paneldb"
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
		db, err := paneldb.Open(panelDBPath)
		if err != nil {
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

func loadDatabaseFromSQLite() (*Database, bool, error) {
	db, err := openServerPanelDB()
	if err != nil {
		return nil, false, err
	}
	ok, err := paneldb.HasUsers(db)
	if err != nil || !ok {
		return nil, false, err
	}
	s, err := paneldb.LoadStore(db)
	if err != nil {
		return nil, false, err
	}
	out := databaseFromStore(s)
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
	return paneldb.SaveStore(db, storeFromDatabase(src), paneldb.SaveOptions{PreserveSubIDs: true})
}

func updateLastSeenInSQLite(password string, ts int64) error {
	db, err := openServerPanelDB()
	if err != nil {
		return err
	}
	return paneldb.UpdateLastSeen(db, password, ts)
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
	OnlineTimeoutSec    int    `json:"online_timeout_sec"`
	MTU                 int    `json:"mtu"`
}

func loadPanelServicePortsFromSQLite() (panelPort, subPort int, ok bool, err error) {
	db, err := openServerPanelDB()
	if err != nil {
		return 0, 0, false, err
	}
	var p, s sql.NullInt64
	err = db.QueryRow(`SELECT port, sub_port FROM panel_config WHERE id = 1`).Scan(&p, &s)
	if err == sql.ErrNoRows {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	panelPort = defaultPanelTCPPort
	subPort = defaultSubTCPPort
	if p.Valid && p.Int64 > 0 {
		panelPort = int(p.Int64)
	}
	if s.Valid && s.Int64 > 0 {
		subPort = int(s.Int64)
	}
	return panelPort, subPort, true, nil
}

func loadInboundFromSQLite() (inboundRuntimeSettings, bool, error) {
	db, err := openServerPanelDB()
	if err != nil {
		return inboundRuntimeSettings{}, false, err
	}
	var n int
	err = db.QueryRow(`SELECT COUNT(*) FROM wdtt_inbound`).Scan(&n)
	if err != nil || n == 0 {
		return inboundRuntimeSettings{}, false, err
	}
	var s inboundRuntimeSettings
	err = db.QueryRow(`SELECT dns, mtu, max_users, handshake_timeout_sec, max_dtls_per_device, online_timeout_sec
		FROM wdtt_inbound WHERE id = 1`).Scan(
		&s.DNS, &s.MTU, &s.MaxUsers, &s.HandshakeTimeoutSec, &s.MaxDtlsPerDevice, &s.OnlineTimeoutSec,
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
	if raw.OnlineTimeoutSec >= 5 && raw.OnlineTimeoutSec <= 600 {
		userOnlineTimeoutSec = raw.OnlineTimeoutSec
	} else {
		userOnlineTimeoutSec = defaultOnlineTimeoutSec
	}
	if raw.MTU >= 576 && raw.MTU <= 1500 {
		wgMTU = raw.MTU
	}
	log.Printf("[CFG] inbound: DTLS timeout=%s, online=%s, max сессий/device=%d (0=без лимита)",
		dtlsHandshakeTimeout, userOnlineTimeoutDuration(), maxDTLSPerDevice)
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
