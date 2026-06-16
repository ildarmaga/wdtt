package paneldb

import (
	"database/sql"
	"fmt"
	"strings"
)

// UpsertDevice inserts or updates a single wdtt_devices row (GETCONF hot path).
func UpsertDevice(db *sql.DB, d *Device) error {
	if db == nil || d == nil {
		return fmt.Errorf("nil db or device")
	}
	id := strings.TrimSpace(d.DeviceID)
	if id == "" {
		return fmt.Errorf("device_id required")
	}
	_, err := db.Exec(`INSERT INTO wdtt_devices (device_id, ip, priv_key, pub_key) VALUES (?,?,?,?)
		ON CONFLICT(device_id) DO UPDATE SET
			ip=excluded.ip,
			priv_key=excluded.priv_key,
			pub_key=excluded.pub_key`,
		id, d.IP, d.PrivKey, d.PubKey,
	)
	return err
}
