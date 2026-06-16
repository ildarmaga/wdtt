package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type eventHub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

var statusEvents = eventHub{subs: make(map[chan []byte]struct{})}

func (h *eventHub) subscribe() chan []byte {
	ch := make(chan []byte, 2)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *eventHub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *eventHub) broadcast(frame []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- frame:
		default:
		}
	}
}

func formatSSEPayload(eventType string, obj interface{}) []byte {
	data, _ := json.Marshal(map[string]interface{}{
		"type": eventType,
		"obj":  obj,
	})
	return []byte(fmt.Sprintf("data: %s\n\n", data))
}

func broadcastServerStatusEvent() {
	s := getCachedServerStatus()
	if s == nil {
		return
	}
	payload := map[string]interface{}{
		"status":   s,
		"usersRev": loadPanelUsersRev(),
	}
	statusEvents.broadcast(formatSSEPayload("status", payload))
}

func (a *App) handleServerEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := statusEvents.subscribe()
	defer statusEvents.unsubscribe(ch)

	s := getCachedServerStatus()
	payload := map[string]interface{}{
		"status":   s,
		"usersRev": loadPanelUsersRev(),
	}
	if _, err := w.Write(formatSSEPayload("status", payload)); err != nil {
		return
	}
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case frame := <-ch:
			if _, err := w.Write(frame); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (a *App) handleServerMetrics(w http.ResponseWriter, r *http.Request) {
	s := getCachedServerStatus()
	if s == nil {
		s = &serverStatus{}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP wdtt_active_users Unique VPN devices online\n")
	fmt.Fprintf(w, "# TYPE wdtt_active_users gauge\n")
	fmt.Fprintf(w, "wdtt_active_users %d\n", s.Wdtt.ActiveUsers)
	fmt.Fprintf(w, "# HELP wdtt_sessions Active DTLS sessions\n")
	fmt.Fprintf(w, "# TYPE wdtt_sessions gauge\n")
	fmt.Fprintf(w, "wdtt_sessions %d\n", s.Wdtt.Sessions)
	fmt.Fprintf(w, "# HELP wdtt_server_uptime_seconds VPN server uptime\n")
	fmt.Fprintf(w, "# TYPE wdtt_server_uptime_seconds gauge\n")
	fmt.Fprintf(w, "wdtt_server_uptime_seconds %d\n", s.Wdtt.ServerUptime)
	fmt.Fprintf(w, "# HELP wdtt_panel_uptime_seconds Panel process uptime\n")
	fmt.Fprintf(w, "# TYPE wdtt_panel_uptime_seconds gauge\n")
	fmt.Fprintf(w, "wdtt_panel_uptime_seconds %d\n", s.AppStats.Uptime)
}

func loadPanelUsersRev() int64 {
	if !panelDBEnabled() {
		return 0
	}
	rev, err := paneldbLoadUsersRev()
	if err != nil {
		return 0
	}
	return rev
}
