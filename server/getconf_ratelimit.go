package main

import (
	"context"
	"log"
	"net"
	"sync"
	"time"
)

const getconfFailLogEvery = 10

var (
	getconfFailMu     sync.Mutex
	getconfFailCounts = map[string]int{}
)

// getconfFailLog logs denied GETCONF, rate-limited per relay+reason.
func getconfFailLog(addr net.Addr, reason string) {
	if addr == nil {
		log.Printf("[WG] GETCONF denied (%s)", reason)
		return
	}
	key := addr.String() + "|" + reason
	getconfFailMu.Lock()
	getconfFailCounts[key]++
	n := getconfFailCounts[key]
	getconfFailMu.Unlock()
	if n == 1 || n%getconfFailLogEvery == 0 {
		if n > 1 {
			log.Printf("[WG] GETCONF denied (%s) from %s (%d times)", reason, addr, n)
		} else {
			log.Printf("[WG] GETCONF denied (%s) from %s", reason, addr)
		}
		return
	}
}

func getconfFailReset(addr net.Addr) {
	if addr == nil {
		return
	}
	keyPrefix := addr.String() + "|"
	getconfFailMu.Lock()
	for k := range getconfFailCounts {
		if len(k) >= len(keyPrefix) && k[:len(keyPrefix)] == keyPrefix {
			delete(getconfFailCounts, k)
		}
	}
	getconfFailMu.Unlock()
}

// Periodic cleanup so the map does not grow forever.
func getconfFailJanitor(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			getconfFailMu.Lock()
			getconfFailCounts = map[string]int{}
			getconfFailMu.Unlock()
		}
	}
}
