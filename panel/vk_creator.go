package panel

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ildarmaga/wdtt/pkg/vkhash"
)

const (
	vkCreatorVersion     = "v0.3.6"
	vkCreatorCLIAsset    = "whitelist-bypass-cli-linux-x64.zip"
	vkCreatorBinName     = "headless-vk-creator"
	vkAuthCookieName     = "remixsid"
	vkCreatorSpawnWait   = 90 * time.Second
	vkCreatorPollEvery   = 500 * time.Millisecond
)

var (
	vkSecretsDir     = filepath.Join(wdttConfigDir, "secrets")
	vkCookiesPath    = filepath.Join(vkSecretsDir, "cookies-vk.json")
	vkCreatorBinDir  = filepath.Join(wdttConfigDir, "bin")
	vkCreatorBinPath = filepath.Join(vkCreatorBinDir, vkCreatorBinName)
	vkSessionsDir    = filepath.Join(wdttConfigDir, "vk-sessions")
	vkSessionsFile   = filepath.Join(wdttConfigDir, "vk-creator-sessions.json")

	vkCreatorMu sync.Mutex
)

type vkCookieEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type vkCreatorSession struct {
	Password  string `json:"password,omitempty"`
	JoinLink  string `json:"join_link"`
	VkHash    string `json:"vk_hash"`
	PID       int    `json:"pid"`
	StartedAt int64  `json:"started_at"`
	LogPath   string `json:"log_path,omitempty"`
}

type vkCreatorSessionStore struct {
	Sessions []vkCreatorSession `json:"sessions"`
}

func vkCreatorReleaseURL() string {
	return fmt.Sprintf("https://github.com/kulikov0/whitelist-bypass/releases/download/%s/%s", vkCreatorVersion, vkCreatorCLIAsset)
}

func vkCreatorStatus() map[string]interface{} {
	cookiesOK, cookieHint := vkCookiesStatus()
	binaryOK := vkCreatorBinaryReady()
	sessions := loadVKCreatorSessions()
	active := make([]map[string]interface{}, 0, len(sessions.Sessions))
	for _, s := range sessions.Sessions {
		running := processAlive(s.PID)
		active = append(active, map[string]interface{}{
			"password":   s.Password,
			"join_link":  s.JoinLink,
			"vk_hash":    s.VkHash,
			"pid":        s.PID,
			"running":    running,
			"started_at": s.StartedAt,
		})
	}
	return map[string]interface{}{
		"cookies_ok":      cookiesOK,
		"cookies_hint":    cookieHint,
		"binary_ok":       binaryOK,
		"binary_path":     vkCreatorBinPath,
		"creator_version": strings.TrimPrefix(vkCreatorVersion, "v"),
		"sessions":        active,
	}
}

