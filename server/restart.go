package server

import "sync"

var (
	restartNotifyMu sync.Mutex
	restartNotify   chan struct{}
)

func setRestartNotify(ch chan struct{}) {
	restartNotifyMu.Lock()
	restartNotify = ch
	restartNotifyMu.Unlock()
}

func notifyRestartRequested() {
	restartNotifyMu.Lock()
	ch := restartNotify
	restartNotifyMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}
