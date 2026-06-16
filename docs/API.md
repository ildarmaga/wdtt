# WDTT Panel API

Базовый URL панели: `https://<IP>:2860/wdtt/` (порт и путь настраиваются в `panel.db`; при включённом SSL HTTP автоматически редиректит на HTTPS).

Все эндпоинты ниже — относительно `https://<IP>:2860/wdtt/`.

## Аутентификация

1. **Логин:** `POST /login`  
   Тело (form или JSON, как в UI): `username`, `password`  
   При успехе устанавливается cookie `wdtt-panel`.

2. **Дальнейшие запросы:** cookie `wdtt-panel` обязателен (сессия).

3. **Выход:** `POST /logout/`

Пример с curl (после логина cookie в файле):

```bash
BASE="https://127.0.0.1:2860/wdtt"
curl -c /tmp/wdtt.cookie -X POST "$BASE/login" \
  -d "username=admin&password=wdtt"

curl -b /tmp/wdtt.cookie "$BASE/panel/api/status"
```

## Формат ответов

Успех:

```json
{ "success": true, "obj": { ... } }
```

Ошибка:

```json
{ "success": false, "msg": "описание ошибки" }
```

---

## Статус и мониторинг

### `GET /panel/api/status`

Сводка: сервисы, IP, статистика WDTT.

**Ответ `obj`:**

| Поле | Тип | Описание |
|------|-----|----------|
| `wdtt_active` | bool | `wdtt.service` запущен |
| `xray_active` | bool | `wdtt-xray.service` запущен |
| `wdtt_iface` | string | Адрес `wdtt0` |
| `server_ip` | string | Публичный IP |
| `main_password` | string | Главный пароль VPN |
| `users_count` | int | Число пользователей в БД |
| `stats` | object | `server.log` — онлайн, трафик, uptime |

---

## Подключения (WDTT Inbound)

### `GET /panel/api/inbound`

Текущие настройки входа + статус.

**Ответ `obj`:** `tag`, `remark`, `listen_host`, `server_host`, `dtls_port`, `wg_port`, `client_port`, `dns`, `max_users`, `service_active`, `iface_up`, `dtls_listening`, `wg_listening`, `active_users`, `online_users`, `xray_active`, …

### `POST /panel/api/inbound/save`

Сохранить inbound и перезапустить WDTT (+ Xray при необходимости).

**Тело:**

```json
{
  "tag": "wdtt-in",
  "remark": "WDTT",
  "listen_host": "0.0.0.0",
  "server_host": "",
  "dtls_port": 56000,
  "wg_port": 56001,
  "client_port": 9000,
  "dns": "1.1.1.1",
  "max_users": 10
}
```

---

## Пользователи VPN

### `GET /panel/api/users`

Список пользователей + inbound для ссылок.

**Ответ `obj`:**

```json
{
  "main_password": "...",
  "users": [
    {
      "password": "abc123",
      "comment": "Иван",
      "device_ids": ["uuid-1"],
      "devices_bound": 1,
      "max_devices": 3,
      "active": true,
      "online": false,
      "expires": "бессрочно",
      "total_gb": 0,
      "traffic_used_fmt": "1.2 GB",
      "link": "wdtt://..."
    }
  ],
  "inbound": { "dtls_port": 56000, "wg_port": 56001, ... }
}
```

### `POST /panel/api/users/add`

Создать пользователя.

**Тело (все поля опциональны):**

```json
{
  "password": "",
  "comment": "новый",
  "expires_at": 0,
  "total_gb": 0,
  "max_down_mbps": 0,
  "max_up_mbps": 0,
  "max_devices": 1,
  "active": true,
  "count": 1
}
```

- `password` пустой → автогенерация  
- только `count` → массовое создание паролей  

**Ответ:** `{ "password": "..." }` или `{ "passwords": ["...", "..."] }`

### `POST /panel/api/users/update`

