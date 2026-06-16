package panel

import "sync"

const (
	defaultDashboardPollSec    = 2
	defaultUsersPollSec        = 5
	defaultConnectionsPollSec  = 5
	minDashboardPollSec        = 2
	maxDashboardPollSec        = 120
	minPagePollSec             = 3
	maxPagePollSec             = 120
)

var (
	pollDefaultsMu       sync.RWMutex
	dashboardPollSec     = defaultDashboardPollSec
	usersPollSec         = defaultUsersPollSec
	connectionsPollSec   = defaultConnectionsPollSec
)

func clampDashboardPollSec(v int) int {
	if v < minDashboardPollSec {
		return minDashboardPollSec
	}
	if v > maxDashboardPollSec {
		return maxDashboardPollSec
	}
	return v
}

func clampPagePollSec(v int) int {
	if v < minPagePollSec {
		return minPagePollSec
	}
	if v > maxPagePollSec {
		return maxPagePollSec
	}
	return v
}

func syncPanelPollDefaults(cfg *PanelConfig) {
	if cfg == nil {
		return
	}
	pollDefaultsMu.Lock()
	defer pollDefaultsMu.Unlock()
	dashboardPollSec = clampDashboardPollSec(cfg.DashboardPollSec)
	usersPollSec = clampPagePollSec(cfg.UsersPollSec)
	connectionsPollSec = clampPagePollSec(cfg.ConnectionsPollSec)
}

func panelPollDefaultsMap() map[string]int {
	pollDefaultsMu.RLock()
	defer pollDefaultsMu.RUnlock()
	return map[string]int{
		"dashboardPollSec":   dashboardPollSec,
		"usersPollSec":       usersPollSec,
		"connectionsPollSec": connectionsPollSec,
	}
}
