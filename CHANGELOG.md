# Changelog

Формат: каждый релиз — секция `## [X.Y.Z]`. CI подставляет её в GitHub Release автоматически.

## [1.4.1] — 2026-06-15

### Исправлено
- **Unified `wdtt` не стартовал из systemd** — убран ранний `flag.Parse()` в `cmd/wdtt/main.go`; флаги `-listen`, `-password` и др. снова обрабатываются сервером.
- **wdtt-install** — исправлена загрузка `wdtt-linux-amd64` из GitHub Releases (regex URL).

## [1.4.0] — 2026-06-15

### Добавлено
- **Единый бинарник `wdtt`** — VPN-сервер и веб-панель в одном процессе (`/usr/local/bin/wdtt`), одна SQLite `panel.db` без гонок между процессами.
- Скрипт `install-local.sh` для деплоя на текущий сервер (сборка unified → `/usr/local/bin/wdtt` → restart).
- Флаг `-version` у unified-бинарника.
- Поле `password` в списке онлайн-устройств (`server.log`) для корректного отображения в панели.

### Исправлено
- **Онлайн главного пароля** — orphan-устройства без записи в `wdtt_user_devices` учитываются в памяти; онлайн по трафику WG (rx+tx), счётчики добираются из `wg show dump` если userspace IPC их не отдаёт.
- **Стабильность подключения** — восстановлено поведение GETCONF/relay из v1.3.4 (`flushTraffic`, post-GETCONF read).
- Главный пароль больше **не копит устройства** в панели (безлимит, без привязки в БД).
- CSRF-токен при загрузке страницы для существующих сессий.
- Admin API для сохранения пользователей/устройств через единую БД.

### Установка
- **wdtt-install v1.4.0** — unified `wdtt-linux-*`, CLI → `/usr/local/bin/wdtt`, демон → `/usr/local/bin/wdtt-app`.
- `deploy.sh` v3.3 — unified-бинарник, `-admin-addr 127.0.0.1:2861`.

### Артефакты релиза
- `wdtt-linux-amd64` / `wdtt-linux-arm64` — **использовать эти**.

## [1.3.4] — 2026-06-14

- Исправлен `online_timeout_sec`, deploy MTU/nft, ссылка без `ps`, expiry.
