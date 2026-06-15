package panel

import "testing"

func TestIsPanelLogMessage(t *testing.T) {
	panelMsgs := []string{
		"[PANEL] веб-панель запущена в том же процессе",
		"WDTT Panel: http://0.0.0.0:2860/panel",
		"panel db: migrated passwords.json",
		"[watchdog] WDTT активен, Xray выключен — запускаем",
		"[panel] hot-reload не удался (timeout)",
		"xray hot-reload (redirect-in): timeout — перезапуск сервиса",
	}
	for _, msg := range panelMsgs {
		if !isPanelLogMessage(msg) {
			t.Fatalf("expected panel log: %q", msg)
		}
	}

	wdttMsgs := []string{
		"[СТАТ] Пользователей: 0 | Сессий: 0 | Всего: 0 | NAT: MASQUERADE",
		"[WG] Запущен на порту 56001",
		"[DTLS] Handshake OK from 1.2.3.4 in 120ms",
		"[DB] users loaded from /etc/wdtt/panel.db (sqlite primary)",
		"[ADMIN] HTTP 127.0.0.1:2861 (/health, POST /admin/reload)",
	}
	for _, msg := range wdttMsgs {
		if isPanelLogMessage(msg) {
			t.Fatalf("expected wdtt log, got panel: %q", msg)
		}
	}
}

func TestFilterUnifiedLogLines(t *testing.T) {
	lines := []string{
		"2026/06/15 09:39:39 INFO - WDTT Panel: WDTT Panel: http://127.0.0.1:2860/panel",
		"2026/06/15 09:39:39 INFO - WDTT: [СТАТ] Пользователей: 0 | Сессий: 0",
		"2026/06/15 09:39:37 INFO - WDTT: [WG] Запущен на порту 56001",
	}
	panel := filterUnifiedLogLines(lines, "panel", 10)
	if len(panel) != 1 {
		t.Fatalf("panel filter: got %d lines, want 1", len(panel))
	}
	wdtt := filterUnifiedLogLines(lines, "wdtt", 10)
	if len(wdtt) != 2 {
		t.Fatalf("wdtt filter: got %d lines, want 2", len(wdtt))
	}
}
