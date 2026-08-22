package paneldb

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	APITokenScopeAdmin   = "admin"
	APITokenScopeReadonly = "readonly"
)

// APIToken — строка api_tokens.
type APIToken struct {
	ID        int64  `json:"id"`
	Token     string `json:"token"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	LastUsed  int64  `json:"last_used"`
}

// GenerateAPIToken генерирует токен формата wdtt_<32 hex>.
func GenerateAPIToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "wdtt_" + hex.EncodeToString(b), nil
}

// CreateAPIToken создаёт новый токен и возвращает его (с полным token).
func CreateAPIToken(db *sql.DB, name, scope string) (*APIToken, error) {
	if db == nil {
		return nil, fmt.Errorf("nil db")
	}
	if scope != APITokenScopeAdmin && scope != APITokenScopeReadonly {
		scope = APITokenScopeAdmin
	}
	token, err := GenerateAPIToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	res, err := db.Exec(`INSERT INTO api_tokens (token, name, scope, enabled, created_at, last_used)
		VALUES (?, ?, ?, 1, ?, 0)`, token, name, scope, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &APIToken{
		ID:        id,
		Token:     token,
		Name:      name,
		Scope:     scope,
		Enabled:   true,
		CreatedAt: now,
	}, nil
}

// ListAPITokens возвращает все токены (без полного token для безопасности).
func ListAPITokens(db *sql.DB) ([]APIToken, error) {
	if db == nil {
		return nil, fmt.Errorf("nil db")
	}
	rows, err := db.Query(`SELECT id, token, name, scope, enabled, created_at, last_used
		FROM api_tokens ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		var t APIToken
		var enabled int
		if err := rows.Scan(&t.ID, &t.Token, &t.Name, &t.Scope, &enabled, &t.CreatedAt, &t.LastUsed); err != nil {
			return nil, err
		}
		t.Enabled = enabled != 0
		// Маскируем токен: показываем только первые 12 символов.
		if len(t.Token) > 12 {
			t.Token = t.Token[:12] + "..."
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// LookupAPIToken ищет токен по полному значению. Возвращает nil если не найден или отключён.
func LookupAPIToken(db *sql.DB, token string) (*APIToken, error) {
	if db == nil || token == "" {
		return nil, nil
	}
	var t APIToken
	var enabled int
	err := db.QueryRow(`SELECT id, token, name, scope, enabled, created_at, last_used
		FROM api_tokens WHERE token = ?`, token).Scan(
		&t.ID, &t.Token, &t.Name, &t.Scope, &enabled, &t.CreatedAt, &t.LastUsed,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if enabled == 0 {
		return nil, nil
	}
	t.Enabled = true
	return &t, nil
}

// TouchAPIToken обновляет last_used.
func TouchAPIToken(db *sql.DB, id int64) {
	if db == nil || id == 0 {
		return
	}
	_, _ = db.Exec(`UPDATE api_tokens SET last_used = ? WHERE id = ?`, time.Now().Unix(), id)
}

// DeleteAPIToken удаляет токен по ID.
func DeleteAPIToken(db *sql.DB, id int64) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	_, err := db.Exec(`DELETE FROM api_tokens WHERE id = ?`, id)
	return err
}

// ToggleAPIToken включает/выключает токен.
func ToggleAPIToken(db *sql.DB, id int64) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	_, err := db.Exec(`UPDATE api_tokens SET enabled = 1 - enabled WHERE id = ?`, id)
	return err
}
