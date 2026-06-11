# WDTT

VPN на VPS: **DTLS + WireGuard** (userspace) с веб-панелью и опциональным **Xray**-маршрутизатором.

```
wdtt/
├── server.go, devices.go, deploy.sh   # wdtt-server
├── panel/                             # wdtt-panel (UI в стиле 3x-ui)
└── docs/API.md                        # REST API панели
```

## Происхождение

Серверная часть основана на проекте **[proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android)** (автор [amurcanov](https://github.com/amurcanov)) — WireGuard-туннель через DTLS-медиарелей ВК.

WDTT расширяет upstream: мультипользователи, панель, Xray, ссылки `wdtt://`, Telegram-бот, установщик.

Подробности: **[CREDITS.md](CREDITS.md)**

## Быстрая установка

```bash
bash <(curl -Ls https://raw.githubusercontent.com/ildarmaga/wdtt-install/main/install.sh) install -p YOUR_PASSWORD --xray --panel
```

Панель: `http://IP:2860/wdtt/` — `admin` / `wdtt`

## Клиенты

| Платформа | Клиент | Ссылки |
|-----------|--------|--------|
| **iOS** | [vk-turn-proxy-ios](https://github.com/anton48/vk-turn-proxy-ios) | TestFlight / IPA из Releases, поддержка `wdtt://` |
| **Android** | [proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android) | Совместим с протоколом WDTT |

Сервер совместим с iOS-клиентом [anton48/vk-turn-proxy-ios](https://github.com/anton48/vk-turn-proxy-ios) — DTLS + TURN relay + WireGuard, ссылки `wdtt://`.

## Компоненты

| Компонент | Описание |
|-----------|----------|
| **wdtt-server** | DTLS `:56000`, WG `wdtt0` (`10.66.66.0/24`), пароли, лимиты |
| **wdtt-panel** | Дашборд, подключения, пользователи, Xray, настройки |
| **wdtt-xray** | Redirect трафика `wdtt0` → outbound (NL, warp…) |

Конфиг: `/etc/wdtt/`, `/etc/wdtt-xray/config.json`

## Сборка

```bash
./build.sh amd64
sudo install -m 0755 wdtt-server-linux-amd64 /usr/local/bin/wdtt-server

chmod +x panel/build.sh
./panel/build.sh /usr/local/bin/wdtt-panel
```

## API панели

Документация: **[docs/API.md](docs/API.md)**

- Аутентификация через cookie сессии
- Пользователи, inbound, сервисы, Xray
- Формат ссылок `wdtt://base64(JSON)`

## Репозитории

| Репозиторий | Назначение |
|-------------|------------|
| [ildarmaga/wdtt](https://github.com/ildarmaga/wdtt) | Сервер + панель (этот репо) |
| [ildarmaga/wdtt-install](https://github.com/ildarmaga/wdtt-install) | Установщик одной строкой |
| [anton48/vk-turn-proxy-ios](https://github.com/anton48/vk-turn-proxy-ios) | iOS-клиент (совместим с WDTT) |
| [amurcanov/proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android) | Android-клиент / исходный VPN-протокол |

## Лицензия

[GNU GPL v3](LICENSE) — как upstream [proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android).
