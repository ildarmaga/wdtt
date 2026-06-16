package panel

import (
	"testing"
	"time"
)

func TestServerStatsFresh(t *testing.T) {
	now := time.Now().Unix()
	fresh := &ServerStats{Timestamp: now, StatsIntervalSec: 5}
	if !serverStatsFresh(fresh) {
		t.Fatal("expected fresh stats")
	}
	maxAge := serverStatsMaxAgeSec(fresh)
	if serverStatsFresh(&ServerStats{Timestamp: now - maxAge - 1, StatsIntervalSec: 5}) {
		t.Fatal("expected stale stats")
	}
	if serverStatsFresh(nil) {
		t.Fatal("nil stats must be stale")
	}
}

func TestServerStatsFreshRespectsInterval(t *testing.T) {
	now := time.Now().Unix()
	s := &ServerStats{Timestamp: now - 6, StatsIntervalSec: 5}
	if !serverStatsFresh(s) {
		t.Fatal("6s old stats should stay fresh when interval=5 (maxAge=13)")
	}
}

func TestUserOnlineFromStatsIgnoresStale(t *testing.T) {
	stats := &ServerStats{
		Timestamp: time.Now().Unix() - 60,
		Online: []map[string]interface{}{
			{"device_id": "dev1", "password": "secret"},
		},
	}
	if userOnlineFromStats("secret", "dev1", false, stats) {
		t.Fatal("stale stats must not mark user online")
	}
}

func TestUserOnlineFromStatsFresh(t *testing.T) {
	stats := &ServerStats{
		Timestamp: time.Now().Unix(),
		Online: []map[string]interface{}{
			{"device_id": "dev1", "password": "secret"},
		},
	}
	if !userOnlineFromStats("secret", "dev1", false, stats) {
		t.Fatal("fresh stats should mark matching user online")
	}
}
