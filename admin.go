package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"golang.zx2c4.com/wireguard/device"
)

var (
	adminListenAddr  = "127.0.0.1:2861"
	maxDTLSPerDevice int32 = 0

	dtlsPerDeviceMutex  sync.Mutex
	dtlsPerDeviceCounts = map[string]*int32{}
)

func dtlsSlotAcquire(deviceID string) bool {
	if deviceID == "" || deviceID == "unknown" {
		return true
	}
	dtlsPerDeviceMutex.Lock()
	defer dtlsPerDeviceMutex.Unlock()
	counter, ok := dtlsPerDeviceCounts[deviceID]
	if !ok {
		counter = new(int32)
		dtlsPerDeviceCounts[deviceID] = counter
	}
	if atomic.LoadInt32(counter) >= maxDTLSPerDevice {
		return false
	}
	atomic.AddInt32(counter, 1)
	return true
}

func dtlsSlotRelease(deviceID string) {
	if deviceID == "" || deviceID == "unknown" {
		return
	}
	dtlsPerDeviceMutex.Lock()
	counter, ok := dtlsPerDeviceCounts[deviceID]
	dtlsPerDeviceMutex.Unlock()
	if !ok {
		return
	}
	if atomic.AddInt32(counter, -1) <= 0 {
		dtlsPerDeviceMutex.Lock()
		if atomic.LoadInt32(counter) <= 0 {
			delete(dtlsPerDeviceCounts, deviceID)
		}
		dtlsPerDeviceMutex.Unlock()
	}
}

func reloadDBFromDisk(wgDev *device.Device) error {
	if trafficDirty.Load() {
		dbMutex.Lock()
		saveDB()
		dbMutex.Unlock()
		trafficDirty.Store(false)
	}

	data, err := os.ReadFile(dbFile)
	if err != nil {
		return err
	}
	var incoming Database
	if err := json.Unmarshal(data, &incoming); err != nil {
		return err
	}

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
	dbMutex.Unlock()

	loadInboundSettings(filepath.Dir(dbFile))
	syncAllSpeedLimits()
	log.Printf("[ADMIN] Конфиг перезагружен из %s", dbFile)
	return nil
}

func startAdminServer(ctx context.Context, wgDev *device.Device) {
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
