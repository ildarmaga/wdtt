# Changelog

Формат: каждый релиз — секция `## [X.Y.Z]`. CI подставляет её в GitHub Release автоматически.

## [1.4.65] — 2026-08-02

### Исправлено — data-loss / sync
- Смена пароля пользователя из модалки (всегда с `device_ids`) снова делает `RenameUserPassword` + refresh WRAP — старый пароль больше не остаётся в SQLite.
- Сброс трафика: flush не перезаписывает нули, пока `users_rev` на диске впереди applied (`errTrafficFlushFenced`).
- Update всегда применяет `vk_hash` (в т.ч. очистку) через `vkhash.Normalize`; выкл. своих портов очищает `ports`.
- VK Creator create+apply возвращает merged CSV хешей; модалка не затирает мультихеш одним новым.
- Main link / link_main учитывают `vk_hash`, ports и sub_url; bot `ports_def` берёт порты из inbound; HotOnly при fail hot-reload возвращает ошибку; JSON preview включает `hash`.

## [1.4.64] — 2026-08-02

### Исправлено
- **Версия Xray в панели**: ответ GitHub ~2MB обрезался лимитом 1MB → ложное `GitHub API HTTP 200`. Лимит 8MB.

## [1.4.63] — 2026-07-30

### Исправлено
- **Версия Xray в панели**: при GitHub rate limit / ошибке API больше не показывается `json: cannot unmarshal object into Go value of type []panel.XrayRelease` — читается `message` из ответа и показывается понятная ошибка.

## [1.4.62] — 2026-07-26

### Добавлено — маскировка RTP «Видео» (PT 96)
- WRAP принимает OPUS PT 111 и VP8-like PT 96 (как Android / desktop «Видео»).
- На первом пакете сервер зеркалит PT клиента на обратном пути.
- Нужно для desktop **PWDTT Client v0.3.251** и Android с режимом «Видео».

### Обновлено
- **PWDTT Client v0.3.251** — настройки «Маскировка» Аудио/Видео.

## [1.4.61] — 2026-07-21

### Изменено — VK `vk.com` → `vk.ru` (как anton48 builds 169–171)
- VK Creator cookie API: `login.vk.ru` / `api.vk.ru`, Origin `vk.ru`.
- `vkhash` + share: join-url → `https://vk.ru/call/join/…`, парсер accept-both (`vk.com` и `vk.ru`).
- Password-login flow пока на `vk.com` (отдельный риск; cookie/creator путь уже на `.ru`).

## [1.4.60] — 2026-07-05

### Исправлено
- **Сброс трафика**: `POST /admin/reload` после «Сбросить» больше не подмешивает старые ↑↓ из RAM сервера обратно в panel.db (fix регрессии v1.4.59 в hot-reload path).

## [1.4.59] — 2026-07-03

### Исправлено
- **Сброс трафика**: после «Сбросить» в панели счётчик ↑↓ больше не восстанавливается из RAM сервера
  (merge in-memory traffic при `users_rev` sync убран — panel.db authoritative).

### Обновлено
- **PWDTT Client v0.3.90** — проверка обновлений, VK login (remixsid), fix скролла в настройках Windows.

## [1.4.58] — 2026-06-24

### Обновлено
- **PWDTT Client v0.3.45** — fix VK auth: откат okcdn `anonymLogin` к v0.3.40 (version:2 без auth_token); toggle OFF = anonymous VK Calls, toggle ON = cookies only.

## [1.4.57] — 2026-06-24

### Обновлено
- **PWDTT Client v0.3.44** — anonymous по умолчанию (как v0.3.41); тумблер «VK cookies» — явный opt-in, исправлен UI toggle.

## [1.4.56] — 2026-06-24

### Обновлено
- **PWDTT Client v0.3.43** — cookie-auth снова по умолчанию при наличии `cookies-vk.json` (fix регрессии v0.3.42); fail-fast на VK flood / broken token вместо captcha-спама.

## [1.4.55] — 2026-06-24

### Исправлено
- **Домен панели** — поле можно оставить пустым (как в 3x-ui): значение больше не подставляется автоматически из SSL-сертификата при сохранении настроек. Домен из сертификата по-прежнему используется только для ссылок/ACME, если поле пустое.

## [1.4.52] — 2026-06-21

### Исправлено
- **CPU в дашборде панели** — расчёт приведён к тому же принципу, что в 3x-ui: `iowait` (ожидание диска) больше не считается занятостью CPU; добавлено сглаживание EMA (α=0.3). Раньше при забитом диске gauge и CPU History показывали ~100%, тогда как 3x-ui на том же VPS — ~60%.

## [1.4.51] — 2026-06-20

