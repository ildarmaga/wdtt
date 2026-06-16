package paneldb

import (
	"database/sql"
	"fmt"
	"strings"
)

// LoadStore читает wdtt_global, wdtt_users, wdtt_user_devices, wdtt_devices.
func LoadStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("nil db")
	}
	out := NewStore()
	_ = db.QueryRow(`SELECT main_password, admin_id, bot_token FROM wdtt_global WHERE id = 1`).Scan(
		&out.MainPassword, &out.AdminID, &out.BotToken,
	)
	rows, err := db.Query(`SELECT password, device_id, max_devices, expires_at, down_bytes, up_bytes,
		total_bytes, max_down_mbps, max_up_mbps, is_deactivated, comment, ports, vk_hash, sub_id, last_seen_at FROM wdtt_users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		u := &User{}
		var pass string
		var deactivated int
		if err := rows.Scan(&pass, &u.DeviceID, &u.MaxDevices, &u.ExpiresAt, &u.DownBytes, &u.UpBytes,
			&u.TotalBytes, &u.MaxDownMBps, &u.MaxUpMBps, &deactivated, &u.Comment, &u.Ports, &u.VkHash, &u.SubID, &u.LastSeenAt); err != nil {
			return nil, err
		}
		u.IsDeactivated = deactivated != 0
		out.Users[pass] = u
	}
	drows, err := db.Query(`SELECT password, device_id FROM wdtt_user_devices ORDER BY password, sort_order`)
	if err != nil {
		return nil, err
	}
	defer drows.Close()
	for drows.Next() {
		var pass, did string
		if err := drows.Scan(&pass, &did); err != nil {
			return nil, err
		}
		if u := out.Users[pass]; u != nil {
			u.DeviceIDs = append(u.DeviceIDs, did)
		}
	}
	devRows, err := db.Query(`SELECT device_id, ip, priv_key, pub_key FROM wdtt_devices`)
	if err != nil {
		return nil, err
	}
	defer devRows.Close()
	for devRows.Next() {
		d := &Device{}
		if err := devRows.Scan(&d.DeviceID, &d.IP, &d.PrivKey, &d.PubKey); err != nil {
			return nil, err
		}
		out.Devices[d.DeviceID] = d
	}
	for _, u := range out.Users {
		NormalizeUser(u)
	}
	for pass, u := range out.Users {
		if pass == out.MainPassword && u != nil {
			u.MaxDevices = 0
		}
	}
	return out, nil
}

// SaveStore полностью перезаписывает wdtt_users/devices (как раньше в panel_db + db_norm).
func SaveStore(db *sql.DB, src *Store, opts SaveOptions) error {
	if db == nil || src == nil {
		return fmt.Errorf("nil db or store")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO wdtt_global (id, main_password, admin_id, bot_token) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			main_password=excluded.main_password,
			admin_id=excluded.admin_id,
			bot_token=excluded.bot_token`,
		src.MainPassword, src.AdminID, src.BotToken); err != nil {
		return err
	}
	if err := bumpUsersRevInTx(tx); err != nil {
		return err
	}

	existingSubIDs := map[string]string{}
	if opts.PreserveSubIDs {
		subRows, err := tx.Query(`SELECT password, sub_id FROM wdtt_users WHERE sub_id != ''`)
		if err != nil {
			return err
		}
		for subRows.Next() {
			var pass, sid string
			if err := subRows.Scan(&pass, &sid); err != nil {
				subRows.Close()
				return err
			}
			existingSubIDs[pass] = sid
		}
		if err := subRows.Close(); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`DELETE FROM wdtt_user_devices`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wdtt_users`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wdtt_devices`); err != nil {
		return err
	}

	for pass, u := range src.Users {
		if u == nil {
			continue
		}
		NormalizeUser(u)
		if pass == src.MainPassword {
			u.MaxDevices = 0
		}
		deact := 0
		if u.IsDeactivated {
			deact = 1
		}
		subID := strings.TrimSpace(u.SubID)
		if subID == "" && opts.PreserveSubIDs {
			subID = existingSubIDs[pass]
		}
		if _, err := tx.Exec(`INSERT INTO wdtt_users (
			password, device_id, max_devices, expires_at, down_bytes, up_bytes, total_bytes,
			max_down_mbps, max_up_mbps, is_deactivated, comment, ports, vk_hash, sub_id, last_seen_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			pass, u.DeviceID, u.MaxDevices, u.ExpiresAt, u.DownBytes, u.UpBytes,
			u.TotalBytes, u.MaxDownMBps, u.MaxUpMBps, deact, u.Comment, u.Ports, u.VkHash, subID, u.LastSeenAt,
		); err != nil {
			return err
		}
		for i, did := range AllDeviceIDs(u) {
			if _, err := tx.Exec(`INSERT INTO wdtt_user_devices (password, device_id, sort_order) VALUES (?,?,?)`,
				pass, did, i); err != nil {
				return err
			}
		}
	}
	for id, d := range src.Devices {
		if d == nil {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO wdtt_devices (device_id, ip, priv_key, pub_key) VALUES (?,?,?,?)`,
			id, d.IP, d.PrivKey, d.PubKey); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateLastSeen обновляет last_seen_at если ts новее.
func UpdateLastSeen(db *sql.DB, password string, ts int64) error {
	if password == "" || ts <= 0 {
		return nil
	}
	_, err := db.Exec(`UPDATE wdtt_users SET last_seen_at = ? WHERE password = ? AND last_seen_at < ?`, ts, password, ts)
	return err
}

// UpdateLastSeenBatch обновляет last_seen_at для нескольких паролей одной транзакцией.
func UpdateLastSeenBatch(db *sql.DB, updates map[string]int64) error {
	if len(updates) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`UPDATE wdtt_users SET last_seen_at = ? WHERE password = ? AND last_seen_at < ?`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for password, ts := range updates {
		if password == "" || ts <= 0 {
			continue
		}
		if _, err := stmt.Exec(ts, password, ts); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
