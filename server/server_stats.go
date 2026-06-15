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
)

func statsLoop(ctx context.Context, configDir string) {
	serverStartTime = time.Now()
	statsFile := filepath.Join(configDir, "server.log")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fromC := atomic.LoadInt64(&totalBytesFromClient)
			toC := atomic.LoadInt64(&totalBytesToClient)
			sessions := atomic.LoadInt32(&activeConns)
			refreshWGActivity()

			online := snapshotOnlineUsers()
			users := int32(len(online))
			atomic.StoreInt32(&activeUsers, users)
			forEachOnlinePassword(touchUserLastSeen)
			total := atomic.LoadInt64(&totalConns)
			uptime := time.Since(serverStartTime)

			log.Printf("[СТАТ] Пользователей: %d | Сессий: %d | Всего: %d | NAT: %s | ↑%.2f МБ | ↓%.2f МБ",
				users, sessions, total, natType,
				float64(fromC)/1024/1024,
				float64(toC)/1024/1024,
			)

			// Пишем server.log
			dbMutex.Lock()
			numPasswords := len(db.Passwords)
			numDevices := len(db.Devices)
			dbMutex.Unlock()

			uptimeStr := formatUptime(uptime)
			downGB := float64(toC) / (1024 * 1024 * 1024)
			upGB := float64(fromC) / (1024 * 1024 * 1024)

			statsJSON, _ := json.Marshal(map[string]interface{}{
				"active_users": users,
				"active":       users,
				"sessions":     sessions,
				"total":        total,
				"nat":          natType,
				"uptime":       uptimeStr,
				"down_gb":      fmt.Sprintf("%.2f", downGB),
				"up_gb":        fmt.Sprintf("%.2f", upGB),
				"passwords":    numPasswords,
				"devices":      numDevices,
				"online":       online,
				"timestamp":    time.Now().Unix(),
			})
			os.WriteFile(statsFile, statsJSON, 0600)

			go func() {
				syncTrafficFromWGPeers()
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
