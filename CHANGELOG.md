# Changelog

Формат: каждый релиз — секция `## [X.Y.Z]`. CI подставляет её в GitHub Release автоматически.

## [1.4.81] — 2026-08-10

### Fix — Подключения: убран WB Stream из UI
- Ошибка `нет такого шаблона "modals/wbCookiesModal"`: public build без wb_cookies_modal, а connections.html всё ещё тянул WB.
- Карточка WB Stream и cookies-модалки убраны с «Подключения» (публичная панель — только WDTT/Xray).
- Подсказка на главной: `RAW: N` = число живых RAW-сессий, не настройка.
- Тест `TestConnectionsHTMLHasNoWBTemplates`.

## [1.4.80] — 2026-08-10

### Fix — seed RAW_DIRECT_PORT из install-inbound.env
- `applyDeployInboundDefaults` читает `RAW_DIRECT_PORT` (install.sh ≥1.4.79).

## [1.4.79] — 2026-08-10

### Fix — public build без WB-полей
- Убраны WbRoom / combinedOnlineCount из panel store (pack не тащить в public).
- migratePanelDBV15: raw_enable + raw_direct_port.

## [1.4.78] — 2026-08-10

### Panel — подсказка RAW без qWDTT
- В модалке inbound: описание режима RAW без упоминания qWDTT; указан PWDTT Desktop ≥0.3.275.

## [1.4.77] — 2026-08-10

### Panel — WDTT / RAW сессии раздельно
- На дашборде: **WDTT** (DTLS/WG) и **RAW** отдельно; общий «Сессий» убран.

## [1.4.76] — 2026-08-10

### Panel — порты в один ряд
- Колонка «Порт»: DTLS / WG / RAW без переноса строки.
- Убраны подсказки под RAW UDP портом и подсетью в модалке.

## [1.4.75] — 2026-08-10

### Panel — RAW UDP порт виден и настраивается
- В **Подключениях** показан третий порт: DTLS / WG / **RAW** (по умолчанию DTLS+3, сейчас 56003).
- В модалке WDTT: поле **RAW UDP порт**, firewall + systemd, restart при смене.
- `wdtt://` JSON: поле `raw` (клиент ≥0.3.276); учёт трафика RAW как у WG без изменений.

## [X.Y.Z]`. CI подставляет её в GitHub Release автоматически.

## [1.4.132] — 2026-08-09

### Fix — RAW INPUT accept + reorder
- `iptables -I INPUT -i wdtt-raw -j ACCEPT` — без этого UFW DROP резал local
  (ping/HTTP на 10.70.66.1 с клиента). Как для `wdtt0`.
- Reorder stall TTL 3s → 40ms (дырка на multipath больше не убивает TCP на секунды).
- TUN Read offset=virtio headroom; `[RAW-DOWN]` counters.

## [1.4.127] — 2026-08-09

### Fix — RAW downlink follows uplink TURN
Downlink `hash%N` часто выбирал другой DTLS/TURN, чем uplink → асимметрия
путей и TCP download ~1–2 Мбит. Теперь `noteUplink` закрепляет 5-tuple на
сессию, принявшую пакет; ответ идёт тем же TURN.

## [1.4.126] — 2026-08-09

### Исправлено
- **RAW убивался сервером каждые ~10s**: `wg-idle … relay_evict` смотрел только на WG peer counters. В RAW трафик мимо WireGuard → counters замирали → stats-тик закрывал все DTLS. Теперь при живых DTLS-сессиях device не эвиктится.

## [1.4.125] — 2026-08-09

### Исправлено
- **RAW download collapse**: chunk-RR downlink без reorder ломал TCP (speedtest ~0.5 Мбит при нормальном upload). Теперь flow-sticky 5-tuple→сессия + async очередь downlink (TUN read не блокируется на DTLS Write). Shared IP на device сохранён.

## [1.4.124] — 2026-08-09

### Изменено
- **RAW multipath**: один `10.70.x.y` на device (все воркеры шарят IP). Downlink chunk-RR по DTLS-сессиям — агрегат скорости как у WG, без sticky «один поток = один TURN».
*(Chunk-RR для RAW откатан в 1.4.125 — без WG reorder вреден.)*

## [1.4.123] — 2026-08-09

