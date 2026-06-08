# WDTT

Монорепозиторий: **VPN-сервер** + **веб-панель** для VPS.

```
wdtt/
├── server.go, deploy.sh, build.sh   # wdtt-server
└── panel/                           # wdtt-panel (3x-ui style)
```

## Быстрая установка

```bash
bash <(curl -Ls https://raw.githubusercontent.com/ildarmaga/wdtt-install/main/install.sh) install -p YOUR_PASSWORD --xray --panel
```

## Сборка вручную

```bash
# Сервер
./build.sh amd64
sudo install -m 0755 wdtt-server-linux-amd64 /usr/local/bin/wdtt-server

# Панель
chmod +x panel/build.sh
./panel/build.sh /usr/local/bin/wdtt-panel

# Деплой сервера
sudo bash deploy.sh install
```

## Компоненты

### wdtt-server (корень репозитория)

- DTLS `:56000/udp`, WireGuard `wdtt0` (`10.66.66.0/24`)
- Пароли, лимиты трафика, Telegram-бот
- Конфиг: `/etc/wdtt/passwords.json`

### wdtt-panel (`panel/`)

- Дашборд, пользователи, настройки Xray
- Порт `2860`, путь `/wdtt/`
- Логин по умолчанию: `admin` / `wdtt`

## Установщик

Отдельный репозиторий [wdtt-install](https://github.com/ildarmaga/wdtt-install) — одна строка для VPS + Xray + панель.

## Лицензия

См. [LICENSE](LICENSE).
