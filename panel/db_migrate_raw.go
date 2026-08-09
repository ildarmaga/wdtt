package panel

import "strings"

func migratePanelDBV15() error {
	_, err := panelDB.Exec(`ALTER TABLE wdtt_inbound ADD COLUMN raw_enable INTEGER NOT NULL DEFAULT 1`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return err
	}
	return nil
}
