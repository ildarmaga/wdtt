package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type xrayTrafficPersist struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

var (
	xrayTrafficMu     sync.Mutex
	xrayPersistedUp   int64
	xrayPersistedDown int64
	xrayLastAPIUp     int64
	xrayLastAPIDown   int64
	xrayHasAPISample  bool
	xrayTrafficLoaded bool
	xrayTrafficDirty  bool
)

func xrayTrafficFile() string {
	return filepath.Join(wdttConfigDir, "xray_traffic.json")
}

func loadXrayTrafficPersist() {
	xrayTrafficMu.Lock()
	defer xrayTrafficMu.Unlock()
	if xrayTrafficLoaded {
		return
	}
	xrayTrafficLoaded = true
	data, err := os.ReadFile(xrayTrafficFile())
	if err != nil {
		return
	}
	var p xrayTrafficPersist
	if json.Unmarshal(data, &p) == nil {
		xrayPersistedUp = p.Up
		xrayPersistedDown = p.Down
	}
}

func saveXrayTrafficPersistLocked() {
	if !xrayTrafficDirty {
		return
	}
	data, err := json.MarshalIndent(xrayTrafficPersist{
		Up:   xrayPersistedUp,
		Down: xrayPersistedDown,
	}, "", "  ")
	if err != nil {
		return
	}
	tmp := xrayTrafficFile() + ".tmp"
	if os.WriteFile(tmp, data, 0600) != nil {
		return
	}
	_ = os.Rename(tmp, xrayTrafficFile())
	xrayTrafficDirty = false
}

// syncXrayTrafficTotals добавляет дельту из Xray API к сохранённым значениям.
// При перезапуске Xray счётчики API обнуляются — дельта не уходит в минус.
func syncXrayTrafficTotals() (up, down int64) {
	loadXrayTrafficPersist()

	traffics, err := getOutboundsTraffic()
	if err != nil || len(traffics) == 0 {
		xrayTrafficMu.Lock()
		up, down = xrayPersistedUp, xrayPersistedDown
		xrayTrafficMu.Unlock()
		return up, down
	}

	var apiUp, apiDown int64
	for _, t := range traffics {
		apiUp += t.Up
		apiDown += t.Down
	}

	xrayTrafficMu.Lock()
	defer xrayTrafficMu.Unlock()

	if xrayHasAPISample {
		if apiUp >= xrayLastAPIUp {
			xrayPersistedUp += apiUp - xrayLastAPIUp
			xrayTrafficDirty = true
		}
		if apiDown >= xrayLastAPIDown {
			xrayPersistedDown += apiDown - xrayLastAPIDown
			xrayTrafficDirty = true
		}
	}
	xrayLastAPIUp, xrayLastAPIDown = apiUp, apiDown
	xrayHasAPISample = true
	saveXrayTrafficPersistLocked()
	return xrayPersistedUp, xrayPersistedDown
}

func combinedTrafficTotals(db *PasswordsDB) (sent, recv uint64) {
	var wdttUp, wdttDown int64
	if db != nil {
		wdttUp, wdttDown = passwordsTrafficTotals(db)
	} else {
		s, r := vpnTrafficTotals()
		wdttUp, wdttDown = int64(s), int64(r)
	}
	xrayUp, xrayDown := syncXrayTrafficTotals()
	return uint64(wdttUp + xrayUp), uint64(wdttDown + xrayDown)
}