**Тело:**

```json
{
  "old_password": "старый",
  "password": "новый",
  "comment": "...",
  "expires_at": 1735689600,
  "total_gb": 50,
  "max_devices": 3,
  "device_ids": ["uuid-1"],
  "active": true,
  "max_down_mbps": 10,
  "max_up_mbps": 5
}
```

Удаление устройства из списка `device_ids` отвязывает его при сохранении.

### `POST /panel/api/users/reset-traffic`

```json
{ "password": "userpass" }
```

### `POST /panel/api/users/delete`

```json
{ "password": "userpass" }
```

### `POST /panel/api/password/main`

Сменить главный пароль VPN.

```json
{ "password": "newMainPass" }
```

---

## Сервисы

### `POST /panel/api/server/restartWdttService`

Перезапуск VPN (unified: in-process restart; legacy: `systemctl restart wdtt`).

```bash
curl -b cookie -X POST "$BASE/panel/api/server/restartWdttService"
```

### `POST /panel/api/server/restartXrayService`

```bash
curl -b cookie -X POST "$BASE/panel/api/server/restartXrayService"
```

Legacy `POST /panel/api/service/{wdtt|xray}/{restart|stop|start}` удалён с v1.4.21.

---

## Xray

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/panel/api/xray/config` | JSON-конфиг |
| POST | `/panel/api/xray/config` | Сохранить конфиг + restart xray |
| GET | `/panel/api/xray/versions` | Доступные версии Xray |
| POST | `/panel/api/xray/install/{tag}` | Установить версию |

Совместимые эндпоинты 3x-ui: `/panel/xray/*`, `/panel/setting/*` — см. исходники `panel/xray_handlers.go`.

---

## Ссылки подключения `wdtt://`

Формат (как `vmess://` в 3x-ui):

```
wdtt:// + base64(JSON)
```

JSON:

```json
{
  "v": "1",
  "ps": "WDTT",
  "tag": "wdtt-in",
  "add": "YOUR_SERVER_IP",
  "dtls": 56000,
  "wg": 56001,
  "lp": 9000,
  "id": "password"
}
```

Поле `did` (device id) не обязательно — устройства привязываются автоматически до лимита `max_devices`.

Генерация на сервере: `buildWdttShareLink()` в `panel/wdtt_link.go`.

---

## Файлы конфигурации

Primary — `/etc/wdtt/panel.db` (SQLite). При обновлении старые JSON в `/etc/wdtt/` импортируются в БД и удаляются (schema v5).

| Файл | Назначение |
|------|------------|
| `/etc/wdtt/panel.db` | Панель, users, inbound, xray meta/config |
| `/etc/wdtt-xray/config.json` | Xray routing (процесс xray читает с диска; панель синхронизирует из БД) |

---

## Примеры автоматизации

```bash
# Создать пользователя на 30 дней, 50 GB, 2 устройства
curl -b /tmp/wdtt.cookie -X POST "$BASE/panel/api/users/add" \
  -H "Content-Type: application/json" \
  -d '{
    "comment": "API user",
    "expires_at": '"$(date -d '+30 days' +%s)"',
    "total_gb": 50,
    "max_devices": 2
  }'

# Увеличить лимит активных пользователей и DNS
curl -b /tmp/wdtt.cookie -X POST "$BASE/panel/api/inbound/save" \
  -H "Content-Type: application/json" \
  -d '{"dns":"1.1.1.1","max_users":20,"dtls_port":56000,"wg_port":56001,"client_port":9000}'
```

---

## Ограничения API

- Нет отдельного API-токена — только сессия панели.
- Unified (`wdtt-app`): изменение inbound/users применяется через hot-reload или in-process restart VPN; панель не останавливается. `wdtt.service` содержит только `-config-dir`.
- Rate-limit на стороне API не реализован — не публикуйте панель в открытый интернет без HTTPS и смены пароля по умолчанию.
