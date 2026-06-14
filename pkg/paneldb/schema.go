package paneldb

import (
	"database/sql"
	"strings"
)

// EnsureWDTTSchema — idempotent ALTER для wdtt_* (общее для сервера и панели).
func EnsureWDTTSchema(db *sql.DB) error {
	for _, stmt := range []string{
		`ALTER TABLE wdtt_global ADD COLUMN admin_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE wdtt_global ADD COLUMN bot_token TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE wdtt_users ADD COLUMN sub_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE wdtt_users ADD COLUMN last_seen_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE wdtt_inbound ADD COLUMN online_timeout_sec INTEGER NOT NULL DEFAULT 15`,
	} {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}

func tableCount(db *sql.DB, table string) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
	return n, err
}

// UserCount — число строк в wdtt_users.
func UserCount(db *sql.DB) (int, error) {
	return tableCount(db, "wdtt_users")
}

// HasUsers — есть ли хотя бы один пользователь.
func HasUsers(db *sql.DB) (bool, error) {
	n, err := UserCount(db)
	return n > 0, err
}
