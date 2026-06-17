package panel

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
	"github.com/ildarmaga/wdtt/pkg/vkhash"
)

var vkCreatorMu sync.Mutex

const vkAuthCookieName = "remixsid"

type vkCookieEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type vkCreatorSession struct {
	Password  string `json:"password,omitempty"`
	JoinLink  string `json:"join_link"`
	VkHash    string `json:"vk_hash"`
	CallID    string `json:"call_id,omitempty"`
	StartedAt int64  `json:"started_at"`
	Finishing bool   `json:"finishing,omitempty"`
}

func resolveUserPassword(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("пароль пуст")
	}
	db, err := loadPasswords()
	if err != nil {
		return "", err
	}
	if entry, ok := db.Passwords[key]; ok && entry != nil {
		return key, nil
	}
	for pass, entry := range db.Passwords {
		if entry != nil && maskPassword(pass) == key {
			return pass, nil
		}
	}
	return "", fmt.Errorf("пользователь не найден")
}

func vkCreatorStatus() map[string]interface{} {
	cookiesOK, cookieHint := vkCookiesStatus()
	sessions, _ := listVKCreatorSessions()
	active := make([]map[string]interface{}, 0, len(sessions))
	for _, s := range sessions {
		running := vkCallAlive(s.JoinLink)
		if s.Finishing && !running {
			_ = dropVKCreatorSession(s)
			continue
		}
		active = append(active, map[string]interface{}{
			"password":   s.Password,
			"join_link":  s.JoinLink,
			"vk_hash":    s.VkHash,
			"call_id":    s.CallID,
			"running":    running,
			"finishing":  s.Finishing,
			"started_at": s.StartedAt,
		})
	}
	return map[string]interface{}{
		"cookies_ok":   cookiesOK,
		"cookies_hint": cookieHint,
		"sessions":     active,
	}
}

func vkCookiesStatus() (ok bool, hint string) {
	data, err := loadVKCookiesFromStore()
	if err != nil || len(data) == 0 {
		return false, "Войдите в VK через кнопку ниже или загрузите cookies вручную."
	}
	if err := validateVKCookiesJSON(data); err != nil {
		return false, err.Error()
	}
	return true, "VK cookies настроены (remixsid найден)."
}

func validateVKCookiesJSON(data []byte) error {
	var cookies []vkCookieEntry
	if err := json.Unmarshal(data, &cookies); err != nil {
		return fmt.Errorf("неверный JSON cookies: %w", err)
	}
	if len(cookies) == 0 {
		return fmt.Errorf("файл cookies пуст")
	}
	for _, c := range cookies {
		if c.Name == vkAuthCookieName && strings.TrimSpace(c.Value) != "" {
			return nil
		}
	}
	return fmt.Errorf("в cookies нет %s — войдите в VK через панель", vkAuthCookieName)
}

func saveVKCookies(raw []byte) error {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return fmt.Errorf("cookies пусты")
	}
	s := string(raw)
	if strings.HasPrefix(s, "remixsid=") || (strings.Contains(s, ";") && !strings.HasPrefix(s, "[")) {
		return saveVKCookieString(s)
	}
	if err := validateVKCookiesJSON(raw); err != nil {
		return err
	}
	return saveVKCookiesToStore(raw)
}

func saveVKCookieString(cookieStr string) error {
	parts := strings.Split(cookieStr, ";")
	var cookies []vkCookieEntry
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		cookies = append(cookies, vkCookieEntry{Name: strings.TrimSpace(kv[0]), Value: strings.TrimSpace(kv[1])})
	}
	data, err := json.Marshal(cookies)
	if err != nil {
		return err
	}
	return saveVKCookies(data)
}

func clearVKCookies() error {
	return clearVKCookiesFromStore()
}

func createVKCall(password string) (joinLink, hash string, sess vkCreatorSession, err error) {
	vkCreatorMu.Lock()
	defer vkCreatorMu.Unlock()

	if ok, hint := vkCookiesStatus(); !ok {
		return "", "", sess, fmt.Errorf("%s", hint)
	}
	cookieHeader, err := vkCookieHeaderFromStore()
	if err != nil {
		return "", "", sess, err
	}
	realPass := strings.TrimSpace(password)
	if realPass != "" {
		if p, err := resolveUserPassword(realPass); err == nil {
			realPass = p
		}
		if n, err := countVKCreatorSessionsForPassword(realPass); err != nil {
			return "", "", sess, err
		} else if n >= vkhash.Max {
			return "", "", sess, fmt.Errorf("не более %d звонков на профиль — завершите лишний", vkhash.Max)
		}
	}

	created, err := vkCreateCallLink(cookieHeader)
	if err != nil {
		return "", "", sess, err
	}
	joinLink = created.JoinLink
	hash = vkhash.Normalize(joinLink)
	if hash == "" {
		return "", "", sess, fmt.Errorf("не удалось извлечь vk hash из %q", joinLink)
	}

	sess = vkCreatorSession{
		Password:  realPass,
		JoinLink:  joinLink,
		VkHash:    hash,
		CallID:    created.CallID,
		StartedAt: time.Now().Unix(),
	}
	if err := insertVKCreatorSession(sess); err != nil {
		return "", "", sess, fmt.Errorf("не удалось сохранить звонок: %w", err)
	}
	return joinLink, hash, sess, nil
}

