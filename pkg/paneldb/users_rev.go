package paneldb

import "database/sql"

// LoadUsersRev returns monotonic revision of user/device bindings (panel edits bump it).
func LoadUsersRev(db *sql.DB) (int64, error) {
	if db == nil {
		return 0, nil
	}
	var rev int64
	err := db.QueryRow(`SELECT users_rev FROM wdtt_global WHERE id = 1`).Scan(&rev)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return rev, err
}

func bumpUsersRevInTx(tx *sql.Tx) error {
	if tx == nil {
		return nil
	}
	_, err := tx.Exec(`UPDATE wdtt_global SET users_rev = users_rev + 1 WHERE id = 1`)
	return err
}
