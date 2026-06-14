package paneldb

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

// DSN — SQLite с WAL и foreign keys.
func DSN(path string) string {
	return path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
}

// Open открывает panel.db и применяет EnsureWDTTSchema.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		path = DefaultPath
	}
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", DSN(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := EnsureWDTTSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// OpenExisting — как Open, но без проверки Stat (для тестов).
func OpenExisting(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	return EnsureWDTTSchema(db)
}