func stopVKCreatorSession(password, callID string) error {
	vkCreatorMu.Lock()
	defer vkCreatorMu.Unlock()
	return stopVKCreatorSessionLocked(password, callID)
}

func finishVKCalls(cookieHeader string, sessions []vkCreatorSession) error {
	var firstErr error
	for _, s := range sessions {
		if !paneldb.ValidVKCallID(s.CallID) {
			continue
		}
		if err := vkForceFinishCall(cookieHeader, s.CallID); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("завершить звонок %s: %w", s.VkHash, err)
		}
	}
	return firstErr
}

func stopVKCreatorSessionLocked(password, callID string) error {
	password = strings.TrimSpace(password)
	callID = strings.TrimSpace(callID)
	if password == "" && callID == "" {
		return fmt.Errorf("укажите call_id или профиль")
	}
	if password != "" {
		if p, err := resolveUserPassword(password); err == nil {
			password = p
		}
	}
	toFinish, err := findVKCreatorSessions(password, callID)
	if err != nil {
		return err
	}
	if len(toFinish) == 0 {
		return fmt.Errorf("звонок не найден")
	}
	cookieHeader, err := vkCookieHeaderFromStore()
	if err != nil {
		return err
	}
	if err := finishVKCalls(cookieHeader, toFinish); err != nil {
		return err
	}
	if callID != "" {
		return markVKCreatorSessionFinishing("", callID)
	}
	return markVKCreatorSessionFinishing(password, "")
}

func applyVKHashToUser(password, hash string) error {
	realPass, err := resolveUserPassword(password)
	if err != nil {
		return err
	}
	hash = vkhash.Normalize(hash)
	if hash == "" {
		return fmt.Errorf("hash пуст")
	}
	db, err := loadPasswords()
	if err != nil {
		return err
	}
	entry, ok := db.Passwords[realPass]
	if !ok || entry == nil {
		return fmt.Errorf("пользователь не найден")
	}
	active := !entry.IsDeactivated
	req := userAPIReq{
		Password:    realPass,
		Comment:     entry.Comment,
		ExpiresAt:   entry.ExpiresAt,
		TotalGB:     bytesToGB(entry.TotalBytes),
		MaxDownMBps: entry.MaxDownMBps,
		MaxUpMBps:   entry.MaxUpMBps,
		Active:      &active,
		Ports:       entry.Ports,
		VkHash:      mergeVKHashes(entry.VkHash, hash),
		MaxDevices:  entry.MaxDevices,
	}
	return updateUser(realPass, realPass, req, false)
}

func removeVKHashFromUser(password, hash string) error {
	realPass, err := resolveUserPassword(password)
	if err != nil {
		return err
	}
	toRemove := vkhash.Parse(hash, 0)
	if len(toRemove) == 0 {
		return nil
	}
	db, err := loadPasswords()
	if err != nil {
		return err
	}
	entry, ok := db.Passwords[realPass]
	if !ok || entry == nil {
		return nil
	}
	remaining := subtractVKHashes(entry.VkHash, toRemove)
	if remaining == entry.VkHash {
		return nil
	}
	active := !entry.IsDeactivated
	req := userAPIReq{
		Password:    realPass,
		Comment:     entry.Comment,
		ExpiresAt:   entry.ExpiresAt,
		TotalGB:     bytesToGB(entry.TotalBytes),
		MaxDownMBps: entry.MaxDownMBps,
		MaxUpMBps:   entry.MaxUpMBps,
		Active:      &active,
		Ports:       entry.Ports,
		VkHash:      remaining,
		MaxDevices:  entry.MaxDevices,
	}
	return updateUser(realPass, realPass, req, false)
}

func subtractVKHashes(existing string, remove []string) string {
	removeSet := make(map[string]struct{}, len(remove))
	for _, h := range remove {
		removeSet[h] = struct{}{}
	}
	var out []string
	for _, p := range vkhash.Parse(existing, 0) {
		if _, drop := removeSet[p]; drop {
			continue
		}
		out = append(out, p)
		if len(out) >= vkhash.Max {
			break
		}
	}
	return strings.Join(out, ",")
}

func mergeVKHashes(existing, added string) string {
	merged := vkhash.Normalize(existing)
	add := vkhash.Normalize(added)
	if add == "" {
		return merged
	}
	if merged == "" {
		return add
	}
	parts := append(vkhash.Parse(merged, 0), vkhash.Parse(add, 0)...)
	seen := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
		if len(out) >= vkhash.Max {
			break
		}
	}
	return strings.Join(out, ",")
}
