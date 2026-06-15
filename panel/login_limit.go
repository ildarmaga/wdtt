package panel

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginMaxFailures = 5
	loginLockWindow  = 15 * time.Minute
)

type loginLimiter struct {
	mu    sync.Mutex
	fails map[string][]time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{fails: make(map[string][]time.Time)}
}

var panelLoginLimiter = newLoginLimiter()

func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return xff
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func (l *loginLimiter) allow(ip string) bool {
	if ip == "" {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-loginLockWindow)
	prev := l.fails[ip]
	alive := prev[:0]
	for _, t := range prev {
		if t.After(cutoff) {
			alive = append(alive, t)
		}
	}
	l.fails[ip] = alive
	return len(alive) < loginMaxFailures
}

func (l *loginLimiter) recordFail(ip string) {
	if ip == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[ip] = append(l.fails[ip], time.Now())
}

func (l *loginLimiter) reset(ip string) {
	if ip == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, ip)
}

func (a *App) sessionCookieSecure() bool {
	if a.cfg == nil {
		return false
	}
	return strings.TrimSpace(a.cfg.WebCertFile) != "" && strings.TrimSpace(a.cfg.WebKeyFile) != ""
}