### Исправлено
- **RAW speedtest ~0**: MSS clamp + DF-clear теперь и для `10.70.0.0/16` (раньше только WG `10.66.66.0/24` → PMTU blackhole на больших TCP). При поднятии `wdtt-raw` вызывается `wdtt-mtu-rules.sh up`.

## [1.4.122] — 2026-08-07

### Изменено
- **RAW multi-worker**: подсеть `10.70.0.0/16` (`10.70.66.1/16` на iface), уникальный `10.70.x.y` на каждый `RAWCONF` (без sticky IP по device). Несколько DTLS-сессий одного клиента больше не вытесняют друг друга.

## [1.4.121] — 2026-08-07

### Исправлено
- **RAW TUN `invalid offset`**: запись в `wdtt-raw` с headroom 16 (virtioNetHdr на Linux). Раньше сессия рвалась сразу после первого IP-пакета → клиент видел «VK обновил relay».

## [1.4.120] — 2026-08-07

### Исправлено
- **Трафик RAW** в панели: направление ↑/↓ как у WG; flush хвоста при disconnect; скорость/iface на дашборде суммируют `wdtt0` + `wdtt-raw`. Лимиты/сброс/пользователи — те же счётчики.

## [1.4.119] — 2026-08-07

### Добавлено
- **Панель RAW**: тег на «Подключениях», переключатель и подсеть `10.70.66.0/24` в модалке WDTT, счётчик RAW-сессий на дашборде. Online учитывает RAW (не только WG).

## [1.4.118] — 2026-08-07

### Добавлено
- **RAW режим** (qWDTT-совместимый): `RAWCONF` / `GETCONF_RAW` на том же DTLS-порту — IP over WRAP без WireGuard (`wdtt-raw`, `10.70.66.0/24`). WG/`GETCONF` без изменений.

## [1.4.117] — 2026-08-07