func vkCookiesStatus() (ok bool, hint string) {
	data, err := os.ReadFile(vkCookiesPath)
	if err != nil {
		return false, "Загрузите cookies-vk.json из WhitelistBypass Creator (кнопка Export Cookies)."
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
	return fmt.Errorf("в cookies нет %s — войдите в VK в Creator и экспортируйте cookies", vkAuthCookieName)
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
	if err := os.MkdirAll(vkSecretsDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(vkCookiesPath, raw, 0600)
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
	if err := os.Remove(vkCookiesPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func vkCreatorBinaryReady() bool {
	info, err := os.Stat(vkCreatorBinPath)
	return err == nil && !info.IsDir() && info.Mode()&0111 != 0
}

func ensureVKCreatorBinary() error {
	if vkCreatorBinaryReady() {
		return nil
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("headless-vk-creator поддерживается только на Linux-сервере")
	}
	if err := os.MkdirAll(vkCreatorBinDir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "wb-cli-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	resp, err := http.Get(vkCreatorReleaseURL())
	if err != nil {
		tmp.Close()
		return fmt.Errorf("скачивание %s: %w", vkCreatorVersion, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf("скачивание %s: HTTP %d", vkCreatorVersion, resp.StatusCode)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return extractVKCreatorFromZip(tmpPath)
}

func extractVKCreatorFromZip(zipPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if filepath.Base(f.Name) != vkCreatorBinName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(vkCreatorBinPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	return fmt.Errorf("%s не найден в %s", vkCreatorBinName, vkCreatorCLIAsset)
}

func createVKCall(password string) (joinLink, hash string, sess vkCreatorSession, err error) {
	vkCreatorMu.Lock()
	defer vkCreatorMu.Unlock()

	if ok, hint := vkCookiesStatus(); !ok {
		return "", "", sess, fmt.Errorf("%s", hint)
	}
	if err := ensureVKCreatorBinary(); err != nil {
		return "", "", sess, err
	}
	if password != "" {
		_ = stopVKCreatorSessionLocked(password)
	}

	if err := os.MkdirAll(vkSessionsDir, 0700); err != nil {
		return "", "", sess, err
	}
	linkFile, err := os.CreateTemp(vkSessionsDir, "vk-link-*.txt")
	if err != nil {
		return "", "", sess, err
	}
	linkPath := linkFile.Name()
	linkFile.Close()

	logPath := filepath.Join(vkSessionsDir, fmt.Sprintf("creator-%d.log", time.Now().Unix()))
	logF, err := os.Create(logPath)
	if err != nil {
		os.Remove(linkPath)
		return "", "", sess, err
	}

	args := []string{
		"--cookies", vkCookiesPath,
		"--write-file", linkPath,
		"--resources", "default",
	}
	cmd := exec.Command(vkCreatorBinPath, args...)
	cmd.Stdout = logF
	cmd.Stderr = logF
	if err := cmd.Start(); err != nil {
		logF.Close()
		os.Remove(linkPath)
		return "", "", sess, fmt.Errorf("запуск creator: %w", err)
	}

	joinLink, err = waitForVKJoinLink(linkPath, vkCreatorSpawnWait)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		logF.Close()
		os.Remove(linkPath)
		return "", "", sess, err
	}
	hash = vkhash.Normalize(joinLink)
	if hash == "" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		logF.Close()
		os.Remove(linkPath)
		return "", "", sess, fmt.Errorf("не удалось извлечь vk hash из %q", joinLink)
	}

	sess = vkCreatorSession{
		Password:  password,
		JoinLink:  joinLink,
		VkHash:    hash,
		PID:       cmd.Process.Pid,
		StartedAt: time.Now().Unix(),
		LogPath:   logPath,
	}
	store := loadVKCreatorSessions()
	store.Sessions = append(filterVKSessions(store.Sessions, password), sess)
	_ = saveVKCreatorSessions(store)
	go watchVKCreatorProcess(cmd, logF, password)
	return joinLink, hash, sess, nil
}

func waitForVKJoinLink(path string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
			if line != "" && strings.Contains(line, "/call/join/") {
				return line, nil
			}
		}
		time.Sleep(vkCreatorPollEvery)
	}
	return "", fmt.Errorf("creator не записал join link за %s — проверьте cookies и лог", timeout)
}

func watchVKCreatorProcess(cmd *exec.Cmd, logF *os.File, password string) {
	_ = cmd.Wait()
	if logF != nil {
		logF.Close()
	}
	vkCreatorMu.Lock()
	defer vkCreatorMu.Unlock()
	store := loadVKCreatorSessions()
	out := store.Sessions[:0]
	for _, s := range store.Sessions {
		if s.Password == password && s.PID == cmd.Process.Pid {
			continue
		}
		out = append(out, s)
	}
	store.Sessions = out
	_ = saveVKCreatorSessions(store)
}

func stopVKCreatorSession(password string) error {
	vkCreatorMu.Lock()
	defer vkCreatorMu.Unlock()
	return stopVKCreatorSessionLocked(password)
}

func stopVKCreatorSessionLocked(password string) error {
	store := loadVKCreatorSessions()
	changed := false
	out := store.Sessions[:0]
	for _, s := range store.Sessions {
		if password != "" && s.Password != password {
			out = append(out, s)
			continue
		}
		if password == "" || s.Password == password {
			if s.PID > 0 && processAlive(s.PID) {
				_ = syscall.Kill(s.PID, syscall.SIGTERM)
			}
			changed = true
			continue
		}
		out = append(out, s)
	}
	if !changed && password != "" {
		return fmt.Errorf("активная VK-сессия не найдена")
	}
	store.Sessions = out
	return saveVKCreatorSessions(store)
}

func filterVKSessions(in []vkCreatorSession, password string) []vkCreatorSession {
	if password == "" {
		return in
	}
	out := make([]vkCreatorSession, 0, len(in))
	for _, s := range in {
		if s.Password != password {
			out = append(out, s)
		}
	}
	return out
}

func loadVKCreatorSessions() vkCreatorSessionStore {
	var store vkCreatorSessionStore
	data, err := os.ReadFile(vkSessionsFile)
	if err != nil {
		return store
	}
	_ = json.Unmarshal(data, &store)
	store.Sessions = pruneDeadVKSessions(store.Sessions)
	return store
}

func saveVKCreatorSessions(store vkCreatorSessionStore) error {
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(vkSessionsFile, data, 0600)
}

func pruneDeadVKSessions(in []vkCreatorSession) []vkCreatorSession {
	out := make([]vkCreatorSession, 0, len(in))
	for _, s := range in {
		if s.PID > 0 && !processAlive(s.PID) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func applyVKHashToUser(password, hash string) error {
	password = strings.TrimSpace(password)
	hash = vkhash.Normalize(hash)
	if password == "" || hash == "" {
		return fmt.Errorf("пароль и hash обязательны")
	}
	db, err := loadPasswords()
	if err != nil {
		return err
	}
	entry, ok := db.Passwords[password]
	if !ok || entry == nil {
		return fmt.Errorf("пользователь не найден")
	}
	active := !entry.IsDeactivated
	req := userAPIReq{
		Password:    password,
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
	return updateUser(password, password, req, false)
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

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
