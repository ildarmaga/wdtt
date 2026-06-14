package paneldb

import "database/sql"

// TrafficSnapshot — счётчики трафика и состояние пользователя для row-level UPDATE.
type TrafficSnapshot struct {
	UpBytes       int64
	DownBytes     int64
	LastSeenAt    int64
	IsDeactivated bool
}

// UpdateUsersTraffic обновляет up/down/is_deactivated/last_seen_at без полной перезаписи store.
func UpdateUsersTraffic(db *sql.DB, updates map[string]TrafficSnapshot) error {
	if db == nil || len(updates) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for pass, u := range updates {
		if pass == "" {
			continue
		}
		deact := 0
		if u.IsDeactivated {
			deact = 1
		}
		if _, err := tx.Exec(`UPDATE wdtt_users SET
			up_bytes = ?,
			down_bytes = ?,
			is_deactivated = ?,
			last_seen_at = CASE WHEN ? > last_seen_at THEN ? ELSE last_seen_at END
			WHERE password = ?`,
			u.UpBytes, u.DownBytes, deact, u.LastSeenAt, u.LastSeenAt, pass,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MergeTrafficSnapshots берёт max(up/down/last_seen) и OR is_deactivated из from в into.
func MergeTrafficSnapshots(into map[string]*User, from map[string]TrafficSnapshot) {
	if into == nil || len(from) == 0 {
		return
	}
	for pass, snap := range from {
		u := into[pass]
		if u == nil {
			continue
		}
		if snap.UpBytes > u.UpBytes {
			u.UpBytes = snap.UpBytes
		}
		if snap.DownBytes > u.DownBytes {
			u.DownBytes = snap.DownBytes
		}
		if snap.LastSeenAt > u.LastSeenAt {
			u.LastSeenAt = snap.LastSeenAt
		}
		if snap.IsDeactivated {
			u.IsDeactivated = true
		}
	}
}
