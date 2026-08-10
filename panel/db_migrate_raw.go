package panel

import "strings"

func migratePanelDBV19() error {
	_, err := panelDB.Exec(`ALTER TABLE wdtt_inbound ADD COLUMN raw_enable INTEGER NOT NULL DEFAULT 1`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return err
	}
	_, err = panelDB.Exec(`ALTER TABLE wdtt_inbound ADD COLUMN raw_direct_port INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return err
	}
	return nil
}
