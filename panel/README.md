# wdtt-panel

Веб-панель управления WDTT VPN + Xray. Интерфейс в стиле [3x-ui](https://github.com/MHSanaei/3x-ui).

## Возможности

- **Дашборд** — CPU/RAM, статус WDTT/Xray, онлайн
- **Подключения** — inbound (порты, DNS, лимит пользователей), статус, QR в пользователях
- **Пользователи** — пароли, лимит устройств, трафик, ссылки `wdtt://`
- **Настройки Xray** — routing, outbounds, балансеры
- **Настройки панели** — порт, SSL, учётная запись

Протокол VPN основан на [proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android) — см. [CREDITS.md](../CREDITS.md).

## API

REST API для автоматизации: **[docs/API.md](../docs/API.md)**

## Установка

Часть монорепозитория [wdtt](https://github.com/ildarmaga/wdtt). Установка:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/ildarmaga/wdtt-install/main/install.sh) install -p PASSWORD --panel --xray
```

Сборка вручную:

```bash
cd panel
./build.sh /usr/local/bin/wdtt-panel
systemctl enable --now wdtt-panel.service
```

Панель: `http://IP:2860/wdtt/` — `admin` / `wdtt` (смените в настройках).

## Сборка

```bash
go build -trimpath -ldflags="-s -w" -o wdtt-panel .
```

Требуется Go 1.21+. UI встроен через `go:embed`.

## GitHub Release (опционально)

Для установки без Go на сервере публикуйте бинарник:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o wdtt-panel-linux-amd64 .
gh release create v1.0.0 wdtt-panel-linux-amd64
```

Установщик скачает `wdtt-panel-linux-amd64` из latest release.

## Конфигурация

Primary — `/etc/wdtt/panel.db` (SQLite). При обновлении JSON-файлы из `/etc/wdtt/` импортируются в БД и удаляются.

| Файл | Назначение |
|------|------------|
| `/etc/wdtt/panel.db` | Панель, users, inbound, xray meta/config |
| `/etc/wdtt-xray/config.json` | Конфиг Xray на диске (процесс xray; панель синхронизирует из БД) |

## Зависимости

- `wdtt.service` — VPN-сервер
- `wdtt-xray.service` — опционально, маршрутизация
