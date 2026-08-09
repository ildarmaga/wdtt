package server

import (
	"context"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/dtls/v3"
	"golang.zx2c4.com/wireguard/device"
)

func handleConn(ctx context.Context, clientConn net.Conn, wgEndpoint string, wgDev *device.Device, keys *wgKeys) {
	atomic.AddInt64(&totalConns, 1)

	var connPassword string
	var connIsMainPass bool
	var connDeviceID string
	var connUserIP string

	dtlsConn, ok := clientConn.(*dtls.Conn)
	if !ok {
		return
	}

	remoteAddr := clientConn.RemoteAddr()
	defer clearWrapAuth(remoteAddr)
	wrapAuthPass := lookupWrapAuth(remoteAddr)
	hsTimeout := relayHandshakeTimeoutFor(remoteAddr)

	select {
	case dtlsHandshakeSem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	hsStart := time.Now()
	hctx, hcancel := context.WithTimeout(ctx, hsTimeout)
	err := dtlsConn.HandshakeContext(hctx)
	hcancel()
	<-dtlsHandshakeSem

	if err != nil {
		log.Printf("[DTLS] Handshake failed from %s (timeout %s): %v", remoteAddr, hsTimeout, err)
		relayNoteHandshakeFail(remoteAddr)
		return
	}
	relayNoteHandshakeOK(remoteAddr)
	log.Printf("[DTLS] Handshake OK from %s in %s", remoteAddr, time.Since(hsStart).Round(time.Millisecond))

	atomic.AddInt32(&activeConns, 1)
	defer atomic.AddInt32(&activeConns, -1)

	buf := make([]byte, 1600)
	firstReadDeadline := dtlsHandshakeTimeout
	if hsTimeout > firstReadDeadline {
		firstReadDeadline = hsTimeout
	}
	clientConn.SetReadDeadline(time.Now().Add(firstReadDeadline))
	n, err := clientConn.Read(buf)
	if err != nil {
		log.Printf("[DTLS] First read failed from %s: %v", clientConn.RemoteAddr(), err)
		return
	}
	clientConn.SetReadDeadline(time.Time{})

	firstPacket := buf[:n]
	firstStr := string(firstPacket)

	// Старый RAW поверх DTLS убран: только direct WRAP на DTLS+3.
	if isRawConfPacket(firstStr) {
		log.Printf("[DTLS] RAWCONF отклонён с %s — нужен direct RAW (DTLS+3)", clientConn.RemoteAddr())
		_, _ = clientConn.Write([]byte("NOCONF"))
		return
	}

	if !strings.HasPrefix(firstStr, "GETCONF:") {
		preview := firstStr
		if len(preview) > 32 {
			preview = preview[:32] + "..."
		}
		log.Printf("[DTLS] Unexpected first packet from %s: %q", clientConn.RemoteAddr(), preview)
	}

	if strings.HasPrefix(firstStr, "GETCONF:") {
		parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(firstStr, "GETCONF:")), "|")
		clientPort := "9000"
		deviceID := "unknown"
		password := ""
		if len(parts) > 0 {
			clientPort = parts[0]
		}
		if len(parts) > 1 {
			deviceID = parts[1]
		}
		if len(parts) > 2 {
			password = parts[2]
		}
		if wrapAuthPass != "" {
			if password == "" {
				password = wrapAuthPass
			} else if password != wrapAuthPass {
				clientConn.Write([]byte("DENIED:wrong_password"))
				getconfFailLog(clientConn.RemoteAddr(), "wrap_password_mismatch")
				log.Printf("[WG] Отказ: GETCONF password != WRAP auth from %s", clientConn.RemoteAddr())
				return
			}
		}

		dbMutex.Lock()

		// Проверяем пароль
		isMainPass := password != "" && password == db.MainPassword
		entry, isGenPass := db.Passwords[password]
		valid := isMainPass || (isGenPass && !isPasswordExpired(entry) && !isTrafficExceeded(entry))

		if valid && isGenPass && entry.IsDeactivated {
			clientConn.Write([]byte("DENIED:deactivated"))
			log.Printf("[WG] Отказ: пароль %s деактивирован, запрос от %s", maskPassword(password), deviceID)
			dbMutex.Unlock()
			return
		}
		if valid && isGenPass && !isMainPass && !entryCanAcceptDevice(entry, deviceID) {
			clientConn.Write([]byte("DENIED:device_mismatch"))
			log.Printf("[WG] Отказ: пароль %s — лимит устройств %d/%d, запрос от %s",
				maskPassword(password), len(allEntryDeviceIDs(entry)), entryMaxDevices(entry), deviceID)
			dbMutex.Unlock()
			return
		}
		if valid {
			connPassword = password
			connIsMainPass = isMainPass

			persistFail := func() {
				clientConn.Write([]byte("NOCONF"))
				dbMutex.Unlock()
			}

			if isGenPass && !isMainPass && !entryHasDevice(entry, deviceID) {
				if bindDeviceToEntry(entry, deviceID) {
					if err := persistUserBindingsSQLiteLocked(password, entry); err != nil {
						log.Printf("[DB] bind device save: %v", err)
						persistFail()
						return
					}
					log.Printf("[WG] Пароль %s привязан к %s (%d/%d)",
						maskPassword(password), deviceID, len(entry.DeviceIDs), entryMaxDevices(entry))
				}
			}
			if isMainPass {
				// Главный пароль = безлимит устройств: НЕ записываем устройства в
				// привязки (wdtt_user_devices), чтобы они не копились в панели.
				// Трафик учитывается на главный пароль в памяти (orphan-резолв).
				ensureMainPasswordEntryLocked()
			}

			dev, exists := db.Devices[deviceID]
			if !exists {
				dev = &ClientDevice{DeviceID: deviceID, IP: getNextIP()}
				privB64, pubB64, keyErr := generateKeyPair()
				if keyErr == nil && dev.IP != "" {
					dev.PrivKey = privB64
					dev.PubKey = pubB64
					db.Devices[deviceID] = dev
					if err := persistDeviceSQLiteLocked(dev); err != nil {
						delete(db.Devices, deviceID)
						log.Printf("[DB] new device save: %v", err)
						persistFail()
						return
					}
					log.Printf("[WG] Новое устройство %s (IP: %s)", deviceID, dev.IP)
				} else {
					dev = nil
				}
			} else if dev.PrivKey == "" || dev.PubKey == "" {
				privB64, pubB64, keyErr := generateKeyPair()
				if keyErr == nil {
					dev.PrivKey = privB64
					dev.PubKey = pubB64
					if dev.IP == "" {
						dev.IP = getNextIP()
					}
					db.Devices[deviceID] = dev
					if err := persistDeviceSQLiteLocked(dev); err != nil {
						log.Printf("[DB] regen keys save: %v", err)
						persistFail()
						return
					}
					log.Printf("[WG] Восстановлены ключи устройства %s (IP: %s)", deviceID, dev.IP)
				} else {
					dev = nil
				}
			}
			if dev != nil {
				connDeviceID = deviceID
				connUserIP = dev.IP
				upsertPeerInWG(wgDev, dev)
				if isGenPass && entry != nil {
					applySpeedLimitForEntryUnlocked(entry)
				}
				clientConn.Write([]byte(buildClientConfig(keys.serverPublic, dev.PrivKey, dev.IP, clientPort)))
				log.Printf("[WG] GETCONF OK: device=%s ip=%s from %s", deviceID, dev.IP, clientConn.RemoteAddr())
				getconfFailReset(clientConn.RemoteAddr())
			} else {
				clientConn.Write([]byte("NOCONF"))
				dbMutex.Unlock()
				return
			}
			dbMutex.Unlock()
		} else {
			if isGenPass && isTrafficExceeded(entry) {
				clientConn.Write([]byte("DENIED:traffic_exceeded"))
				log.Printf("[WG] Отказ: лимит трафика для %s, от %s", maskPassword(password), deviceID)
			} else if isGenPass && isPasswordExpired(entry) {
				clientConn.Write([]byte("DENIED:expired"))
				log.Printf("[WG] Отказ: пароль %s истёк, от %s", maskPassword(password), deviceID)
			} else {
				clientConn.Write([]byte("DENIED:wrong_password"))
				getconfFailLog(clientConn.RemoteAddr(), "wrong_password")
			}
			dbMutex.Unlock()
			return
		}
		if connDeviceID != "" {
			relaySessionEvictIdle(connDeviceID, relayStaleEvictIdle)
		}
		clientConn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		n, err = clientConn.Read(buf)
		if err != nil {
			return
		}
		clientConn.SetReadDeadline(time.Time{})
		firstPacket = buf[:n]
		firstStr = string(firstPacket)
	}

	if firstStr == "READY" {
		clientConn.Write([]byte("READY_OK"))
		clientConn.SetReadDeadline(time.Now().Add(10 * time.Minute))
		n, err = clientConn.Read(buf)
		if err != nil {
			return
		}
		clientConn.SetReadDeadline(time.Time{})
		firstPacket = buf[:n]
	}

	// WG прокси
	wgConn, err := net.Dial("udp", wgEndpoint)
	if err != nil {
		return
	}
	defer wgConn.Close()

	if uc, ok := wgConn.(*net.UDPConn); ok {
		uc.SetReadBuffer(2 * 1024 * 1024)
		uc.SetWriteBuffer(2 * 1024 * 1024)
	}

	if _, err := wgConn.Write(firstPacket); err != nil {
		return
	}
	atomic.AddInt64(&totalBytesFromClient, int64(len(firstPacket)))

	// Клиент с сохранённым конфигом часто шлёт WG-пакеты сразу, без GETCONF.
	if connDeviceID == "" {
		id, ip, pass, main := tryResolveConnFromWG(wgConn)
		if id != "" {
			connDeviceID, connUserIP, connPassword, connIsMainPass = id, ip, pass, main
		}
	}

	pctx, pcancel := context.WithCancel(ctx)
	defer pcancel()

	var relaySess *relaySession
	var sessionEntered bool
	var sessionMu sync.Mutex

	enterSession := func() (ok, done bool) {
		sessionMu.Lock()
		defer sessionMu.Unlock()
		if sessionEntered {
			return true, true
		}
		if connDeviceID == "" {
			localPort := 0
			if ua, ok := wgConn.LocalAddr().(*net.UDPAddr); ok && ua != nil {
				localPort = ua.Port
			}
			if id, ip, pass, main := resolveConnByWGLocalPort(localPort); id != "" {
				connDeviceID, connUserIP, connPassword, connIsMainPass = id, ip, pass, main
				log.Printf("[WG] Без GETCONF: device=%s ip=%s pass=%s", id, ip, maskPassword(pass))
			}
		}
		if connDeviceID == "" {
			return false, false
		}
		if !userSessionEnter(connDeviceID, connUserIP, userLabel(connPassword, connIsMainPass), connPassword) {
			return false, true
		}
		sessionEntered = true
		relaySess = relaySessionRegister(connDeviceID, pcancel)
		return true, true
	}

	defer func() {
		sessionMu.Lock()
		id := connDeviceID
		entered := sessionEntered
		rs := relaySess
		sessionMu.Unlock()
		if entered && id != "" {
			userSessionLeave(id)
			if rs != nil {
				relaySessionUnregister(id, rs)
			}
		}
	}()

	if ok, done := enterSession(); done && !ok {
		log.Printf("[WG] Отказ: лимит %d DTLS-сессий для device=%s", maxDTLSPerDevice, connDeviceID)
		return
	}

	// Учёт трафика: saveDB → SQLite (panel.db).
	// Здесь только сброс локальных счётчиков и проверка лимита/деактивации.
	var sessUpBytes, sessDownBytes int64
	flushTraffic := func() bool {
		atomic.SwapInt64(&sessUpBytes, 0)
		atomic.SwapInt64(&sessDownBytes, 0)
		if ok, done := enterSession(); done && !ok {
			return false
		}
		dbMutex.Lock()
		defer dbMutex.Unlock()
		pass := resolveTrafficPassword(connPassword, connIsMainPass, connDeviceID)
		if pass == "" {
			return true
		}
		e, ok := db.Passwords[pass]
		if !ok || e == nil {
			return true
		}
		return !isTrafficExceeded(e) && !e.IsDeactivated
	}
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-pctx.Done():
				flushTraffic()
				return
			case <-t.C:
				if !flushTraffic() {
					pcancel()
					return
				}
			}
		}
	}()

	context.AfterFunc(pctx, func() {
		clientConn.SetDeadline(time.Now())
		wgConn.SetDeadline(time.Now())
	})

	var proxyWg sync.WaitGroup
	proxyWg.Add(2)

	// Клиент → WG
	go func() {
		defer proxyWg.Done()
		defer pcancel()
		b := getBuf()
		defer putBuf(b)
		for {
			select {
			case <-pctx.Done():
				return
			default:
			}
			clientConn.SetReadDeadline(time.Now().Add(30 * time.Minute))
			nn, err := clientConn.Read(*b)
			if err != nil {
				return
			}
			// DTLS keepalive ping клиента (1 байт 0xFF): обновляем presence и
			// эхо-отвечаем 0xFF (нужно старым клиентам для consent-freshness).
			//
			// ВАЖНО: эхо-запись идёт БЕЗ SetWriteDeadline. В v1.4.50 здесь
			// стоял write-deadline 5s, и он протекал на горутину «WG → Клиент»
			// (она пишет в тот же clientConn): клиент шлёт keepalive раз в 10s,
			// дедлайн истекал через 5s, и в окне 5–10s любая запись обратного
			// трафика падала по таймауту → сервер рвал сессию, а клиент видел
			// EOF и считал это «VK обновил relay». Отсюда массовые 16-секундные
			// смерти воркеров на v1.4.50, которых нет на v1.4.49. 1-байтовая
			// DTLS-запись поверх UDP не блокируется, поэтому дедлайн не нужен;
			// без него обратный трафик (тоже пишется без дедлайна) больше не
			// рвётся. Записи в clientConn из двух горутин безопасны —
			// pion/dtls сериализует Write внутренним локом.
			if nn == 1 && (*b)[0] == 0xFF {
				if relaySess != nil {
					relaySess.touch()
				}
				if sessionEntered {
					userTouchActivity(connDeviceID)
				}
				_, _ = clientConn.Write([]byte{0xFF})
				continue
			}
			atomic.AddInt64(&totalBytesFromClient, int64(nn))
			if ok, done := enterSession(); done && !ok {
				return
			}
			userTouchActivity(connDeviceID)
			if relaySess != nil {
				relaySess.touch()
			}
			atomic.AddInt64(&sessUpBytes, int64(nn))
			if _, err := wgConn.Write((*b)[:nn]); err != nil {
				return
			}
		}
	}()

	// WG → Клиент
	go func() {
		defer proxyWg.Done()
		defer pcancel()
		b := getBuf()
		defer putBuf(b)
		for {
			select {
			case <-pctx.Done():
				return
			default:
			}
			wgConn.SetReadDeadline(time.Now().Add(30 * time.Minute))
			nn, err := wgConn.Read(*b)
			if err != nil {
				if isNetTimeout(err) {
					if pctx.Err() != nil {
						return
					}
					continue
				}
				return
			}
			atomic.AddInt64(&totalBytesToClient, int64(nn))
			if ok, done := enterSession(); done && !ok {
				return
			}
			userTouchActivity(connDeviceID)
			if relaySess != nil {
				relaySess.touch()
			}
			atomic.AddInt64(&sessDownBytes, int64(nn))
			if _, err := clientConn.Write((*b)[:nn]); err != nil {
				return
			}
		}
	}()

	proxyWg.Wait()
}
