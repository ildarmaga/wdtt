package paneldb

import (
	"database/sql"
	"fmt"
	"strings"
)

// ReconcileVKHashesSync подтягивает живые vk_calls в wdtt_users.vk_hash и
// снимает только finishing-хеши creator'а.
//
// Ручные хеши (есть в профиле, нет строк в vk_calls) не трогаем — иначе
// сохранение из модалки затиралось при следующем списке пользователей
// (github.com/ildarmaga/wdtt/issues/23).
func ReconcileVKHashesSync(db *sql.DB) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("nil db")
	}
	activeByPass := map[string][]string{}
	finishingByPass := map[string]map[string]bool{}
	crows, err := db.Query(`
		SELECT password, vk_hash, finishing FROM vk_calls
		WHERE TRIM(vk_hash) != ''
		ORDER BY password, started_at ASC`)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	for crows.Next() {
		var pass, h string
		var finishing int
		if err := crows.Scan(&pass, &h, &finishing); err != nil {
			crows.Close()
			return 0, err
		}
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if finishing != 0 {
			if finishingByPass[pass] == nil {
				finishingByPass[pass] = map[string]bool{}
			}
			finishingByPass[pass][h] = true
			continue
		}
		list := activeByPass[pass]
		dup := false
		for _, x := range list {
			if x == h {
				dup = true
				break
			}
		}
		if !dup {
			activeByPass[pass] = append(list, h)
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
		want := mergeVKHashReconcile(have, activeByPass[pass], finishingByPass[pass])
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

// mergeVKHashReconcile: keep manual hashes, drop finishing creator hashes, add active.
func mergeVKHashReconcile(have string, active []string, finishing map[string]bool) string {
	parts := strings.Split(normalizeHashCSV(have), ",")
	out := make([]string, 0, len(parts)+len(active))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		if finishing[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, h := range active {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	return strings.Join(out, ",")
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
