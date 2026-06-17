package paneldb

import (
	"database/sql"
	"fmt"
	"strings"
)

// PatchUserDeviceBindings обновляет привязки одного пароля без полной перезаписи store.
// max_devices не меняется — только device_id (legacy) и wdtt_user_devices.
func PatchUserDeviceBindings(db *sql.DB, mainPassword, password string, deviceIDs []string, removeDeviceIDs []string) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return fmt.Errorf("password required")
	}
	u := &User{DeviceIDs: append([]string(nil), deviceIDs...)}
	NormalizeUser(u)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM wdtt_user_devices WHERE password = ?`, password); err != nil {
		return err
	}
	for i, did := range AllDeviceIDs(u) {
		if _, err := tx.Exec(`INSERT INTO wdtt_user_devices (password, device_id, sort_order) VALUES (?,?,?)`,
			password, did, i); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE wdtt_users SET device_id = ? WHERE password = ?`,
		u.DeviceID, password); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, did := range removeDeviceIDs {
		did = strings.TrimSpace(did)
		if did == "" || seen[did] {
			continue
		}
		seen[did] = true
		if _, err := tx.Exec(`DELETE FROM wdtt_devices WHERE device_id = ?`, did); err != nil {
			return err
		}
	}
	if err := bumpUsersRevInTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}
