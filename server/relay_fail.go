package main

import (
	"context"
	"log"
	"net"
	"sync"
	"time"
)

// После рестарта iOS часто шлёт WRAP со старых TURN relay; DTLS там не поднимается.
// Укороченный timeout и сброс счётчика после успеха ускоряют переход на новые relay.
// Базовый timeout берётся из панели (handshake_timeout_sec) → dtlsHandshakeTimeout.

const (
	relayDropAfterFails   = 3
	relayFailMapClearSize = 8000
)

var (
	relayFailMu     sync.Mutex
	relayFailCounts = map[string]int{}
)

func relayHandshakeTimeoutFor(addr net.Addr) time.Duration {
	base := dtlsHandshakeTimeout
	if addr == nil {
		return base
	}
	relayFailMu.Lock()
	fails := relayFailCounts[addr.String()]
	relayFailMu.Unlock()

	switch {
	case fails >= 2:
		t := base / 4
		if t < 5*time.Second {
			return 5 * time.Second
		}
		return t
	case fails >= 1:
		t := base / 2
		if t < 8*time.Second {
			return 8 * time.Second
		}
		return t
	default:
		return base
	}
}

func relayNoteHandshakeFail(addr net.Addr) {
	if addr == nil {
		return
	}
	key := addr.String()
	relayFailMu.Lock()
	relayFailCounts[key]++
	n := relayFailCounts[key]
	relayFailMu.Unlock()
	if n == 1 || n == relayDropAfterFails {
		log.Printf("[RELAY] Stale TURN relay %s: %d DTLS handshake fail(s), fast-fail enabled", key, n)
	}
}

func relayNoteHandshakeOK(addr net.Addr) {
	if addr == nil {
		return
	}
	key := addr.String()
	relayFailMu.Lock()
	delete(relayFailCounts, key)
	relayFailMu.Unlock()
}

func relayShouldDrop(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	relayFailMu.Lock()
	fails := relayFailCounts[addr.String()]
	relayFailMu.Unlock()
	return fails >= relayDropAfterFails
}

func relayFailJanitor(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			relayFailMu.Lock()
			n := len(relayFailCounts)
			if n > relayFailMapClearSize {
				relayFailCounts = map[string]int{}
				log.Printf("[RELAY] Cleared %d stale relay fail entries", n)
			}
			relayFailMu.Unlock()
		}
	}
}
