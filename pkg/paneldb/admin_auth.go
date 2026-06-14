package paneldb

import "database/sql"

// LoadPanelSessionKey returns panel_config.session_key (shared admin reload secret).
func LoadPanelSessionKey(db *sql.DB) (string, error) {
	if db == nil {
		return "", nil
	}
	var key string
	err := db.QueryRow(`SELECT session_key FROM panel_config WHERE id = 1`).Scan(&key)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return key, err
}
