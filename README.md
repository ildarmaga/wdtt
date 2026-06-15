# WDTT

VPN на VPS через медиарелеи ВК (TURN) с веб-панелью и опциональным **Xray**-маршрутизатором.

```
wdtt/
├── server/                             # wdtt-server (DTLS + userspace WG + NAT)
├── panel/                              # wdtt-panel (UI в стиле 3x-ui)
├── pkg/                                # sharelink, vkhash, paneldb (общее)
├── deploy.sh
└── docs/                               # SERVER.md (разбор) + API.md (REST панели)
```

## Происхождение

Серверная часть основана на проекте **[proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android)** (автор [amurcanov](https://github.com/amurcanov)) — WireGuard-туннель через DTLS-медиарелеи ВК.

WDTT расширяет upstream: мультипользователи, веб-панель, Xray, ссылки `wdtt://`, Telegram-бот, установщик. Подробности: **[CREDITS.md](CREDITS.md)**

## Быстрая установка

```bash
bash <(curl -Ls https://raw.githubusercontent.com/ildarmaga/wdtt-install/main/install.sh)
```

Панель: `http://IP:2860/wdtt/` — логин `admin` / `wdtt` (смените в настройках).

## Совместимые клиенты

| Клиент | Платформа | Формат ссылок |
|--------|-----------|---------------|
| **VK Turn Proxy** | iOS | `wdtt://` (colon), 1-й хеш или `VK_HASH` |
| **WDTT** | Android | `wdtt://` (colon), до 4 хешей через `,` или `VK_HASH` |
| **PWDTT** | Desktop Win/Linux | `wdtt://` (colon), до 4 хешей через `,` или `VK_HASH` |
| **WDTT** | Windows | `wdtt://` (colon), до 4 хешей через `,` или `VK_HASH` |
| **qWDTT** | Android (fork) | `qwdtt://`, `hashes=h1,h2,…` (bare, до 4) или `VK_HASH` |
| **WDTT** (qwdtt) | Windows | `qwdtt://`, `hashes=h1,h2,…` (bare, до 4) или `VK_HASH` |

## Компоненты

| Компонент | Описание |
|-----------|----------|
| **wdtt** | DTLS `:56000`, userspace WG `wdtt0`, панель `:2860`, NAT, Telegram-бот (единый процесс) |
| **wdtt-server** | Только VPN-сервер (legacy, отдельный бинарник) |
| **wdtt-panel** | Только веб-панель (legacy, отдельный бинарник) |
| **wdtt-xray** | Redirect трафика `wdtt0` → outbound (NL, WARP…) |

Конфиг: `/etc/wdtt/panel.db` (SQLite, общая для сервера и панели), `/etc/wdtt-xray/config.json`.

## Сборка

```bash
# Единый бинарник server+panel (основной, systemd → /usr/local/bin/wdtt)
./build.sh amd64 unified
sudo ./install-local.sh amd64

# Отдельно (legacy)
./build.sh amd64 server   # wdtt-server-linux-amd64
./build.sh amd64 panel    # wdtt-panel-linux-amd64
./build.sh amd64 all      # всё сразу
```

Локальная разработка: `go.work` связывает корень (`pkg/`), `server/` и `panel/`.

Основные флаги `wdtt`: `-listen`, `-wg-port`, `-config-dir`, `-password`,
`-admin-addr`, `-no-panel`, `-handshake-timeout`, `-max-dtls-per-device`.
См. **[docs/SERVER.md](docs/SERVER.md)**.

## API панели

Документация: **[docs/API.md](docs/API.md)** — cookie-сессии, пользователи, inbound, Xray, формат ссылок `wdtt://`.

## Репозитории

| Репозиторий | Назначение |
|-------------|------------|
| [ildarmaga/wdtt](https://github.com/ildarmaga/wdtt) | Сервер + панель (этот репо) |
| [ildarmaga/wdtt-install](https://github.com/ildarmaga/wdtt-install) | Установщик одной строкой |
| [anton48/vk-turn-proxy-ios](https://github.com/anton48/vk-turn-proxy-ios) | iOS-клиент |
| [amurcanov/proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android) | Android-клиент / исходный VPN-протокол |

## Лицензия

[GNU GPL v3](LICENSE) — как upstream [proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android).
