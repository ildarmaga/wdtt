# WDTT

VPN на VPS: **VK TURN → WRAP/DTLS → WireGuard** (userspace) с веб-панелью и опциональным **Xray**-маршрутизатором.

Трафик клиента идёт через медиарелеи ВК (TURN) до вашего VPS, поэтому подключение
выглядит как обычный WebRTC-звонок и устойчиво к блокировкам.

```
wdtt/
├── server.go, devices.go, deploy.sh   # wdtt-server (DTLS + userspace WG + NAT)
├── relay_sessions.go, relay_fail.go    # учёт/эвикция и fast-fail TURN-relay
├── panel/                              # wdtt-panel (UI в стиле 3x-ui)
└── docs/                               # SERVER.md (разбор) + API.md (REST панели)
```

## Происхождение

Серверная часть основана на проекте **[proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android)** (автор [amurcanov](https://github.com/amurcanov)) — WireGuard-туннель через DTLS-медиарелеи ВК.

WDTT расширяет upstream: мультипользователи, веб-панель, Xray, ссылки `wdtt://`, Telegram-бот, установщик. Подробности: **[CREDITS.md](CREDITS.md)**

## Быстрая установка

```bash
bash <(curl -Ls https://raw.githubusercontent.com/ildarmaga/wdtt-install/main/install.sh) install -p YOUR_PASSWORD --xray --panel
```

Панель: `http://IP:2860/wdtt/` — логин `admin` / `wdtt` (смените в настройках).

## Архитектура

```
[Клиент] ──TURN(VK :19302)──▶ [VK relay] ──WRAP/DTLS──▶ [wdtt-server :56000]
                                                              │ GETCONF → WG config
                                                              ▼
                                              [userspace WireGuard wdtt0 10.66.66.0/24]
                                                              │ MASQUERADE
                                                              ▼
                                              [Xray (опц.) → outbound] → Интернет
```

- UDP-пакеты до DTLS маскируются под **RTP** (ChaCha20-Poly1305, ключ из пароля по HKDF).
- После DTLS-handshake клиент шлёт `GETCONF` и получает WG-конфиг; дальше — relay WG внутри DTLS.
- Подробный разбор протокола и фаз — **[docs/SERVER.md](docs/SERVER.md)**.

## Компоненты

| Компонент | Описание |
|-----------|----------|
| **wdtt-server** | DTLS `:56000`, userspace WG `wdtt0` (`10.66.66.0/24`), пароли, лимиты трафика/скорости, NAT, Telegram-бот |
| **wdtt-panel** | Дашборд, подключения (inbound), пользователи, Xray, настройки, ссылки `wdtt://` |
| **wdtt-xray** | Redirect трафика `wdtt0` → outbound (NL, WARP…) |

Конфиг: `/etc/wdtt/panel.db` (SQLite, общая для сервера и панели), `/etc/wdtt-xray/config.json`.

### Устойчивость TURN-relay (v1.3.x)

- `relay_sessions.go` — учёт активных relay-сессий, эвикция покинутых после 3 мин простоя.
- `relay_fail.go` — fast-fail «мёртвых» VK relay по числу неудачных handshake.
- Живые сессии не эвиктятся: активность обновляется на каждом пакете и keepalive.

## Клиенты

| Платформа | Клиент | Заметки |
|-----------|--------|---------|
| **Windows / Linux** | [ildarmaga/pwdtt-client](https://github.com/ildarmaga/pwdtt-client) | Десктоп (Wails), подписки и `wdtt://`, health-выбор relay |
| **iOS** | [anton48/vk-turn-proxy-ios](https://github.com/anton48/vk-turn-proxy-ios) | TestFlight / IPA, поддержка `wdtt://` |
| **Android** | [amurcanov/proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android) | Совместим с протоколом WDTT |

Все клиенты используют единую схему DTLS + TURN relay + WireGuard и ссылки `wdtt://base64(JSON)`.

## Сборка

```bash
# Сервер
./build.sh amd64
sudo install -m 0755 wdtt-server-linux-amd64 /usr/local/bin/wdtt-server

# Панель (UI встроен через go:embed, нужен Go 1.21+)
chmod +x panel/build.sh
./panel/build.sh /usr/local/bin/wdtt-panel
```

Основные флаги `wdtt-server`: `-listen`, `-wg-port`, `-config-dir`, `-password`,
`-admin`, `-bot-token`, `-handshake-timeout`, `-admin-addr`, `-max-dtls-per-device`.
См. **[docs/SERVER.md](docs/SERVER.md)**.

## API панели

Документация: **[docs/API.md](docs/API.md)** — cookie-сессии, пользователи, inbound, Xray, формат ссылок `wdtt://`.

## Репозитории

| Репозиторий | Назначение |
|-------------|------------|
| [ildarmaga/wdtt](https://github.com/ildarmaga/wdtt) | Сервер + панель (этот репо) |
| [ildarmaga/wdtt-install](https://github.com/ildarmaga/wdtt-install) | Установщик одной строкой |
| [ildarmaga/pwdtt-client](https://github.com/ildarmaga/pwdtt-client) | Десктоп-клиент (Windows/Linux) |
| [anton48/vk-turn-proxy-ios](https://github.com/anton48/vk-turn-proxy-ios) | iOS-клиент |
| [amurcanov/proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android) | Android-клиент / исходный VPN-протокол |

## Лицензия

[GNU GPL v3](LICENSE) — как upstream [proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android).
