package server

import (
	"errors"
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	"golang.zx2c4.com/wireguard/device"
)

var (
	adminListenAddr   = "127.0.0.1:2861"
	maxDTLSPerDevice  int32 = 0
	adminReloadSecret atomic.Value // string
	adminReloadMu     sync.Mutex
)

func setAdminReloadSecret(key string) {
	adminReloadSecret.Store(strings.TrimSpace(key))
}

func adminReloadAuthorized(r *http.Request) bool {
	secret, _ := adminReloadSecret.Load().(string)
	if secret == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		return host == "127.0.0.1" || host == "::1"
	}
	got := strings.TrimSpace(r.Header.Get("X-WDTT-Admin-Token"))
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}

func loadAdminReloadSecretFromDB() {
	if !serverPanelDBReady() {
		return
	}
	db, err := openServerPanelDB()
	if err != nil {
		log.Printf("[ADMIN] session_key load: %v", err)
		return
	}
	key, err := paneldb.LoadPanelSessionKey(db)
	if err != nil {
		log.Printf("[ADMIN] session_key load: %v", err)
		return
	}
	setAdminReloadSecret(key)
}

func reloadDBFromDisk(wgDev *device.Device) error {
	diskRev := panelUsersRevOnDisk()
	trustDisk := trustPanelTrafficCounters(diskRev)

	var memTraffic map[string]paneldb.TrafficSnapshot
	if !trustDisk {
		dbMutex.Lock()
		memTraffic = trafficSnapshotLocked()
		dbMutex.Unlock()
	}

	data, err := loadDatabaseFromDiskSource()
	if err != nil {
		return err
	}
	incoming := *data
	if !trustDisk {
		mergeTrafficIntoDatabase(&incoming, memTraffic)
	}

	dbMutex.Lock()
	for id, dev := range db.Devices {
		if incoming.Devices == nil {
			if wgDev != nil {
				removePeerFromWG(wgDev, dev)
			}
			cancelRawSessionsForDevice(id)
			continue
		}
		if _, ok := incoming.Devices[id]; !ok {
			if wgDev != nil {
				removePeerFromWG(wgDev, dev)
			}
			cancelRawSessionsForDevice(id)
		}
	}
	if incoming.Passwords != nil {
		db.Passwords = incoming.Passwords
	}
	if incoming.Devices != nil {
		db.Devices = incoming.Devices
	}
	if incoming.MainPassword != "" {
		db.MainPassword = incoming.MainPassword
	}
	db.AdminID = incoming.AdminID
	db.BotToken = incoming.BotToken
	migrateDatabaseDevices()
	if err := refreshWrapKeysFromDBLocked(); err != nil {
		dbMutex.Unlock()
		return err
	}
	suspendExpiredPasswordsLocked(wgDev)
	if wgDev != nil {
		for deviceID, dev := range db.Devices {
			pass := passwordForDeviceLocked(deviceID)
			if pass == "" {
				pass = bindOrphanDeviceToMainLocked(deviceID)
			}
			if pass == "" {
				removePeerFromWG(wgDev, dev)
				cancelRawSessionsForDevice(deviceID)
				continue
			}
			entry := db.Passwords[pass]
			if entry == nil || isPasswordExpired(entry) || entry.IsDeactivated || isTrafficExceeded(entry) {
				removePeerFromWG(wgDev, dev)
				cancelRawSessionsForDevice(deviceID)
				cancelRawSessionsForPassword(pass)
				continue
			}
			upsertPeerInWG(wgDev, dev)
		}
	} else {
		for deviceID := range db.Devices {
			pass := passwordForDeviceLocked(deviceID)
			if pass == "" {
				cancelRawSessionsForDevice(deviceID)
				continue
			}
			entry := db.Passwords[pass]
			if entry == nil || isPasswordExpired(entry) || entry.IsDeactivated || isTrafficExceeded(entry) {
				cancelRawSessionsForDevice(deviceID)
				cancelRawSessionsForPassword(pass)
			}
		}
	}
	if trafficDirty.Load() {
		if !trustDisk {
			if err := saveTrafficToSQLiteLocked(); err != nil {
				if !errors.Is(err, errTrafficFlushFenced) {
					dbMutex.Unlock()
					return err
				}
				// Fenced: keep dirty until a later flush after appliedUsersRev catches up.
			} else {
				trafficDirty.Store(false)
			}
		} else {
			trafficDirty.Store(false)
		}
	}
	dbMutex.Unlock()

	loadInboundSettings()
	syncAllSpeedLimits()
	syncVPNLocalServices(wgIfaceName)
	loadAdminReloadSecretFromDB()
	if diskRev > 0 {
		rememberUsersRev(diskRev)
	} else {
		rememberUsersRevFromDisk()
	}
	log.Printf("[ADMIN] Конфиг перезагружен из %s", panelDBPath)
	return nil
}

func startAdminServer(ctx context.Context, wgDev *device.Device) {
	loadAdminReloadSecretFromDB()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":         true,
			"vpn_active": serverVPNActive.Load(),
			"uptime_sec": serverUptimeSeconds(),
		})
	})
	mux.HandleFunc("/admin/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if !adminReloadAuthorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		adminReloadMu.Lock()
		defer adminReloadMu.Unlock()
		if err := reloadDBFromDisk(wgDev); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/admin/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if !adminReloadAuthorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		adminReloadMu.Lock()
		if wgDev != nil {
			if err := reloadDBFromDisk(wgDev); err != nil {
				adminReloadMu.Unlock()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			loadAdminReloadSecretFromDB()
		}
		adminReloadMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"restarting":true}`))
		go notifyRestartRequested()
	})

	registerStoreAdminRoutes(mux, wgDev)

	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", adminListenAddr)
	if err != nil {
		log.Printf("[ADMIN] HTTP не запущен (%s): %v", adminListenAddr, err)
		return
	}
	log.Printf("[ADMIN] HTTP %s (/health, POST /admin/reload, POST /admin/restart, /admin/users/update, /admin/users/delete)", adminListenAddr)
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[ADMIN] HTTP: %v", err)
		}
	}()
}