### Исправлено
- **Массовые 16-секундные обрывы TURN-воркеров (регрессия v1.4.50).** Эхо-ответ `0xFF` ставил `SetWriteDeadline(5s)` на общий `clientConn`, и этот дедлайн протекал на горутину «WG → Клиент», которая пишет обратный трафик в тот же conn без своего дедлайна. Клиент шлёт keepalive раз в 10s → дедлайн истекал через 5s → в окне 5–10s записи обратного трафика падали по таймауту → сервер рвал DTLS-сессию, а клиент видел `EOF` и логировал это как «VK обновил relay». Отсюда постоянная переаллокация воркеров на v1.4.50, которой нет на v1.4.49.
  - Эхо `0xFF` сохранено (нужно старым клиентам для consent-freshness), но **без** `SetWriteDeadline`: 1-байтовая DTLS-запись поверх UDP не блокируется, дедлайн не нужен, а обратный трафик больше не рвётся.

## [1.4.50] — 2026-06-19

### Исправлено
- **DTLS keepalive pong** — сервер эхо-ответ `0xFF` на ping от клиента; без этого consent-freshness на TURN-воркерах (client ≥ 0.3.30) не видит входящую активность и ложно убивает воркеров.

## [1.4.49] — 2026-06-18

### Добавлено
- **Логи Xray (панель)** — колонка **To** показывает домены: обогащение по DNS-ответам из access log (`got answer: domain -> [ip]`) и `routeOnly: false` на `redirect-in` для SNI.

### Обновлено
- **PWDTT Client v0.3.21** — без VK login; кнопка «Переподключить»; UI 680×860; relay-логи при обрыве TURN.

## [1.4.48] — 2026-06-17

### Исправлено
- **VK Creator** — ошибка лимита звонков показывалась дважды (`HttpUtil` + дублирующий `$message.error`).

## [1.4.47] — 2026-06-17

### Исправлено
- **VK Creator** — не более 4 звонков на профиль (как лимит VK hash); при превышении — ошибка «завершите лишний».

## [1.4.46] — 2026-06-17

### Исправлено
- **VK Creator** — при создании нового звонка старые для того же пользователя больше не завершаются и не удаляются; в таблице видны все активные звонки.
- **Ссылки / подписка** — VK hash включается в JSON-ссылку (`wdtt://`) и в ответ подписки.

### Обновлено
- **PWDTT Client v0.3.10** — принимает sub URL с любым путём/портом панели; wdtt:// с полем `sub` внутри.

## [1.4.45] — 2026-06-17

### Обновлено
- **PWDTT Client v0.3.9** в `release-assets/`: импорт только по ссылке подписки панели (`https://…/subs/…`); поддержка `did` и `vk_hash`; без прямого `wdtt://` и ручного ввода сервера.

## [1.4.44] — 2026-06-17

