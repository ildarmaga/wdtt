package panel

import (
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

const (
	legacyVKCookiesPath  = "/etc/wdtt/secrets/cookies-vk.json"
	legacyVKSessionsPath = "/etc/wdtt/vk-creator-sessions.json"
)

func migratePanelDBV13() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS vk_cookies (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			cookies_json TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS vk_calls (
			call_id TEXT PRIMARY KEY,
			password TEXT NOT NULL DEFAULT '',
			join_link TEXT NOT NULL DEFAULT '',
			vk_hash TEXT NOT NULL DEFAULT '',
			started_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vk_calls_password ON vk_calls(password)`,
		`CREATE INDEX IF NOT EXISTS idx_vk_calls_started ON vk_calls(started_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := panelDB.Exec(stmt); err != nil {
			return err
		}
	}
	if err := migrateLegacyVKFiles(); err != nil {
		return err
	}
	n, err := paneldb.PurgeInvalidVKCalls(panelDB)
	if err != nil {
		return err
	}
	if n > 0 {
		log.Printf("panel db: removed %d invalid vk_calls row(s)", n)
	}
	return nil
}

func migratePanelDBV14() error {
	_, err := panelDB.Exec(`ALTER TABLE vk_calls ADD COLUMN finishing INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return err
	}
	return nil
}

func migrateLegacyVKFiles() error {
	if data, err := os.ReadFile(legacyVKCookiesPath); err == nil && len(data) > 0 {
		existing, _ := paneldb.LoadVKCookiesJSON(panelDB)
		if len(existing) == 0 {
			if err := paneldb.SaveVKCookiesJSON(panelDB, data); err != nil {
				return err
			}
			log.Printf("panel db: migrated %s → vk_cookies", legacyVKCookiesPath)
		}
	}
	data, err := os.ReadFile(legacyVKSessionsPath)
	if err != nil {
		return nil
	}
	var store struct {
		Sessions []struct {
			Password  string `json:"password"`
			JoinLink  string `json:"join_link"`
			VkHash    string `json:"vk_hash"`
			CallID    string `json:"call_id"`
			StartedAt int64  `json:"started_at"`
		} `json:"sessions"`
	}
	if json.Unmarshal(data, &store) != nil {
		return nil
	}
	imported := 0
	for _, s := range store.Sessions {
		if !paneldb.ValidVKCallID(s.CallID) {
			continue
		}
		if err := paneldb.InsertVKCall(panelDB, paneldb.VKCall{
			CallID: s.CallID, Password: s.Password, JoinLink: s.JoinLink,
			VkHash: s.VkHash, StartedAt: s.StartedAt,
		}); err != nil {
			return err
		}
		imported++
	}
	if imported > 0 {
		log.Printf("panel db: migrated %s → vk_calls (%d)", legacyVKSessionsPath, imported)
	}
	_ = os.Remove(legacyVKSessionsPath)
	return nil
}
