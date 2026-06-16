# Changelog

Формат: каждый релиз — секция `## [X.Y.Z]`. CI подставляет её в GitHub Release автоматически.

### Исправлено
- **wdtt-install v1.4.28** — если pinned release ещё не на GitHub, сборка из git tag (не fallback на older `latest`).

## [1.4.27] — 2026-06-16

### Исправлено
- **Старт unified** — `BootstrapDB()` синхронно до `server.Run()`: panel.db и seed `wdtt_users` готовы до WRAP (гонка с goroutine panel).
- **wdtt-install v1.4.27** — `seed` env до `build_wdtt`; проверка `/health` после первого `systemctl restart`.

## [1.4.26] — 2026-06-16

### Исправлено
- **wdtt-install v1.4.26** — fresh install качает `v${INSTALLER_VERSION}` (fallback на latest), чтобы не попасть на старый бинарник при гонке с GitHub Releases.
- **Тесты** — регрессия seed `wdtt_users` и загрузки `main_password` без строк users.

## [1.4.25] — 2026-06-16

### Исправлено
- **Первый install** — seed создаёт строку главного пароля в `wdtt_users` (исправлен `[WRAP] нет активных паролей для WRAP` при чистой установке).
- **Server load DB** — загрузка `main_password` из `wdtt_global`, даже если `wdtt_users` ещё пуст (race panel/server при старте).

## [1.4.24] — 2026-06-16

### Исправлено
- **wdtt-install v1.4.24** — прямой download `releases/latest/download/wdtt-linux-*` (без GitHub API); `git reset --hard` при обновлении clone; fallback — `go build ./cmd/wdtt` без `build.sh`/`npx`.
- **deploy.sh** — тот же прямой download URL для Releases.

## [1.4.23] — 2026-06-16

### Исправлено
- **bundle-panel-core.sh** — без `esbuild`/`npx` использует уже закоммиченный `panel-core.min.js` (install на чистом VPS не падает).
- **wdtt-install v1.4.23** — предупреждение при неудачном download; fallback-сборка с `WDTT_SKIP_BUNDLE=1`; portable парсинг GitHub API без `grep -P`.

## [1.4.22] — 2026-06-16

### Изменено
- **deploy.sh v3.5** — скачивание `wdtt-linux-*` из GitHub Releases (`WDTT_VERSION` / latest); отключение legacy `wdtt-panel.service`; cleanup `wdtt-panel`/`wdtt-app` бинарников.
- **wdtt-install v1.4.22** — актуальный help (`update --version v1.4.22`); синхронизация с deploy.

## [1.4.21] — 2026-06-16

### Удалено
- **Legacy API** — `POST /panel/api/service/{wdtt|xray}/{restart|stop|start}`; мёртвая страница `panel.html`.

### Изменено
- **docs/SERVER.md** — unified `wdtt-app`, актуальная сборка и schema v12.
- **docs/API.md** — restart через `/panel/api/server/restartWdttService` и `restartXrayService`.

## [1.4.20] — 2026-06-16

### Изменено
- **Panel user CRUD** — `createUser`, смена главного пароля и сброс трафика пишут в SQLite точечно (`UpsertUser`, `SetMainPassword`, `ResetUserTraffic`) вместо полного `SaveStore`.

## [1.4.19] — 2026-06-16

### Исправлено
- **Журнал «Управление»** — panel-сообщения пишутся в `/etc/wdtt/panel.log` и читаются оттуда (без DTLS-спама из VPN journal).
- **Журнал WDTT** — схлопывание повторяющихся строк `(×N)`; меньший объём выборки из journal.
- **UI журнала** — возвращена цветная разметка (как раньше); SSE/poll по-прежнему на паузе при открытой модалке.

## [1.4.18] — 2026-06-16

### Исправлено
- **Журнал (дашборд)** — `journalctl -o cat` (быстрее JSON); cap 1500 строк при фильтре panel/wdtt в unified; xray access log через `tail` вместо полного скана.
- **Xray access log** — `tail` последних строк вместо полного скана 42+ MB файла.
- **UI журнала** — plain-text `<pre>` вместо `v-html`; пауза SSE/poll CPU-графика пока модалка открыта; отложенный рендер через `requestAnimationFrame`.

