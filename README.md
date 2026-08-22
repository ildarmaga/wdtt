# WDTT

VPN на VPS через медиарелеи ВК (TURN) с веб-панелью и опциональным **Xray**-маршрутизатором.

```
wdtt/
├── server/                             # wdtt-server (DTLS + userspace WG + NAT)
├── panel/                              # wdtt-panel (UI в стиле 3x-ui)
├── pkg/                                # sharelink, vkhash, paneldb (общее)
├── Docs/                               # Документация API
├── deploy.sh
└── docs/                               # SERVER.md (разбор)
```

## Происхождение

Серверная часть основана на проекте **[proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android)** (автор [amurcanov](https://github.com/amurcanov)) — WireGuard-туннель через DTLS-медиарелеи ВК.

WDTT расширяет upstream: мультипользователи, веб-панель, Xray, ссылки `wdtt://`, установщик. Подробности: **[CREDITS.md](CREDITS.md)**

## Быстрый старт

### Требования

- VPS с публичным IP (Ubuntu 20.04+, Debian 11+, CentOS 8+, Fedora, Arch)
- Root-доступ
- Открытые UDP-порты: 56000 (DTLS), 56001 (WG), 56003 (RAW)

### Установка одной строкой

```bash
bash <(curl -Ls https://raw.githubusercontent.com/ildarmaga/wdtt-install/main/install.sh)
```

Установщик автоматически:
- Скачает бинарник `wdtt-app` из GitHub Releases
- Настроит NAT, firewall, sysctl
- Создаст systemd-сервис `wdtt.service`
- Сгенерирует пароль панели

После установки:
```
Панель: http://IP:2860/wdtt/
Логин:  admin
Пароль: выводится в конце установки (или в /etc/wdtt/install-main-password.env)
```

### Установка из исходников

```bash
# Клонировать репозиторий
git clone https://github.com/ildarmaga/wdtt.git
cd wdtt

# Собрать единый бинарник (server + panel)
./build.sh amd64 unified

# Установить и запустить
sudo ./install-local.sh amd64
```

### Первые шаги после установки

1. **Откройте панель** — `http://IP:2860/wdtt/`
2. **Смените пароль** — Settings → Account → измените логин и пароль
3. **Настройте VPN** — Подключения → укажите порты, DNS, MTU
4. **Создайте пользователя** — Пользователи → Добавить (или автоматически через «Пакетное создание»)
5. **Скопируйте ссылку** — отправьте клиенту `wdtt://...` для подключения

### Создание API-токена

Для программного доступа к панели:

1. Откройте **Settings → API Tokens**
2. Нажмите **Создать токен**
3. Выберите скоуп: `admin` (полный доступ) или `readonly` (только чтение)
4. Сохраните токен — он показывается только один раз

```bash
# Использование
curl -H "Authorization: Bearer wdtt_..." http://IP:2860/wdtt/api/users
```

Документация API: **[Docs/Api.md](Docs/Api.md)**

### Управление сервисом

```bash
systemctl status wdtt      # Статус
systemctl restart wdtt      # Перезапуск
systemctl stop wdtt         # Остановка
journalctl -u wdtt -f       # Логи в реальном времени
```

### Обновление

```bash
# Скачать новую версию
curl -fsSL -o /usr/local/bin/wdtt-app \
  https://github.com/ildarmaga/wdtt/releases/latest/download/wdtt-linux-amd64
chmod +x /usr/local/bin/wdtt-app

# Перезапустить
systemctl restart wdtt
```

### Удаление

```bash
bash deploy.sh uninstall
```

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

Полноценный REST API с аутентификацией через API-ключи (Bearer Token).

```bash
# Использование
curl -H "Authorization: Bearer wdtt_..." https://panel.example.com/api/users
```

Документация: **[Docs/Api.md](Docs/Api.md)** — управление пользователями, инбаунды, статистика, Xray, сертификаты, firewall, настройки, API-токены.

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
