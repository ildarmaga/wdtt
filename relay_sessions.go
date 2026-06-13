package main

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Multi-worker clients keep many DTLS relays alive via 15s keepalive; 20s caused mass evictions.
const relayStaleEvictIdle = 3 * time.Minute

type relaySession struct {
	cancel       context.CancelFunc
	lastActivity int64
}

var (
	relaySessionsMu sync.Mutex
	relaySessions   = map[string][]*relaySession{}
)

func (s *relaySession) touch() {
	atomic.StoreInt64(&s.lastActivity, time.Now().UnixNano())
}

func relaySessionEvictIdle(deviceID string, minIdle time.Duration) int {
	if deviceID == "" {
		return 0
	}
	cutoff := time.Now().Add(-minIdle).UnixNano()

	relaySessionsMu.Lock()
	defer relaySessionsMu.Unlock()

	list := relaySessions[deviceID]
	if len(list) == 0 {
		return 0
	}

	kept := list[:0]
	evicted := 0
	for _, s := range list {
		if atomic.LoadInt64(&s.lastActivity) < cutoff {
			s.cancel()
			evicted++
		} else {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		delete(relaySessions, deviceID)
	} else {
		relaySessions[deviceID] = kept
	}
	if evicted > 0 {
		log.Printf("[RELAY] Evicted %d stale session(s) for %s (idle > %s)", evicted, deviceID, minIdle)
	}
	return evicted
}

func relaySessionRegister(deviceID string, cancel context.CancelFunc) *relaySession {
	s := &relaySession{cancel: cancel}
	s.touch()

	relaySessionsMu.Lock()
	relaySessions[deviceID] = append(relaySessions[deviceID], s)
	relaySessionsMu.Unlock()
	return s
}

func relaySessionUnregister(deviceID string, s *relaySession) {
	relaySessionsMu.Lock()
	defer relaySessionsMu.Unlock()

	list := relaySessions[deviceID]
	for i, x := range list {
		if x == s {
			relaySessions[deviceID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(relaySessions[deviceID]) == 0 {
		delete(relaySessions, deviceID)
	}
}

func relayEvictAllIdle(minIdle time.Duration) {
	relaySessionsMu.Lock()
	devices := make([]string, 0, len(relaySessions))
	for deviceID := range relaySessions {
		devices = append(devices, deviceID)
	}
	relaySessionsMu.Unlock()

	for _, deviceID := range devices {
		relaySessionEvictIdle(deviceID, minIdle)
	}
}

// relayEvictDevice закрывает все DTLS-relay устройства (при offline sweep).
func relayEvictDevice(deviceID string) int {
	if deviceID == "" {
		return 0
	}
	relaySessionsMu.Lock()
	defer relaySessionsMu.Unlock()
	list := relaySessions[deviceID]
	if len(list) == 0 {
		return 0
	}
	for _, s := range list {
		s.cancel()
	}
	delete(relaySessions, deviceID)
	return len(list)
}
