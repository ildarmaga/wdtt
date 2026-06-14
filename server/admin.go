package main

import (
	"context"
	"crypto/subtle"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	"golang.zx2c4.com/wireguard/device"
)

var (
	adminListenAddr   = "127.0.0.1:2861"
	maxDTLSPerDevice  int32 = 0
	adminReloadSecret atomic.Value // string
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
	dbMutex.Lock()
	memTraffic := trafficSnapshotLocked()
	dbMutex.Unlock()

	data, err := loadDatabaseFromDiskSource()
	if err != nil {
		return err
	}
	incoming := *data
	mergeTrafficIntoDatabase(&incoming, memTraffic)

	dbMutex.Lock()
	for id, dev := range db.Devices {
		if incoming.Devices == nil {
			removePeerFromWG(wgDev, dev)
			continue
		}
		if _, ok := incoming.Devices[id]; !ok {
			removePeerFromWG(wgDev, dev)
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
	for deviceID, dev := range db.Devices {
		pass := passwordForDeviceLocked(deviceID)
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
	if trafficDirty.Load() {
		if err := saveTrafficToSQLiteLocked(); err != nil {
			dbMutex.Unlock()
			return err
		}
		trafficDirty.Store(false)
	}
	dbMutex.Unlock()

	loadInboundSettings()
	syncAllSpeedLimits()
	syncVPNLocalServices(wgIfaceName)
	loadAdminReloadSecretFromDB()
	log.Printf("[ADMIN] Конфиг перезагружен из %s", panelDBPath)
	return nil
}

func startAdminServer(ctx context.Context, wgDev *device.Device) {
	loadAdminReloadSecretFromDB()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
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
		if err := reloadDBFromDisk(wgDev); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", adminListenAddr)
	if err != nil {
		log.Printf("[ADMIN] HTTP не запущен (%s): %v", adminListenAddr, err)
		return
	}
	log.Printf("[ADMIN] HTTP %s (/health, POST /admin/reload)", adminListenAddr)
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
