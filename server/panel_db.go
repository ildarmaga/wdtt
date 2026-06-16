package server

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	_ "modernc.org/sqlite"
)

var panelDBPath = "/etc/wdtt/panel.db"

var (
	serverPanelDB     *sql.DB
	serverPanelDBErr  error
	serverPanelDBOnce sync.Once
	appliedUsersRev   int64
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
	s, err := paneldb.LoadStore(db)
	if err != nil {
		return nil, false, err
	}
	if s.MainPassword == "" && len(s.Users) == 0 {
		return nil, false, nil
	}
	out := databaseFromStore(s)
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

func persistDeviceSQLiteLocked(dev *ClientDevice) error {
	if dev == nil || !serverPanelDBReady() {
		return fmt.Errorf("device or panel.db unavailable")
	}
	sqlDB, err := openServerPanelDB()
	if err != nil {
		return err
	}
	return paneldb.UpsertDevice(sqlDB, &paneldb.Device{
		DeviceID: dev.DeviceID,
		IP:       dev.IP,
		PrivKey:  dev.PrivKey,
		PubKey:   dev.PubKey,
	})
}

func persistUserBindingsSQLiteLocked(password string, entry *PasswordEntry) error {
	if entry == nil || !serverPanelDBReady() {
		return fmt.Errorf("entry or panel.db unavailable")
	}
	sqlDB, err := openServerPanelDB()
	if err != nil {
		return err
	}
	if err := paneldb.PatchUserDeviceBindings(sqlDB, db.MainPassword, password, entry.DeviceIDs, nil); err != nil {
		return err
	}
	rememberUsersRevFromDisk()
	return nil
}

func persistUserEntrySQLiteLocked(password string, entry *PasswordEntry) error {
	if entry == nil || !serverPanelDBReady() {
		return fmt.Errorf("entry or panel.db unavailable")
	}
	sqlDB, err := openServerPanelDB()
	if err != nil {
		return err
	}
	if err := paneldb.UpsertUser(sqlDB, db.MainPassword, password, userToPaneldb(entry)); err != nil {
		return err
	}
	rememberUsersRevFromDisk()
	return nil
}

func persistUserRenameSQLiteLocked(oldPass, newPass string, entry *PasswordEntry) error {
	if entry == nil || !serverPanelDBReady() {
		return fmt.Errorf("entry or panel.db unavailable")
	}
	sqlDB, err := openServerPanelDB()
	if err != nil {
		return err
	}
	if oldPass != newPass {
		if err := paneldb.RenameUserPassword(sqlDB, oldPass, newPass); err != nil {
			return err
		}
	}
	if err := paneldb.UpsertUser(sqlDB, db.MainPassword, newPass, userToPaneldb(entry)); err != nil {
		return err
	}
	rememberUsersRevFromDisk()
	return nil
}

func persistUserDevicesSQLiteLocked(password string, entry *PasswordEntry, removedDeviceIDs []string) error {
	if entry == nil || !serverPanelDBReady() {
		return fmt.Errorf("entry or panel.db unavailable")
	}
	sqlDB, err := openServerPanelDB()
	if err != nil {
		return err
	}
	if err := paneldb.PatchUserDeviceBindings(sqlDB, db.MainPassword, password, entry.DeviceIDs, removedDeviceIDs); err != nil {
		return err
	}
	rememberUsersRevFromDisk()
	return nil
}

func persistUserDeactivatedSQLiteLocked(password string, deactivated bool) error {
	if !serverPanelDBReady() {
		return fmt.Errorf("panel.db unavailable")
	}
	sqlDB, err := openServerPanelDB()
	if err != nil {
		return err
	}
	if err := paneldb.SetUserDeactivated(sqlDB, password, deactivated); err != nil {
		return err
	}
	rememberUsersRevFromDisk()
	return nil
}

func persistDeleteUserSQLiteLocked(password string, deviceIDs []string) error {
	if !serverPanelDBReady() {
		return fmt.Errorf("panel.db unavailable")
	}
	sqlDB, err := openServerPanelDB()
	if err != nil {
		return err
	}
	if err := paneldb.DeleteUser(sqlDB, password, deviceIDs); err != nil {
		return err
	}
	rememberUsersRevFromDisk()
	return nil
}

func persistDeleteDeviceSQLiteLocked(deviceID string) error {
	if !serverPanelDBReady() {
		return fmt.Errorf("panel.db unavailable")
	}
	sqlDB, err := openServerPanelDB()
	if err != nil {
		return err
	}
	if err := paneldb.DeleteDevice(sqlDB, deviceID); err != nil {
		return err
	}
	rememberUsersRevFromDisk()
	return nil
}

func trafficSnapshotLocked() map[string]paneldb.TrafficSnapshot {
	out := make(map[string]paneldb.TrafficSnapshot, len(db.Passwords))
	for pass, e := range db.Passwords {
		if e == nil {
			continue
		}
		out[pass] = paneldb.TrafficSnapshot{
			UpBytes:       e.UpBytes,
			DownBytes:     e.DownBytes,
			LastSeenAt:    e.LastSeenAt,
			IsDeactivated: e.IsDeactivated,
		}
	}
	return out
}

func mergeTrafficIntoDatabase(incoming *Database, from map[string]paneldb.TrafficSnapshot) {
	if incoming == nil || incoming.Passwords == nil || len(from) == 0 {
		return
	}
	users := make(map[string]*paneldb.User, len(incoming.Passwords))
	for pass, e := range incoming.Passwords {
		if e == nil {
			continue
		}
		users[pass] = userToPaneldb(e)
	}
	paneldb.MergeTrafficSnapshots(users, from)
	for pass, u := range users {
		if e := incoming.Passwords[pass]; e != nil && u != nil {
			e.UpBytes = u.UpBytes
			e.DownBytes = u.DownBytes
			e.LastSeenAt = u.LastSeenAt
			e.IsDeactivated = u.IsDeactivated
		}
	}
}

func saveTrafficToSQLiteLocked() error {
	if !serverPanelDBReady() {
		return fmt.Errorf("panel.db not available at %s", panelDBPath)
	}
	sqlDB, err := openServerPanelDB()
	if err != nil {
		return err
	}
	return paneldb.UpdateUsersTraffic(sqlDB, trafficSnapshotLocked())
}

func saveTrafficDB() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	return saveTrafficToSQLiteLocked()
}

