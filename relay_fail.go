package main

import (
	"log"
	"net"
	"sync"
	"time"
)

// После рестарта iOS часто шлёт WRAP со старых TURN relay; DTLS там не поднимается.
// Укороченный timeout и сброс счётчика после успеха ускоряют переход на новые relay.

var (
	relayFailMu     sync.Mutex
	relayFailCounts = map[string]int{}
)

func relayHandshakeTimeoutFor(addr net.Addr) time.Duration {
	if addr == nil {
		return dtlsHandshakeTimeout
	}
	relayFailMu.Lock()
	fails := relayFailCounts[addr.String()]
	relayFailMu.Unlock()

	switch {
	case fails >= 3:
		return 5 * time.Second
	case fails >= 1:
		return 10 * time.Second
	default:
		return dtlsHandshakeTimeout
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
	if n == 2 || n == 5 {
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
	return fails >= 5
}
