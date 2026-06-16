# Changelog

Формат: каждый релиз — секция `## [X.Y.Z]`. CI подставляет её в GitHub Release автоматически.

## [1.4.17] — 2026-06-16

### Изменено
- **`blockPing`** — в API отдаётся значение из БД; `blockPingLive` показывает фактическое состояние UFW; при открытии настроек UFW синхронизируется с БД.
- **Panel JS bundle** — `csrf.js`, `axios-init.js`, `util/index.js`, `websocket.js` → один `panel-core.min.js` (esbuild, сборка в `build.sh`).
- **API restart** — страница подключений использует `/panel/api/server/restartWdttService` (как дашборд).

## [1.4.16] — 2026-06-16

### Изменено
- **Row-level SQLite (bot/admin)** — Telegram-бот и `/admin/users/*` пишут точечно (`UpsertUser`, `PatchUserDeviceBindings`, `DeleteUser`, `DeleteDevice`) вместо полного `SaveStore`.
- **Cache-bust assets** — `assets_ver` хэширует ключевые custom JS/CSS, не только `custom.min.css`.

## [1.4.15] — 2026-06-16

### Добавлено
- **SSE `/panel/api/server/events`** — push статуса дашборда (fallback на poll если EventSource недоступен).
- **Prometheus `/panel/api/server/metrics`** — `wdtt_active_users`, `wdtt_sessions`, uptime gauges.
- **Row-level SQLite на GETCONF** — `UpsertDevice` + `PatchUserDeviceBindings` вместо полного `SaveStore`.

### Изменено
- **Status collector** — кэш `loadPasswords` по `users_rev`; интервал из `dashboard_poll_sec`.
- **Users page** — автообновление по `users_rev` через SSE.

### Удалено
- **`subUpdates`** — колонка `sub_updates` (schema v12), поле из API/конфига и переводов.

## [1.4.14] — 2026-06-16

### Добавлено
- **Schema v11** — `wg_keepalive_sec`, `stats_interval_sec`, `dashboard_poll_sec`, `users_poll_sec`, `connections_poll_sec`.
- **Inbound advanced** — WG keepalive и интервал stats в модалке подключений.
- **Настройки → Общие** — интервалы опроса дашборда, пользователей и подключений.
- **Подписка** — заголовок `Profile-Home-Page-Url` для `subProfileUrl`.
- **Lazy CodeMirror** — редактор xray грузится только на странице xray.

### Изменено
- **Stats loop** — один WG dump за тик, batch `last_seen`, `[СТАТ]` по интервалу stats (не каждые 2 с).
- **Poll** — пауза при скрытой вкладке (`PagePollUtil`).
- **Подключения** — отдельные поля DTLS handshake и online timeout.
- **Синхронизация panel.db** — server подтягивает правки пользователей/устройств по `users_rev` в stats loop.

### Исправлено
- **Онлайн** — динамический TTL `server.log`; после рестарта нет «призраков» и ложных online из offline WG-пиров.
- **Admin reload** — авторизация через `X-WDTT-Admin-Token` (session key панели); без токена — только localhost.

## [1.4.13] — 2026-06-16

### Изменено
- **TLS подписки** — при заданном сертификате панели пути `subCertFile`/`subKeyFile` прописываются автоматически (если пусто).
- **Подписка** — убран «Интервал обновления подписки» и заголовок `Profile-Update-Interval` (клиент сам обновляет метрику).

## [1.4.12] — 2026-06-16

### Исправлено
- **qwdtt-ссылки** — `peer` только hostname, без `:56000` (убран `%3A56000` в URL).
- **Обновление панели** — в диалоге подставляется реальная версия вместо `#version#` (Xray — аналогично).

## [1.4.11] — 2026-06-16

### Добавлено
- **Uptime WDTT-PANEL / WDTT-SERVER** — на дашборде отдельно: процесс (systemd) и VPN-сервер (сбрасывается при in-process restart); `/health` отдаёт `uptime_sec` и `vpn_active`.

### Изменено
- **Подключения** — switch VPN переключает `enable` через inbound save, не `systemctl stop` (панель не падает в unified).
- **deploy.sh** — при переустановке сохраняется `panel.db`; seed `install-main-password.env` на чистой установке.

### Исправлено
- **Re-enable VPN** — admin HTTP активен при `enable=false`; повторное включение без `systemctl restart`.
- **Hot-reload fallback** — в unified при ошибке reload → in-process restart (создание пользователей, inbound).
- **Toast «Перезапуск VPN»** — одно уведомление «VPN-сервер перезапущен» (без дубля HttpUtil).
- **DTLS renew / MTU** — in-process restart в unified; `wdtt-mtu-rules.sh` при restart VPN.
- **stop WDTT** — заблокирован в unified (используйте выключение в Подключениях).