func updateLastSeenInSQLite(password string, ts int64) error {
	db, err := openServerPanelDB()
	if err != nil {
		return err
	}
	return paneldb.UpdateLastSeen(db, password, ts)
}

func updateLastSeenBatchInSQLite(updates map[string]int64) error {
	if len(updates) == 0 {
		return nil
	}
	db, err := openServerPanelDB()
	if err != nil {
		return err
	}
	return paneldb.UpdateLastSeenBatch(db, updates)
}

func applyDBGlobalFromCLI(mainPass, adminID, botToken string) {
	if mainPass != "" {
		db.MainPassword = mainPass
	}
	if adminID != "" {
		db.AdminID = adminID
	}
	if botToken != "" {
		db.BotToken = botToken
	}
}

func loadDatabaseFromDiskSource() (*Database, error) {
	if incoming, ok, err := loadDatabaseFromSQLite(); err != nil {
		return nil, err
	} else if ok {
		return incoming, nil
	}
	return &Database{
		Passwords: make(map[string]*PasswordEntry),
		Devices:   make(map[string]*ClientDevice),
	}, nil
}

func saveDatabaseDual() error {
	return saveDB()
}

func rememberUsersRev(rev int64) {
	if rev > appliedUsersRev {
		appliedUsersRev = rev
	}
}

func rememberUsersRevFromDisk() {
	if !serverPanelDBReady() {
		return
	}
	sqlDB, err := openServerPanelDB()
	if err != nil {
		return
	}
	rev, err := paneldb.LoadUsersRev(sqlDB)
	if err == nil {
		rememberUsersRev(rev)
	}
}