## [1.4.17] — 2026-06-16

### Сводка с v1.4.13
Четыре релиза подряд (1.4.14 → 1.4.17): производительность stats/poll, SSE и метрики, точечные записи в SQLite вместо полного `SaveStore`, удаление `subUpdates`, исправление `blockPing`, бандл panel JS.

### Изменено
- **`blockPing`** — в API отдаётся значение из БД (`blockPing`); фактическое состояние UFW — в `blockPingLive`; при GET `/panel/setting/all` UFW синхронизируется с БД при расхождении; предупреждение в UI если live ≠ сохранённое.
- **Panel JS bundle** — `csrf.js`, `axios-init.js`, `util/index.js`, `websocket.js` → один `panel-core.min.js` (~16 KB, esbuild); скрипт `scripts/bundle-panel-core.sh`, вызов из `build.sh`; `assets_ver` хэширует бандл.
- **API restart** — страница «Подключения» вызывает `/panel/api/server/restartWdttService` (как дашборд), вместо legacy `/panel/api/service/wdtt/restart`.

## [1.4.16] — 2026-06-16

### Изменено
- **Row-level SQLite (bot/admin)** — Telegram-бот и `/admin/users/*` пишут точечно через `pkg/paneldb`: `UpsertUser`, `RenameUserPassword`, `SetUserDeactivated`, `DeleteUser`, `DeleteDevice`, `PatchUserDeviceBindings` — вместо полного `SaveStore` на каждое действие.
- **Cache-bust assets** — `assets_ver` хэширует `custom.min.css`, `panel-core.min.js`, `wdtt-share.js`.

## [1.4.15] — 2026-06-16

### Добавлено
- **SSE `/panel/api/server/events`** — push событий `status` (дашборд) и `users_rev` (страница пользователей); клиент `PanelEventsClient` в `websocket.js` как `window.wsClient`; poll остаётся fallback.
- **Prometheus `/panel/api/server/metrics`** — gauges `wdtt_active_users`, `wdtt_sessions`, uptime panel/VPN.
- **Row-level SQLite на GETCONF** — `UpsertDevice` + `PatchUserDeviceBindings` вместо полного `SaveStore`.
- **Кэш паролей** — `loadPasswordsCached()` по `users_rev` в status collector.

### Изменено
- **Status collector** — интервал из `dashboard_poll_sec`; меньше обращений к SQLite.
- **Users page** — автообновление списка по SSE `users_rev`.

### Удалено
- **`subUpdates`** — колонка `sub_updates` (миграция schema **v12**), поле из API/конфига, UI и всех файлов переводов.

## [1.4.14] — 2026-06-16

### Добавлено
- **Schema v11** — поля `wg_keepalive_sec`, `stats_interval_sec`, `dashboard_poll_sec`, `users_poll_sec`, `connections_poll_sec` в `panel_config` / inbound.
- **Inbound advanced** — WG PersistentKeepalive и интервал stats в модалке подключений.
- **Настройки → Общие** — интервалы опроса дашборда, пользователей и подключений (секунды).
- **Подписка** — HTTP-заголовок `Profile-Home-Page-Url` из `subProfileUrl`.
- **Lazy CodeMirror** — `codemirror-loader.js`; редактор xray/config грузится только на странице xray.

### Изменено
- **Stats loop** — один `wg show` dump за тик; batch-обновление `last_seen`; лог `[СТАТ]` по `stats_interval_sec`, не каждые 2 с.
- **Poll** — пауза опроса при скрытой вкладке (`PagePollUtil.visibilityPause`).
- **Подключения** — раздельные поля `online_timeout_sec` и `handshake_timeout_sec` (DTLS handshake vs online TTL).
- **Синхронизация panel.db** — server в stats loop вызывает `syncPanelDeviceEditsLocked()` по `users_rev` (правки из панели → WG peers).

### Исправлено
- **Онлайн** — динамический TTL записей `server.log`; после рестарта нет «призраков» и ложного online у offline WG-пиров; удалён мёртвый `syncOnlineFromWGPeers()`.
- **Admin reload** — `POST /admin/reload` требует `X-WDTT-Admin-Token` (session key панели); без токена — только с localhost.

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
