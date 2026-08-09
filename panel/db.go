package panel

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	_ "modernc.org/sqlite"
)

var panelDBPath = "/etc/wdtt/panel.db"

var panelDB *sql.DB

func panelDBEnabled() bool {
	return panelDB != nil
}

func initPanelDB() error {
	if err := os.MkdirAll(filepath.Dir(panelDBPath), 0700); err != nil {
		return err
	}
	dsn := panelDBPath + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return err
	}
	panelDB = db
	if err := migratePanelDB(); err != nil {
		db.Close()
		return err
	}
	return paneldb.OpenExisting(panelDB)
}

func migratePanelDB() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY NOT NULL,
			value TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, stmt := range stmts {
		if _, err := panelDB.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	var ver int
	err := panelDB.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&ver)
	if err == sql.ErrNoRows {
		if err := migratePanelDBV2(); err != nil {
			return err
		}
		if err := migratePanelDBV3(); err != nil {
			return err
		}
		if err := migratePanelDBV4(); err != nil {
			return err
		}
		if err := migratePanelDBV5(); err != nil {
			return err
		}
		if err := migratePanelDBV6(); err != nil {
			return err
		}
		if err := migratePanelDBV7(); err != nil {
			return err
		}
		if err := migratePanelDBV8(); err != nil {
			return err
		}
		if err := migratePanelDBV9(); err != nil {
			return err
		}
		if err := migratePanelDBV10(); err != nil {
			return err
		}
		if err := migratePanelDBV11(); err != nil {
			return err
		}
		if err := migratePanelDBV12(); err != nil {
			return err
		}
		if err := migratePanelDBV13(); err != nil {
			return err
		}
		if err := migratePanelDBV14(); err != nil {
			return err
		}
		if err := migratePanelDBV15(); err != nil {
			return err
		}
		if _, err = panelDB.Exec(`INSERT INTO schema_version (version) VALUES (?)`, dbSchemaVersion); err != nil {
			return err
		}
		migrateLegacyJSONFiles()
		return migrateEnsureUserSubIDs()
	}
	if err != nil {
		return err
	}
	if ver < 2 {
		if err := migratePanelDBV2(); err != nil {
			return err
		}
	}
	if ver < 3 {
		if err := migratePanelDBV3(); err != nil {
			return err
		}
	}
	if ver < 4 {
		if err := migratePanelDBV4(); err != nil {
			return err
		}
	}
	if ver < 5 {
		if err := migratePanelDBV5(); err != nil {
			return err
		}
	}
	if ver < 6 {
		if err := migratePanelDBV6(); err != nil {
			return err
		}
	}
	if ver < 7 {
		if err := migratePanelDBV7(); err != nil {
			return err
		}
	}
	if ver < 8 {
		if err := migratePanelDBV8(); err != nil {
			return err
		}
	}
	if ver < 9 {
		if err := migratePanelDBV9(); err != nil {
			return err
		}
	}
	if ver < 10 {
		if err := migratePanelDBV10(); err != nil {
			return err
		}
	}
	if ver < 11 {
		if err := migratePanelDBV11(); err != nil {
			return err
		}
	}
	if ver < 12 {
		if err := migratePanelDBV12(); err != nil {
			return err
		}
	}
	if ver < 13 {
		if err := migratePanelDBV13(); err != nil {
			return err
		}
	}
	if ver < 14 {
		if err := migratePanelDBV14(); err != nil {
			return err
		}
	}
	if ver < 15 {
		if err := migratePanelDBV15(); err != nil {
			return err
		}
	}
	if ver < dbSchemaVersion {
		_, err = panelDB.Exec(`UPDATE schema_version SET version = ?`, dbSchemaVersion)
	}
	if err != nil {
		return err
	}
	migrateLegacyJSONFiles()
	// Повторно после старого wdtt-server без sub_id в save/load.
	return migrateEnsureUserSubIDs()
}

func loadPanelConfigFromDB() (*PanelConfig, error) {
	return loadPanelConfigNorm()
}

func savePanelConfigToDB(cfg *PanelConfig) error {
	return savePanelConfigNorm(cfg)
}

func exportPanelDB(w io.Writer) error {
	if _, err := os.Stat(panelDBPath); err != nil {
		return err
	}
	f, err := os.Open(panelDBPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

func importPanelDBFromReader(r io.Reader) error {
	if panelDB == nil {
		return fmt.Errorf("database not initialized")
	}
	tmpPath := panelDBPath + ".import"
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	testDB, err := sql.Open("sqlite", tmpPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := testDB.Ping(); err != nil {
		testDB.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("invalid sqlite database: %w", err)
	}
	var ver int
	if err := testDB.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&ver); err != nil {
		testDB.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("unsupported database schema: %w", err)
	}
	testDB.Close()

	if _, err := os.Stat(panelDBPath); err == nil {
		backup := panelDBPath + ".bak"
		_ = os.Remove(backup)
		if err := os.Rename(panelDBPath, backup); err != nil {
			os.Remove(tmpPath)
			return err
		}
	}
	if err := os.Rename(tmpPath, panelDBPath); err != nil {
		return err
	}
	panelDB.Close()
	panelDB = nil
	return initPanelDB()
}