// syncPanelDeviceEditsLocked подтягивает пользователей/устройства из panel.db после правок панели.
// Вызывать только при удержанном dbMutex.
func syncPanelDeviceEditsLocked() {
	if !serverPanelDBReady() {
		return
	}
	sqlDB, err := openServerPanelDB()
	if err != nil {
		return
	}
	rev, err := paneldb.LoadUsersRev(sqlDB)
	if err != nil || rev <= appliedUsersRev {
		return
	}
	incoming, ok, err := loadDatabaseFromSQLite()
	if err != nil || !ok {
		return
	}
	mergeTrafficIntoDatabase(incoming, trafficSnapshotLocked())

	for pass := range db.Passwords {
		if _, ok := incoming.Passwords[pass]; !ok {
			delete(db.Passwords, pass)
		}
	}
	for pass, diskEntry := range incoming.Passwords {
		if diskEntry == nil {
			continue
		}
		cp := clonePasswordEntry(diskEntry)
		if pass == db.MainPassword {
			cp.MaxDevices = 0
		}
		db.Passwords[pass] = cp
	}
	oldDevices := db.Devices
	if incoming.Devices != nil {
		next := make(map[string]*ClientDevice, len(incoming.Devices))
		for id, d := range incoming.Devices {
			if d == nil {
				continue
			}
			cp := *d
			next[id] = &cp
		}
		db.Devices = next
	} else {
		db.Devices = make(map[string]*ClientDevice)
	}
	if incoming.MainPassword != "" {
		db.MainPassword = incoming.MainPassword
	}
	migrateDatabaseDevices()
	if err := refreshWrapKeysFromDBLocked(); err != nil {
		log.Printf("[DB] wrap keys sync: %v", err)
		return
	}
	serverWGDevMu.RLock()
	wgDev := serverWGDev
	serverWGDevMu.RUnlock()
	if wgDev != nil {
		for id, dev := range oldDevices {
			if _, ok := db.Devices[id]; !ok {
				removePeerFromWG(wgDev, dev)
			}
		}
		suspendExpiredPasswordsLocked(wgDev)
		for deviceID, dev := range db.Devices {
			pass := passwordForDeviceLocked(deviceID)
			if pass == "" {
				pass = bindOrphanDeviceToMainLocked(deviceID)
			}
			if pass == "" {
				removePeerFromWG(wgDev, dev)
				continue
			}
			entry := db.Passwords[pass]
			if entry == nil || isPasswordExpired(entry) || entry.IsDeactivated || isTrafficExceeded(entry) {
				removePeerFromWG(wgDev, dev)
				continue
			}
			upsertPeerInWG(wgDev, dev)
		}
	}
	rememberUsersRev(rev)
	log.Printf("[DB] синхронизация из panel.db (users_rev=%d)", rev)
}

func clonePasswordEntry(e *PasswordEntry) *PasswordEntry {
	if e == nil {
		return nil
	}
	cp := *e
	cp.DeviceIDs = append([]string(nil), e.DeviceIDs...)
	return &cp
}

type inboundRuntimeSettings = paneldb.RuntimeSettings

func loadPanelServicePortsFromSQLite() (panelPort, subPort int, ok bool, err error) {
	db, err := openServerPanelDB()
	if err != nil {
		return 0, 0, false, err
	}
	return paneldb.LoadPanelServicePorts(db)
}

func loadInboundFromSQLite() (inboundRuntimeSettings, bool, error) {
	db, err := openServerPanelDB()
	if err != nil {
		return inboundRuntimeSettings{}, false, err
	}
	return paneldb.LoadRuntimeSettings(db)
}

func loadStartupFromSQLite() (paneldb.StartupSettings, bool, error) {
	db, err := openServerPanelDB()
	if err != nil {
		return paneldb.StartupSettings{}, false, err
	}
	return paneldb.LoadStartupSettings(db)
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
	if raw.WgKeepaliveSec >= 10 && raw.WgKeepaliveSec <= 120 {
		wgKeepaliveSec = raw.WgKeepaliveSec
	} else {
		wgKeepaliveSec = keepalive
	}
	if raw.StatsIntervalSec >= 2 && raw.StatsIntervalSec <= 60 {
		statsIntervalSec = raw.StatsIntervalSec
	} else {
		statsIntervalSec = defaultStatsIntervalSec
	}
	if raw.MTU >= 576 && raw.MTU <= 1500 {
		wgMTU = raw.MTU
	}
	log.Printf("[CFG] inbound: DTLS timeout=%s, online=%s, WG keepalive=%ds, stats=%ds, max сессий/device=%d (0=без лимита)",
		dtlsHandshakeTimeout, userOnlineTimeoutDuration(), wgKeepaliveSec, statsIntervalSec, maxDTLSPerDevice)
	log.Printf("[CFG] DNS клиентов: %s, MTU: %d, лимит активных: %d", clientDNS, wgMTU, maxGeneratedPasswords)
}

func loadInboundSettings() {
	if raw, ok, err := loadInboundFromSQLite(); err != nil {
		log.Printf("[DB] inbound sqlite load: %v", err)
	} else if ok {
		applyInboundRuntimeSettings(raw)
		log.Printf("[CFG] inbound loaded from %s", panelDBPath)
		return
	}
	log.Printf("[CFG] inbound: defaults (empty wdtt_inbound in %s)", panelDBPath)
}
