# WDTT

[![Поддержать WDTT](https://devgamemaga.mooo.com:9443/b/soft.svg)](https://devgamemaga.mooo.com:9443/donate)

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
| **PWDTT** | Desktop Win/Linux | `wdtt://` (colon), до 4 хешей — `pwdtt-client-*` из [релизов WDTT](https://github.com/ildarmaga/wdtt/releases/latest); исходники: [ildarmaga/pwdtt-client](https://github.com/ildarmaga/pwdtt-client) (модифицированная версия [luminescq/PWDTT](https://github.com/luminescq/PWDTT), GPL-3.0) |
| **WDTT** | Windows | `wdtt://` (colon), до 4 хешей через `,` или `VK_HASH` |
| **qWDTT** | Android (fork) | `qwdtt://`, `hashes=h1,h2,…` (bare, до 4) или `VK_HASH` |
| **WDTT** (qwdtt) | Windows | `qwdtt://`, `hashes=h1,h2,…` (bare, до 4) или `VK_HASH` |

## VK hash из панели

В **Настройки → VK Creator** можно создать VK hash (join link) и сразу записать его пользователю — без отдельного сервера creator на VPS.

**Нужны VK cookies** (минимум `remixsid`). Получить удобнее всего через десктопное приложение **[WhitelistBypass.Creator](https://github.com/kulikov0/whitelist-bypass/releases)** из репозитория [kulikov0/whitelist-bypass](https://github.com/kulikov0/whitelist-bypass/releases): войти в VK, экспортировать cookies и вставить в панель (или только значение `remixsid` из DevTools).

В панели также доступны: вход по паролю VK, загрузка `cookies-vk.json`, таблица активных звонков (живой / завершается / мёртвый), кнопка «Завершить» (`forceFinish`, строка удаляется после смерти звонка). Данные хранятся в `/etc/wdtt/panel.db` (`vk_cookies`, `vk_calls`).

## Компоненты

| Компонент | Описание |
|-----------|----------|
| **wdtt** | Unified: DTLS + WG + панель `:2860` + подписка `:2096` (один процесс `wdtt-app`) |
| **wdtt-server** | Только VPN (legacy) |
| **wdtt-panel** | Только панель (legacy) |
| **wdtt-xray** | Redirect трафика `wdtt0` → outbound (NL, WARP…) |

Конфиг VPN и панели: `/etc/wdtt/panel.db` (SQLite — inbound, users, settings).  
Systemd `wdtt.service` запускает только `/usr/local/bin/wdtt-app -config-dir /etc/wdtt`.  
Xray: `/etc/wdtt-xray/config.json`.

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

Флаги CLI (fallback; в production параметры VPN — в **panel.db** / Подключения):  
`-config-dir`, `-no-panel`, `-listen`, `-wg-port`, `-password`, `-admin-addr`, …  
См. **[docs/SERVER.md](docs/SERVER.md)**.

## API панели

Документация: **[docs/API.md](docs/API.md)** — cookie-сессии, пользователи, inbound, Xray, формат ссылок `wdtt://`.

## Репозитории

| Репозиторий | Назначение |
|-------------|------------|
| [ildarmaga/wdtt](https://github.com/ildarmaga/wdtt) | Сервер + панель + **PWDTT Client** (`pwdtt-client-*` в [релизах](https://github.com/ildarmaga/wdtt/releases/latest)) |
| [ildarmaga/pwdtt-client](https://github.com/ildarmaga/pwdtt-client) | Исходники десктопного клиента PWDTT (модифицированная версия [luminescq/PWDTT](https://github.com/luminescq/PWDTT), GPL-3.0) |
| [ildarmaga/wdtt-install](https://github.com/ildarmaga/wdtt-install) | Установщик одной строкой |
| [anton48/vk-turn-proxy-ios](https://github.com/anton48/vk-turn-proxy-ios) | iOS-клиент |
| [amurcanov/proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android) | Android-клиент / исходный VPN-протокол |
| [kulikov0/whitelist-bypass](https://github.com/kulikov0/whitelist-bypass) | WhitelistBypass.Creator — получение VK cookies для панели |

## Лицензия

[GNU GPL v3](LICENSE) — как upstream [proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android).