### Изменено
- **Релизы / PWDTT Client** — бинарники клиента лежат в `release-assets/` и прикрепляются к каждому релизу WDTT; пользователям не нужен отдельный репозиторий. Ссылки в README и release notes — только на [релизы WDTT](https://github.com/ildarmaga/wdtt/releases).

## [1.4.43] — 2026-06-17

### Исправлено
- **Релизы / pwdtt-client** — CI не игнорирует ошибку скачивания (убран `| tail -1`); без секрета `PWDTT_CLIENT_GH_TOKEN` job падает явно.

## [1.4.42] — 2026-06-17

### Добавлено
- **Релизы** — в каждый GitHub Release WDTT: бинарники **[pwdtt-client](https://github.com/ildarmaga/pwdtt-client)** (`pwdtt-client-linux-amd64`, `pwdtt-client-windows-amd64.exe`) + блок в описании (`docs/RELEASE_CLIENT.md`). CI: секрет `PWDTT_CLIENT_GH_TOKEN` — см. `docs/RELEASE.md`.

## [1.4.40] — 2026-06-17

### Исправлено
- **Журналы (UI)** — `formattedLogs` объявлен в `logModal`/`xraylogModal` до инициализации Vue 2; без этого поле не было реактивным и модалка оставалась с «…» после успешного API-ответа.

## [1.4.39] — 2026-06-17

### Исправлено
- **Лимит устройств** — `PatchUserDeviceBindings` больше не сбрасывает `max_devices` до 1 при привязке нового устройства (GETCONF); обновляются только `device_id` и `wdtt_user_devices`.
- **Журналы** — парсинг POST в `/panel/api/server/logs/` через `readJSON`; ошибка загрузки показывается в модалке вместо пустого «…».

## [1.4.38] — 2026-06-16

### Исправлено
- **Журналы (unified)** — WDTT/Panel читаются из in-memory ring buffer (как 3x-ui), без `journalctl` на каждый запрос; модалка открывается сразу, данные подгружаются в фоне.
- **Журнал Xray** — access log: убрана перезагрузка конфига из SQLite на каждую строку (~6 с → десятки мс); error log читается из `/var/log/wdtt-xray/error.log` вместо `journalctl`; модалка открывается сразу.
- **CPU History → Процессы** — `<sparkline />` в HTML «съедал» блок таблицы (браузер парсил его как незакрытый тег); заменено на `<sparkline></sparkline>`.

### Добавлено
- **CPU History → Процессы** — список под графиком CPU; порядок по PID (без прыжков); без белого overlay при обновлении.

## [1.4.37] — 2026-06-16

### Добавлено
- **Страница подписки (HTML)** — все форматы ссылок из «Подключений»: JSON, colon (iOS/Android/PWDTT/Windows), qwdtt. Raw-подписка для клиентов без изменений (одна JSON-ссылка).

### Исправлено
- **qwdtt-ссылки** — пароль в конце query без `%26`/`%40` (`pass=f1k-23&-89@`), как ожидает qWDTT.
- **VK Creator** — при завершении звонка hash удаляется из профиля пользователя (когда звонок мёртв и строка убирается из таблицы); то же при замене звонка новым.

## [1.4.36] — 2026-06-16

### Добавлено
- **VK Creator в панели (нативно)** — создание VK hash / join link без отдельного `headless-vk-creator`: вкладка **Настройки → VK Creator**, выбор пользователя, кнопка «Создать звонок», hash сразу записывается в профиль.
- **Cookies и звонки в `panel.db`** — `vk_cookies`, `vk_calls` (SQLite v13–v14); импорт из legacy `cookies-vk.json` / `vk-creator-sessions.json`.
- **Вход VK в панели** — логин/пароль VK, вставка `remixsid`, загрузка `cookies-vk.json`.
- **Завершение звонка** — `calls.forceFinish`; строка в таблице исчезает только когда звонок перестаёт быть «живым» (статус «завершается»).

### Изменено
- Убрана установка внешнего бинарника creator из UI; достаточно HTTP API VK и cookies.

### Исправлено
- Сохранение cookies из формы (`ParseForm` вместо `ParseMultipartForm` на urlencoded).
- «hash получен, но не сохранён: пользователь не найден» — в API уходит `password_key`, не маскированный пароль.
- Очистка битых `call_id` в БД; валидация UUID из `calls.start`.

### Cookies для VK Creator
Для создания звонков нужны VK cookies (`remixsid`). Удобно получить через **[WhitelistBypass.Creator](https://github.com/kulikov0/whitelist-bypass/releases)** (релизы [whitelist-bypass](https://github.com/kulikov0/whitelist-bypass/releases)), затем вставить `remixsid` или экспортированный JSON в панель.

## [1.4.35] — 2026-06-16

### Добавлено
- **VK Creator (Whitelist Bypass)** — вкладка в настройках: загрузка `cookies-vk.json`, установка `headless-vk-creator`, создание VK hash/join link из панели; кнопка в карточке пользователя.

## [1.4.34] — 2026-06-16

### Исправлено
- **Журнал WDTT** — снова показываются строки `[СТАТ]` (пользователи, сессии, NAT, трафик); при их отсутствии в journal — сводка из `/etc/wdtt/server.log`.

## [1.4.33] — 2026-06-16

### Исправлено
- **Журнал Xray (access log)** — больше не показывает internal `api -> api` (опрос stats панелью); при отсутствии VPN-трафика — понятное сообщение вместо мусора.

## [1.4.32] — 2026-06-16

### Исправлено
- **Журнал WDTT/Xray** — убран `journalctl --grep/-v` (не поддерживается на части systemd); фильтр `[СТАТ]` в коде; строки без timestamp из journal больше не отбрасываются.
- **Журнал Xray (access log)** — fallback на `/var/log/wdtt-xray/access.log`, если в config `none`, но файл есть; при idle показываются internal `api -> api` строки вместо пустого «No Record...».
- **Выбор версии Xray** — обёртка `.xray-version-list-wrap` в HTML (CSS v1.4.31 без неё не работала).

## [1.4.31] — 2026-06-16

### Исправлено
- **Выбор версии Xray** — прокрутка списка версий (max-height 420px), как в обновлении панели.

## [1.4.30] — 2026-06-16

### Исправлено
- **Обновление панели** — прокрутка списка версий (как в «Посмотреть порты»), max-height 420px.

## [1.4.29] — 2026-06-16

### Изменено
- **Главный пользователь** — comment `ADMIN` вместо «Владелец» (seed, migrate legacy).

### Исправлено
- **Журналы** — быстрее загрузка: кэш unified-режима, без `systemctl status` на каждый запрос, исключение `[СТАТ]` в journalctl, меньший over-fetch; модалка открывается сразу.

## [1.4.28] — 2026-06-16
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
