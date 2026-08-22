package panel

func migratePanelDBV16() error {
	_, err := panelDB.Exec(`CREATE TABLE IF NOT EXISTS api_tokens (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		token      TEXT NOT NULL UNIQUE,
		name       TEXT NOT NULL DEFAULT '',
		scope      TEXT NOT NULL DEFAULT 'admin',
		enabled    INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL DEFAULT 0,
		last_used  INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return err
	}
	_, err = panelDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_tokens_token ON api_tokens(token)`)
	return err
}
