package paneldb

import (
	"database/sql"
	"fmt"
	"strings"
)

// UpsertUser inserts or replaces one wdtt_users row without touching other users.
func UpsertUser(db *sql.DB, mainPassword, password string, u *User) error {
	if db == nil || u == nil {
		return fmt.Errorf("nil db or user")
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return fmt.Errorf("password required")
	}
	NormalizeUser(u)
	maxDev := u.MaxDevices
	if password == strings.TrimSpace(mainPassword) {
		maxDev = 0
	}
	deact := 0
	if u.IsDeactivated {
		deact = 1
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO wdtt_users (
		password, device_id, max_devices, expires_at, down_bytes, up_bytes, total_bytes,
		max_down_mbps, max_up_mbps, is_deactivated, comment, ports, vk_hash, sub_id, last_seen_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(password) DO UPDATE SET
		device_id=excluded.device_id,
		max_devices=excluded.max_devices,
		expires_at=excluded.expires_at,
		down_bytes=excluded.down_bytes,
		up_bytes=excluded.up_bytes,
		total_bytes=excluded.total_bytes,
		max_down_mbps=excluded.max_down_mbps,
		max_up_mbps=excluded.max_up_mbps,
		is_deactivated=excluded.is_deactivated,
		comment=excluded.comment,
		ports=excluded.ports,
		vk_hash=excluded.vk_hash,
		sub_id=CASE WHEN excluded.sub_id != '' THEN excluded.sub_id ELSE wdtt_users.sub_id END,
		last_seen_at=excluded.last_seen_at`,
		password, u.DeviceID, maxDev, u.ExpiresAt, u.DownBytes, u.UpBytes,
		u.TotalBytes, u.MaxDownMBps, u.MaxUpMBps, deact, u.Comment, u.Ports, u.VkHash, u.SubID, u.LastSeenAt,
	); err != nil {
		return err
	}
	if err := bumpUsersRevInTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// RenameUserPassword changes primary key for user and device bindings.
func RenameUserPassword(db *sql.DB, oldPass, newPass string) error {
	oldPass = strings.TrimSpace(oldPass)
	newPass = strings.TrimSpace(newPass)
	if oldPass == "" || newPass == "" || oldPass == newPass {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE wdtt_users SET password = ? WHERE password = ?`, newPass, oldPass); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE wdtt_user_devices SET password = ? WHERE password = ?`, newPass, oldPass); err != nil {
		return err
	}
	if err := bumpUsersRevInTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// SetUserDeactivated toggles is_deactivated for one user.
func SetUserDeactivated(db *sql.DB, password string, deactivated bool) error {
	password = strings.TrimSpace(password)
	if password == "" {
		return nil
	}
	deact := 0
	if deactivated {
		deact = 1
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE wdtt_users SET is_deactivated = ? WHERE password = ?`, deact, password); err != nil {
		return err
	}
	if err := bumpUsersRevInTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteUser removes user row, bindings, and listed devices.
func DeleteUser(db *sql.DB, password string, deviceIDs []string) error {
	password = strings.TrimSpace(password)
	if password == "" {
		return fmt.Errorf("password required")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM wdtt_user_devices WHERE password = ?`, password); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wdtt_users WHERE password = ?`, password); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range deviceIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := tx.Exec(`DELETE FROM wdtt_devices WHERE device_id = ?`, id); err != nil {
			return err
		}
	}
	if err := bumpUsersRevInTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// SetMainPassword updates wdtt_global.main_password without rewriting users.
func SetMainPassword(db *sql.DB, mainPassword string) error {
	mainPassword = strings.TrimSpace(mainPassword)
	if mainPassword == "" {
		return fmt.Errorf("main password required")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE wdtt_global SET main_password = ? WHERE id = 1`, mainPassword); err != nil {
		return err
	}
	if err := bumpUsersRevInTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ResetUserTraffic zeroes traffic counters and clears quota deactivation for one user.
func ResetUserTraffic(db *sql.DB, password string) error {
	password = strings.TrimSpace(password)
	if password == "" {
		return fmt.Errorf("password required")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE wdtt_users SET up_bytes = 0, down_bytes = 0, is_deactivated = 0 WHERE password = ?`, password)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	if err := bumpUsersRevInTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteDevice removes device row and all user bindings referencing it.
func DeleteDevice(db *sql.DB, deviceID string) error {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM wdtt_user_devices WHERE device_id = ?`, deviceID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wdtt_devices WHERE device_id = ?`, deviceID); err != nil {
		return err
	}
	if err := bumpUsersRevInTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}
