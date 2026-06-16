package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// ==================== Статистика ====================

var (
	totalBytesFromClient int64
	totalBytesToClient   int64
	activeConns          int32 // DTLS-сессии (воркеры)
	activeUsers          int32 // уникальные устройства онлайн
	totalConns           int64
	natType              string = "Инициализация..."
	serverStartTime      time.Time
	serverVPNActive      atomic.Bool
)

func markServerVPNActive(active bool) {
	serverVPNActive.Store(active)
}

func serverUptimeSeconds() uint64 {
	if !serverVPNActive.Load() || serverStartTime.IsZero() {
		return 0
	}
	secs := time.Since(serverStartTime).Seconds()
	if secs < 0 {
		return 0
	}
	return uint64(secs)
}

func resetServerStatsCache(configDir string) {
	statsFile := filepath.Join(configDir, "server.log")
	statsJSON, _ := json.Marshal(map[string]interface{}{
		"active_users":       0,
		"active":             0,
		"sessions":           0,
		"total":              0,
		"online":             []interface{}{},
		"timestamp":          time.Now().Unix(),
		"vpn_active":         true,
		"uptime_sec":         0,
		"stats_interval_sec": statsIntervalSec,
	})
	_ = os.WriteFile(statsFile, statsJSON, 0600)
}

func statsIntervalDuration() time.Duration {
	sec := statsIntervalSec
	if sec < 2 {
		sec = 2
	}
	if sec > 60 {
		sec = 60
	}
	return time.Duration(sec) * time.Second
}

func statsLoop(ctx context.Context, configDir string) {
	if serverStartTime.IsZero() {
		serverStartTime = time.Now()
	}
	statsFile := filepath.Join(configDir, "server.log")
	for {
		interval := statsIntervalDuration()
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		fromC := atomic.LoadInt64(&totalBytesFromClient)
		toC := atomic.LoadInt64(&totalBytesToClient)
		sessions := atomic.LoadInt32(&activeConns)

		peers, peerErr := collectWGPeerInfos()
		if peerErr == nil {
			refreshWGActivityFromPeers(peers)
		}

		dbMutex.Lock()
		syncPanelDeviceEditsLocked()
		dbMutex.Unlock()

		online := snapshotOnlineUsers()
		users := int32(len(online))
		atomic.StoreInt32(&activeUsers, users)
		touchOnlineUsersLastSeenBatch()
		total := atomic.LoadInt64(&totalConns)
		uptime := time.Since(serverStartTime)

		dbMutex.Lock()
		numPasswords := len(db.Passwords)
		numDevices := len(db.Devices)
		dbMutex.Unlock()

		uptimeStr := formatUptime(uptime)
		downGB := float64(toC) / (1024 * 1024 * 1024)
		upGB := float64(fromC) / (1024 * 1024 * 1024)
		intervalSec := statsIntervalSec
		if intervalSec < 2 {
			intervalSec = 2
		}

		log.Printf("[СТАТ] Пользователей: %d | Сессий: %d | Всего: %d | NAT: %s | ↑%.2f МБ | ↓%.2f МБ",
			users, sessions, total, natType,
			float64(fromC)/1024/1024,
			float64(toC)/1024/1024,
		)

		statsJSON, _ := json.Marshal(map[string]interface{}{
			"active_users":       users,
			"active":             users,
			"sessions":           sessions,
			"total":              total,
			"nat":                natType,
			"uptime":             uptimeStr,
			"uptime_sec":         int64(uptime.Seconds()),
			"vpn_active":         true,
			"down_gb":            fmt.Sprintf("%.2f", downGB),
			"up_gb":              fmt.Sprintf("%.2f", upGB),
			"passwords":          numPasswords,
			"devices":            numDevices,
			"online":             online,
			"timestamp":          time.Now().Unix(),
			"stats_interval_sec": intervalSec,
		})
		os.WriteFile(statsFile, statsJSON, 0600)

		peersCopy := peers
		peersOK := peerErr == nil
		go func() {
			if peersOK {
				syncTrafficFromWGPeerInfos(peersCopy)
			}
			syncVPNLocalServices(wgIfaceName)
			relayEvictAllIdle(relayStaleEvictIdle)
		}()

		if trafficDirty.Load() {
			dbMutex.Lock()
			if err := saveTrafficToSQLiteLocked(); err != nil {
				log.Printf("[DB] save traffic: %v", err)
			} else {
				trafficDirty.Store(false)
			}
			dbMutex.Unlock()
		}
	}
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dд %dч %dм", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dч %dм", hours, mins)
	}
	return fmt.Sprintf("%dм", mins)
}

// ==================== Утилиты ====================
