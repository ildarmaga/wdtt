package paneldb

import (
	"database/sql"
	"fmt"
	"strings"
)

// VKCall — созданный через панель VK-звонок (hash для пользователя).
type VKCall struct {
	CallID    string
	Password  string
	JoinLink  string
	VkHash    string
	StartedAt int64
	Finishing bool
}

const vkCallIDLen = 36

// ValidVKCallID — UUID call_id из calls.start (36 символов).
func ValidVKCallID(id string) bool {
	return len(strings.TrimSpace(id)) == vkCallIDLen
}

// ListVKCalls возвращает звонки, новые первыми.
func ListVKCalls(db *sql.DB) ([]VKCall, error) {
	if db == nil {
		return nil, fmt.Errorf("nil db")
	}
	rows, err := db.Query(`SELECT call_id, password, join_link, vk_hash, started_at, finishing
		FROM vk_calls ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VKCall
	for rows.Next() {
		var c VKCall
		var finishing int
		if err := rows.Scan(&c.CallID, &c.Password, &c.JoinLink, &c.VkHash, &c.StartedAt, &finishing); err != nil {
			return nil, err
		}
		c.Finishing = finishing != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// InsertVKCall сохраняет звонок (upsert по call_id).
func InsertVKCall(db *sql.DB, c VKCall) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	if !ValidVKCallID(c.CallID) {
		return fmt.Errorf("некорректный call_id %q", c.CallID)
	}
	fin := 0
	if c.Finishing {
		fin = 1
	}
	_, err := db.Exec(`INSERT INTO vk_calls (call_id, password, join_link, vk_hash, started_at, finishing)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(call_id) DO UPDATE SET
			password=excluded.password,
			join_link=excluded.join_link,
			vk_hash=excluded.vk_hash,
			started_at=excluded.started_at,
			finishing=excluded.finishing`,
		c.CallID, c.Password, c.JoinLink, c.VkHash, c.StartedAt, fin)
	return err
}

// SetVKCallFinishing помечает звонок как завершаемый (forceFinish уже отправлен).
func SetVKCallFinishing(db *sql.DB, callID string) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return fmt.Errorf("call_id пуст")
	}
	res, err := db.Exec(`UPDATE vk_calls SET finishing = 1 WHERE call_id = ?`, callID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("звонок не найден")
	}
	return nil
}

// SetVKCallsFinishingByPassword помечает все звонки профиля как завершаемые.
func SetVKCallsFinishingByPassword(db *sql.DB, password string) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return fmt.Errorf("password пуст")
	}
	_, err := db.Exec(`UPDATE vk_calls SET finishing = 1 WHERE password = ?`, password)
	return err
}

// DeleteVKCall удаляет звонок по call_id.
func DeleteVKCall(db *sql.DB, callID string) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return fmt.Errorf("call_id пуст")
	}
	_, err := db.Exec(`DELETE FROM vk_calls WHERE call_id = ?`, callID)
	return err
}

// DeleteVKCallsByPassword удаляет все звонки профиля.
func DeleteVKCallsByPassword(db *sql.DB, password string) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return fmt.Errorf("password пуст")
	}
	_, err := db.Exec(`DELETE FROM vk_calls WHERE password = ?`, password)
	return err
}

// PurgeInvalidVKCalls удаляет строки с битым call_id (legacy/тестовый мусор).
func PurgeInvalidVKCalls(db *sql.DB) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("nil db")
	}
	res, err := db.Exec(`DELETE FROM vk_calls WHERE length(trim(call_id)) != ?`, vkCallIDLen)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// LoadVKCookiesJSON читает cookies-vk JSON из БД.
func LoadVKCookiesJSON(db *sql.DB) ([]byte, error) {
	if db == nil {
		return nil, fmt.Errorf("nil db")
	}
	var raw string
	err := db.QueryRow(`SELECT cookies_json FROM vk_cookies WHERE id = 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	return []byte(raw), nil
}

// SaveVKCookiesJSON сохраняет cookies-vk JSON в БД.
func SaveVKCookiesJSON(db *sql.DB, raw []byte) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	_, err := db.Exec(`INSERT INTO vk_cookies (id, cookies_json, updated_at)
		VALUES (1, ?, strftime('%s','now'))
		ON CONFLICT(id) DO UPDATE SET cookies_json=excluded.cookies_json, updated_at=excluded.updated_at`,
		string(raw))
	return err
}

// ClearVKCookies удаляет cookies из БД.
func ClearVKCookies(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	_, err := db.Exec(`DELETE FROM vk_cookies WHERE id = 1`)
	return err
}