### Исправлено
- **Смена пароля admin** ([ildarmaga/wdtt#27](https://github.com/ildarmaga/wdtt/issues/27)): логин не декодировал form-urlencoded пароль со спецсимволами.

## [1.4.116] — 2026-08-03

### Изменено
- **Мобильная панель**: «Пользователи» / «Подключения» — карточки без горизонтального скролла; лишние кнопки WB/WDTT убраны (меню только «Изменить»).

## [1.4.103] — 2026-08-03

### Исправлено
- **VK_HASH / WB_ROOM**: колонки в «Пользователи» синхронизируются с живыми `vk_calls` / `wb_rooms` (мёртвые хеши и stale room с профиля снимаются).

 — 2026-08-03

### Исправлено
- **Онлайн vs устройства**: «Онлайн» снова только по WG; WB joiner показывается отдельным тегом **WB** (больше не выглядит как рассинхрон Онлайн при 0 живых WG).

 — 2026-08-03

### Исправлено
- **VK мёртвые звонки**: сами исчезают из таблицы; «Завершить» на мёртвом больше не падает с VK API 8.

 — 2026-08-03

### Исправлено
- **VK «завершается»**: после `forceFinish` join-ссылка у VK остаётся живой — больше не ждём мёртвый preview, строка снимается после одного показа «завершается».

 — 2026-08-03

### Исправлено
- **WBT ICE rebind**: на `sub offer` / live ICE больше не делается SwapTunnel→RestartLink — smux не разъезжается (`invalid protocol` / Soft-rebind). Полный rebind только после `tunnelLost`.

## [1.4.98] — 2026-08-02

### Исправлено
- **Лимит скорости WB**: те же ↓/↑ Мбит/с из профиля режут трафик WB Creator (раньше только WG/tc на wdtt0). Save обновляет лимит без пересоздания комнаты.

## [1.4.97] — 2026-08-02

### Исправлено
- **Лимит скорости live**: смена 1→5 Мбит/с через `tc class replace` без удаления class/filter — действует сразу, без переподключения клиента.

## [1.4.96] — 2026-08-02

### Исправлено
- **Лимит скорости**: HTB больше не блокируется qdisc `fq` (BBR) на `wdtt0`; после Save лимит сразу применяется к IP устройств; дробные Мбит/с (0.1) корректно уходят в tc (kbit).

## [1.4.95] — 2026-08-02

### Исправлено
- WB Creator «Удалить»: комната остаётся со статусом «завершается», пока runner реально не остановится (как VK).

## [1.4.94] — 2026-08-02

### Исправлено
- VK Creator «Завершить»: звонок остаётся в таблице со статусом «завершается», пока VK preview реально не умрёт (раньше удалялся сразу).

## [1.4.93] — 2026-08-02

### Изменено
- VK Creator: убран блок «Вход по паролю» из панели.

## [1.4.92] — 2026-08-02

### Исправлено
- **Синхронизация**: rename пароля обновляет `vk_calls` (раньше только `wb_rooms`).
- **Удаление пользователя**: каскадно чистит `wb_rooms`/`vk_calls`, останавливает creator-сессии.
- **Трафик WB**: delta flush бампит `users_rev` — VPN absolute flush больше не затирает байты.
- **WB_ROOM**: reconcile при открытии WB Creator + ошибка очистки `wb_room` при удалении комнаты больше не глотается.

## [1.4.91] — 2026-08-02

### Изменено
- VK Creator: убрана инструкция Chrome/remixsid (вход по паролю в панели).
- WB Creator: убран info-блок «Creator в wdtt-app».

## [1.4.90] — 2026-08-02

### Исправлено
- **WB_ROOM**: Save пользователя больше не затирает `wb_room` (модалка поле не шлёт). Комнаты WB Creator снова остаются на профиле.

## [1.4.89] — 2026-08-02

### Исправлено — data-loss / sync (как public 1.4.65)
- Rename+WRAP при manageDevices; traffic flush fence; clear/normalize vk_hash+ports; VK Creator merged hash; main link; bot inbound ports; HotOnly error; JSON preview hash.
- WbRoom сохраняется при apply/remove VK hash.

## [1.4.88] — 2026-08-02

### Исправлено
- **Версия Xray в панели**: ответ GitHub ~2MB обрезался лимитом 1MB → ложное `GitHub API HTTP 200`. Лимит 8MB.

## [1.4.87] — 2026-07-30

### Исправлено
- **Версия Xray в панели**: при GitHub rate limit / ошибке API — понятное сообщение вместо `json: cannot unmarshal object into Go value of type []panel.XrayRelease`.

## [1.4.86] — 2026-07-21

### iOS — DEV UI (0.4.30)
- Mock tunnel + баннер; Settings → DEV UI или `WDTT_DEV_UI=1`.
- Stub без `Mobile.xcframework`; Canvas Preview «WDTT DEV».
- Fix CI compile (MainActor seed, Preview ViewBuilder).
- Веб-макет: `ios-proxy-app/ui-preview` → `:8766`.

### iOS — название WDTT + иконка (0.4.28)
- Display name: **WDTT** (было «WB Stream»).
- AppIcon: чёрный фон + «W» (1024×1024).

### Изменено — VK `vk.com` → `vk.ru` (как anton48 builds 169–171)
- Панель VK Creator / cookie API: `login.vk.ru` + `api.vk.ru`, Origin `vk.ru`.
- `vkhash` + share: join-url → `https://vk.ru/call/join/…`, парсер accept-both.
- Клиентский vkcore (WB): join + cookie-path + captcha domain из `redirect_uri`.

## [1.4.85] — 2026-07-21

### Исправлено — долгий первый SOCKS bring-up (~9s)
- После skip deferred rebind `pre-SOCKS` ждал rebound **5s** впустую, потом ещё RestartLink.
- Ждём rebound ≤800ms; RestartLink только если SwapTunnel реально был. Первый коннект снова ~2–3s.

## [1.4.84] — 2026-07-21

### Исправлено — soft-rebind: dial-hold при SwapTunnel/RestartLink
- На время смены smux joiner закрывает in-flight SOCKS и не принимает новые dial’ы (~1.5s) — без лавины `closed pipe` у v2rayN.
- Клиент ≥0.3.225 не эскалирует эти ошибки в полный reconnect.

## [1.4.83] — 2026-07-21

### Исправлено — soft-rebind после смены сети (smux desync)
- Creator больше не пропускает KCP rebind на `sub offer` («keep smux in sync») — joiner всегда `RestartLink`, иначе `OpenStream closed pipe` / 0 B/s при живом WebRTC.
- Debounce `SwapTunnel` <2s (creator+joiner) — без двойного rebind от deferred sub-offer.
- Клиент ≥0.3.224 (эскалация SOCKS на полный reconnect).

## [1.4.82] — 2026-07-10

### Исправлено — SOCKS UDP двусторонний (Steam / v2rayN TUN)
- Shared SOCKS UDP ASSOCIATE был request/response: ответ только на клиентский send → Steam SDR / игры теряли unsolicited datagrams.
- Теперь per-dest streaming flow (как DialUDP): server→client качается без ожидания ping.
- Тест: `TestSocksUDPUnsolicitedDatagrams`. Клиент ≥0.3.222.

## [1.4.81] — 2026-07-10

### Исправлено — dual ICE track #3/#4 + RestartLink storm
- ICE renegotiation давала remote VP8 #3/#4 → оба шли в HandleFrame + повторный `AdaptTrackCount`/`RestartLink` → smux/`remote not ready`.
- Drain треков >2; scale-up только один раз (`atomic`).
- KCP: camera-only **без** seq/reorder (как single). Тесты: `TestShouldDrainRemoteVP8`, `TestDualFingerprintConcurrentSOCKS`, `TestLinkNeverReordersDualFingerprint`.
- Клиент ≥0.3.221.

## [1.4.80] — 2026-07-10

### Исправлено — dual-track KCP только на camera
- `SendRaw` больше не шардит и не дублирует на screenshare: WB SFU роняет/тормозит 2-й трек → reorder gaps / 2× load → `remote not ready` в Telegram.
- Dual VP8 остаётся (fingerprint + AdaptTrackCount); данные KCP — camera + seq prefix.
- Нужен клиент ≥0.3.220 (тот же relay).

## [1.4.79] — 2026-07-10

### Исправлено — Dual-track по выбору клиента (1 или 2)
- Creator стартует с **1** VP8; Dual-track=on у joiner → scale-up до 2 + `ScreenShare=true` (не drain 2-го) + KCP restart до старта joiner KCP.
- Dual-track=off → обе стороны на 1 треке. Убран force dual на SocksOnly.
- Клиент ≥0.3.212.

## [1.4.74] — 2026-07-10

### Исправлено — Windows benign SOCKS closes
- `IsBenignConnError` теперь глотает `wsarecv` / `forcibly closed by the remote host` (WSAECONNRESET) — на Windows клиенте эти closes раньше всё равно попадали в `[ERROR]`.
- Клиент ≥0.3.209 (тот же фильтр + UI safety net в `classifyWBLog`).

## [1.4.73] — 2026-07-10

### Исправлено — шум в логах SOCKS (joiner)
- Joiner-side SOCKS read loop логировал каждый нормальный `use of closed network connection` / RST как `[ERROR]` — из-за этого рабочий туннель выглядел «сломанным» (десятки ошибок на закрытие сессий приложением).
- Теперь benign-ошибки (`IsBenignConnError`) не пишутся, как уже сделано на creator-стороне. `EOF with no data read` и сброс CDN — это поведение назначения, не туннеля.
- Функционал туннеля не менялся; поведение SOCKS то же, чище лог.

## [1.4.72] — 2026-07-09

### Исправлено — dual SOCKS + sticky obfuscator после recycle
- **Корень 1**: `OnPeerRestart` → `Reset()`/`closeAll` убивал живые SOCKS; mid-session scale-up 1→2 ломал путь.
- **Корень 2**: один `TunnelObfuscator` на все SFU-сессии — sticky peer epoch глотал кадры нового joiner (`resending vp8 config`, CONNECT TIMEOUT).
- Creator стартует с **2** VP8; soft-shrink при dual=off; peer-restart Reset только если tcp+udp=0; OnTunnelLost/`sess.Done` → `obf.ResetPeer()` + `Close` + `activeBridge=nil`.
- Клиент ≥0.3.208 (Dual-track toggle).

## [1.4.71] — 2026-07-09

### Изменено — сервер следует Dual-track клиента (1 или 2)
- Joiner шлёт `trackCount` в VP8 config; creator логирует `joiner Dual-track=…` и вызывает `AdaptTrackCount`.
- dual=on → scale-up до 2 треков; dual=off → soft-shrink writers до 1 (без Stop/renegotiate — hard shrink ломал pub PC).
- Клиент: Настройки → Dual-track (0.3.208+).

## [1.4.70] — 2026-07-09

### Исправлено — creator 1 VP8 + не shrink mid-session
- Creator стартовал с `ScreenShare=true` (2 трека); joiner dual=false просил 1 → `AdaptTrackCount` shrink ломал pub PC (`connection closed`) → SOCKS почти без ↓.
- Creator по умолчанию 1 трек; shrink отключён (только scale-up при dual). Клиент ≥0.3.208.

## [1.4.69] — 2026-07-09

### Исправлено — RelayBridge SendData не шардить на screenshare
- `MultiTrackTunnel.SendData` всегда на camera track: шардинг `connID%N` ронял SOCKS ClientHello на WB SFU.
- Клиент ≥0.3.208 (dual-track UI + тот же fix). Dual-track опционален; дефолт выкл.

## [1.4.68] — 2026-07-09

### Изменено — WB creator на RelayBridge (как kulikov0)
- Вместо `wbtunnel` KCP/smux — `tunnel.RelayBridge` (оригинальный framing MsgConnect/MsgData).
- Клиент ≥0.3.206 обязателен (тот же протокол). Старые KCP-клиенты не совместимы.

## [1.4.67] — 2026-07-09

### Изменено — простой KCP: без AIMD / streamSem / per-host
- Клиент SOCKS (v0.3.205): убраны лимиты потоков и delay-based shrink окна (`wnd=64` под нагрузкой).
- Creator/joiner: фиксированное send window 2048; failure ack `0x01` сохранён.
- ICE settle перед ServeSOCKS; не эмитить `SOCKS_READY` на rebind до Listen.

## [1.4.66] — 2026-07-09

### Исправлено — zombie smux-слоты после dial fail (Telegram → streamSem full)
- Поле 0.3.201: creator при ошибке dial молчал; joiner держал `streamSem` до 90 с → 512 слотов забиты, reject-спам 149.154.
- **Фикс**: failure ack `0x01`; joiner wait 25 с; cap 128 + per-host 12. Клиент v0.3.202.

## [1.4.65] — 2026-07-09

### Исправлено — KCP wnd=64 после Telegram-шторма (grow блокировался elevated ewma)
- Поле 0.3.200: warmup OK, затем `floor=155 ewma=250 wnd=64`; сервер за минуту принял ~166 connect на 149.154 (Telegram) в одну KCP-сессию.
- **CC**: `nextKCPWnd` проверял shrink(ewma) раньше grow(rttFast) → при ewma выше shrinkThresh окно не росло даже при низком recent-min RTT.
- **Admission**: joiner SOCKS (xray path) без `streamSem`; custom без QUIC-block.
- **Фикс** (общий relay): grow before shrink; streamSem на handleSOCKS; всегда block UDP/443. Клиент v0.3.201.

## [1.4.64] — 2026-07-09

### Исправлено — creator зависал после обрыва SFU → 403 «guests cannot create rooms»
- Поле: после `websocket close 1006` / ICE closed комната `019ef0d5` не делала rejoin; KCP `tuneLoop` тикал вхолостую, панель показывала «ожидание joiner», клиент ловил `owner/creator offline` / 403.
- **Причина**: `Session.endSession` закрывал `done` только после `stopTunnels` (мог зависнуть), а `Creator.Close` ждал `wg.Wait` навечно на `io.Copy` активных TCP-потоков → runner не доходил до rejoin.
- **Фикс**: `done` закрывается до `stopTunnels`; `Creator.Close` рвёт активные dial-сокеты, таймаут 3s на drain; creator при `sub ICE down` сразу `tunnel lost` (не ждёт stale peersBySID); TCP/UDP workers abort на `ctx.Done`.

## [1.4.63] — 2026-07-09

### Исправлено — окно KCP залипало на дне после upload-всплеска (пила загрузки)
- Инструментация 1.4.62 (`SLOW WriteSample`, `selected ICE pair`) показала: send-путь не блокируется (0 строк SLOW), несущая по UDP (srflx), а окно KCP после всплеска RTT на upload падает на `wnd=64` и зависает там 1–2 минуты даже при вернувшемся RTT (`rtt=44ms wnd=64` спустя 90с) — загрузка зажата в пилу.
- **Причина**: рост окна был привязан к медленному сглаженному `ewma`, который декеит ниже порога роста ~10–20 тиков после всплеска.
- **Фикс (Vegas/BBR-style)**: рост окна теперь по быстрому recent-min RTT (мин за 4 тика = 2с), shrink остаётся на ewma+гистерезис. Окно поднимается со дна за 1–2 тика после реального спада RTT. Общий relay-код — фиксит и сервер (download-направление), и клиент.
- Тесты: `TestNextKCPWndFastRecovery` + все прежние CC-тесты зелёные.

## [1.4.62] — 2026-07-09

### Наблюдаемость — локализация WB-клина на upload (транспорт под KCP)
- Реальные bidir-тесты (`relay/wbtunnel/carrier_load_test.go`, секундные прогоны ↑/↓ через смоделированную несущую) + разбор pion-транспорта показали: при upload-всплеске RTT несущей улетает в 15c+ при ~77 KB данных в полёте → bloat **ниже KCP**, в pion/SRTP/ICE/TURN. `track.WriteSample` синхронный и держит `writeMu` до возврата всего стека вниз; один медленный `WriteSample` замораживает весь VP8-писатель дорожки (keepalive + все KCP-кадры, ACK тоже).
- **Лог `vp8tunnel: SLOW WriteSample <ms>`** (порог 150 ms, rate-limit 500 ms) — прямо показывает залипание несущей.
- **Лог `[lk] pub/sub selected ICE pair`** — какой транспорт реально несёт медиа (UDP srflx vs TCP/UDP TURN relay).
- Только наблюдаемость: конфиг ICE не менялся, поведение как в 1.4.61.

## [1.4.61] — 2026-07-08

### Исправлено
- **WB Stream / relay — WB встаёт намертво на upload-всплеске** (↓/↑ = 0, RTT → 20 c+, `OpenStream: timeout`, без восстановления; крошечный upload 30 KB/s ронял оба направления). Разбор исходников kcp-go по слоям показал корень: kcp-go гонит **всю** отдачу (данные + ACK) через **одну** горутину `postProcess` → `defaultTx`, зовущую наш `kcpConn.WriteTo` по очереди. Приоритетная ACK-полоса (1.4.59) стоит **после** этой сериализации и потому не спасала. Наш `WriteTo` при полном `outbound` **блокировался бесконечно** → `postProcess` замирал → вставала вся отдача, включая ACK → RTO-экспонента → клин. Это дедлок, а не насыщение канала.
- **`kcpConn` сделан неблокирующим**, как настоящий UDP-сокет: `WriteTo` и `deliver` дропают при полной очереди вместо блокировки (KCP надёжный — потеря ретрансмитится по RTO, но tx/rx-горутины несущей больше не замирают). Убрана старая 2-сек блокировка на приёме, стопорившая весь read-путь несущей.
- **Потолок KCP-окна 1024 → 384**: на быстром low-RTT пути окно доgrowало до 1024 (≈1.2 MB in-flight) при реальном BDP ~120 сегментов. Лишний «боезапас» вываливался в lossy WB/TURN на upload-всплеске разом → RTT в потолок. 384 (~460 KB) держит ~3 MB/s@150ms и режет разовый выброс.
- Тесты: `WriteTo`/`deliver` никогда не блокируют (со старым кодом виснут по таймауту), дроп рапортует успех.

## [1.4.60] — 2026-07-08

### Исправлено
- **WB Stream / relay — «плавание» скорости скачивания** (↓ 1.5 MB/s → 14 KB/s → назад при низком RTT). Причина: dual-track VP8 идёт через lossy/jittery TURN, и один тик раздутого RTT (джиттер несущей, не реальная очередь) вызывал жёсткий пропорциональный cut окна → быстрое скачивание обваливалось в пилу. На VK такого нет — там одноканальная несущая без reorder.
- **Гистерезис на shrink**: первый «высокий» тик теперь только слегка поджимает окно (×0.90); жёсткий пропорциональный cut срабатывает лишь если RTT держится высоким `kcpShrinkStreak` тиков подряд (реальный затор). Транзиентный джиттер поглощается, скачивание держит скорость; настоящий bufferbloat всё так же сливается за ~3 тика.
- Тесты: `nextKCPWnd` гистерезис (транзиент → мягко, sustained → жёстко), drain-bufferbloat (≤4 тика).

## [1.4.59] — 2026-07-08

### Исправлено
- **WB Stream / relay**: download всё ещё вставал в 0 B/s при одновременном upload'е, даже когда KCP-окно ужалось до минимума. Причина — не в окне: ACK'и скачивания и bulk-данные отдачи делили **один FIFO `outbound`**. Во время upload-всплеска очередь забивалась крупными PUSH-сегментами (~1200 Б), а ACK'и приёма попадали в её хвост (до 4096 пакетов) → сервер не получал ACK вовремя → его окно вставало → ↓ падало в 0, RTT уходил в 7 c, smux рвался по `OpenStream: timeout`. Объясняет, почему при чистом скачивании (рилсы) стопора нет: `outbound` не забивается.
- **Двухполосный `outbound`**: ACK/control-пакеты (KCP ACK / оконные пробы — без полезной нагрузки) идут по приоритетной полосе и **отправляются раньше** любого bulk. `pumpOutbound` перед каждым тиком полностью сливает hi-полосу. ACK'и скачивания больше не застревают за отдачей.
- Классификация пакета — разбором KCP-заголовка (нет data-bearing PUSH → control), а не по размеру: устойчиво к коалесцированным ACK.

## [1.4.58] — 2026-07-08

### Исправлено
- **WB Stream / relay**: download вставал при одновременном upload'е. Окно вырастало до 1024 на быстром скачивании (низкий RTT), а внезапный upload-всплеск этим же окном вываливал ~1.2 МБ в несущую; старый shrink ×0.7/тик драйнил буфер ~8 c — RTT успевал уйти в 800→2600 мс, ↓ падало в 0 B/s, новые smux-стримы отваливались по `write connect: timeout`.
- **Пропорциональный shrink**: чем сильнее раздут RTT над полом, тем жёстче режем окно (factor = floor/rttEwma, кламп [0.25, 0.80]) → 1024→64 за 1–2 тика вместо ~8.
- **Каденция tuneLoop 1с→500мс** — всплеск ловится и гасится вдвое быстрее.

## [1.4.57] — 2026-07-08

### Исправлено
- **WB Stream / relay**: KCP congestion control переписан на delay-based AIMD размера окна вместо `nc`-тумблера. `nc=1` постоянно (KCP не делает своего cwnd), а само окно — контроллер: additive-increase у пола RTT, multiplicative-decrease (×0.7) при раздувании → in-flight отслеживает реальный BDP и сливает буфер TURN до убегания. Лечит наблюдаемый разгон WBT RTT 56 → 11669 мс и ↓ 0 B/s под upload-нагрузкой.
- **RTT floor заморожен во время затора**: при RTT выше shrink-порога floor не ползёт вверх (раньше огромный gap тянул floor на сотни мс/тик даже при 0.01 → порог shrink рос, окно переставало ужиматься; в логе floor 91 → 1609 → 2795 мс). Теперь creep только когда путь у базовой линии.

## [1.4.56] — 2026-07-07

### Исправлено
- **WB Stream / relay**: adaptive KCP congestion control (delay-based `nc` toggle) — меньше пилы 0↔100 KB/s на lossy TURN при dual-track.

## [1.4.55] — 2026-06-24

### Исправлено
- **Домен панели** — поле можно оставить пустым (как в 3x-ui): значение больше не подставляется автоматически из SSL-сертификата при сохранении настроек. Домен из сертификата по-прежнему используется только для ссылок/ACME, если поле пустое.

## [1.4.54] — 2026-06-24

### Исправлено
- **VK Creator — проверка cookies.** Статус больше не показывает ложное «Cookies: OK» по одному наличию `remixsid`: панель проверяет сессию через `web_token` и помечает протухшие cookies как «устарели».

## [1.4.53] — 2026-06-21

### Добавлено
- **WB Stream Creator — фаза 2** — WebRTC creator внутри `wdtt-app`; статус туннеля; pause/resume.
- **WB Stream на «Подключения»** — карточка как у WDTT: комнаты, вкл/выкл, cookies, join link.

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