## [1.4.10] — 2026-06-16

### Добавлено
- **install-main-password.env** — seed главного пароля VPN при установке через `wdtt-install` / `deploy.sh` (без `-password` в systemd).

## [1.4.9] — 2026-06-16

### Добавлено
- **In-process restart VPN** — `POST /admin/restart` перезапускает DTLS/WG/admin внутри unified-процесса; панель не останавливается.
- **Inbound из panel.db** — порты, listen, MTU, admin-addr читаются из БД при старте/restart (systemd не источник правды).

### Изменено
- **wdtt.service** — `ExecStart` только `-config-dir`; без `-listen`, `-wg-port`, `-password`, `-admin-addr`.
- **deploy.sh v3.4** — минимальный unit, `install-inbound.env` для seed портов в panel.db.
- **Панель** — сохранение inbound без `systemctl restart`; «Перезапуск VPN» = in-process restart; CSRF в модалке inbound.
- **install-local.sh**, README, API.md — unified-архитектура.

### Исправлено
- Unified inbound: лимит/DNS/таймауты через hot-reload; порты/MTU через in-process restart.
- Главный пароль и пользователи больше не вызывают полный restart процесса в unified.

## [1.4.8] — 2026-06-16

### Добавлено
- **In-process restart VPN** — `POST /admin/restart` перезапускает DTLS/WG/admin внутри unified-процесса; панель не останавливается.
- **Inbound из panel.db** — порты, listen, MTU и admin-addr читаются из БД при каждом старте/restart сервера (systemd unit больше не источник правды в unified).

### Исправлено
- **Unified inbound** — смена портов, MTU, listen и admin-addr применяется через in-process restart, без `systemctl restart`.
- **wdtt.service** — ExecStart без `-listen`, `-wg-port`, `-password`, `-admin-addr`; VPN-параметры только в panel.db.

## [1.4.7] — 2026-06-15

### Исправлено
- **Unified-режим** — сохранение inbound (лимит пользователей, DNS, таймауты и т.п.) больше не переписывает `wdtt.service` и не делает `systemctl restart` (панель остаётся доступной).
- **Unified-режим** — кнопка «Перезапуск WDTT» в API выполняет hot-reload вместо полного restart процесса.
- **Подключения / inbound** — сохранение формы WDTT отправляет CSRF-токен (исправлен `403 invalid csrf token`).

## [1.4.6] — 2026-06-15

### Исправлено
- **Обновление из панели** — скачивается unified `wdtt-linux-*` вместо несуществующего `wdtt-panel-linux-*`; путь установки берётся из текущего бинарника (`wdtt` / `wdtt-app`).

## [1.4.5] — 2026-06-15

### Исправлено
- **Главный пароль** — смена пароля владельца обновляет существующую запись, а не создаёт дубликат в списке пользователей.
- **Главный пароль** — применение через hot-reload вместо полного `systemctl restart` (больше нет `500` / `signal: terminated` в unified-режиме).

## [1.4.4] — 2026-06-15

### Исправлено
- **Firewall / UFW** — при включении UFW автоматически открываются UDP-порты DTLS и WireGuard (`56000`, `56001` или из inbound).
- **Панель** — убрана кнопка «Остановить» на карточке WDTT (в unified-режиме гасит и панель).
- **Настройки Xray** — убрана дублирующая кнопка «Перезапуск Xray» (сохранение конфига уже перезапускает сервис).

## [1.4.3] — 2026-06-15

### Исправлено
- **Firewall / UFW** — `panelApiJson` на странице подключений отправляет `X-CSRF-Token`; включение UFW и правки портов больше не падают с `invalid csrf token`.

## [1.4.2] — 2026-06-15

### Исправлено
- **SSL/ACME на unified** — перезапуск панели через `wdtt.service` вместо несуществующего `wdtt-panel.service`; HTTPS применяется после выпуска сертификата.
- **Журналы** — раздельная фильтрация panel/wdtt в unified-процессе; убрано дублирование префикса `WDTT: WDTT-APP`.
- **ACME** — кнопка удаления домена в таблице acme.sh.

### Установка
- **wdtt-install v1.4.2** — убран `wdtt-panel` из summary; `SyslogIdentifier=wdtt`; DNS Xray без DoH.

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
