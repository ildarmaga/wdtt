package paneldb

import (
	"database/sql"
	"fmt"
	"strings"
)

// ReconcileVKHashesSync выравнивает wdtt_users.vk_hash с активными vk_calls.
// Источник истины — незавершённые vk_calls профиля (CSV в порядке started_at).
// Возвращает число изменённых профилей.
func ReconcileVKHashesSync(db *sql.DB) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("nil db")
	}
	wantByPass := map[string][]string{}
	crows, err := db.Query(`
		SELECT password, vk_hash FROM vk_calls
		WHERE finishing = 0 AND TRIM(vk_hash) != ''
		ORDER BY password, started_at ASC`)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	for crows.Next() {
		var pass, h string
		if err := crows.Scan(&pass, &h); err != nil {
			crows.Close()
			return 0, err
		}
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		list := wantByPass[pass]
		dup := false
		for _, x := range list {
			if x == h {
				dup = true
				break
			}
		}
		if !dup {
			wantByPass[pass] = append(list, h)
		}
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return 0, err
	}

	urows, err := db.Query(`SELECT password, TRIM(COALESCE(vk_hash, '')) FROM wdtt_users`)
	if err != nil {
		return 0, err
	}
	defer urows.Close()

	type fix struct{ pass, want string }
	var fixes []fix
	for urows.Next() {
		var pass, have string
		if err := urows.Scan(&pass, &have); err != nil {
			return 0, err
		}
		want := strings.Join(wantByPass[pass], ",")
		have = normalizeHashCSV(have)
		if have != want {
			fixes = append(fixes, fix{pass, want})
		}
	}
	if err := urows.Err(); err != nil {
		return 0, err
	}
	if len(fixes) == 0 {
		return 0, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, f := range fixes {
		if _, err := tx.Exec(`UPDATE wdtt_users SET vk_hash = ? WHERE password = ?`, f.want, f.pass); err != nil {
			return 0, err
		}
	}
	if err := bumpUsersRevInTx(tx); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(fixes), nil
}

func normalizeHashCSV(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, ",")
}
