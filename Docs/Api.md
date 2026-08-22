# WDTT Panel API

Полноценный REST API для управления панелью WDTT. Доступен по адресу `https://<your-panel>/panel/api/`.

## Аутентификация

### API-ключ (Bearer Token)

Для программного доступа используйте заголовок `Authorization`:

```
Authorization: Bearer wdtt_<token>
```

Токены создаются в панели: **Settings → API Tokens → Создать токен**.

### Скоупы токенов

| Скоуп | Описание |
|-------|----------|
| `admin` | Полный доступ ко всем эндпоинтам |
| `readonly` | Только GET-запросы (POST/PUT/DELETE возвращают 403) |

### Cookie-сессия (для веб-UI)

Для веб-интерфейса используется cookie `wdtt-panel` + CSRF-токен `X-CSRF-Token`. API-ключ работает параллельно, не заменяя cookie.

---

## Формат ответа

Все ответы в формате JSON:

```json
{
  "success": true,
  "msg": "сообщение",
  "obj": { ... }
}
```

- `success` — `true` при успехе, `false` при ошибке
- `msg` — текстовое сообщение
- `obj` — данные ответа (при успехе)

---

## Управление пользователями

### Список пользователей

```
POST /panel/api/users
```

Возвращает всех пользователей с информацией о трафике, устройствах, сроках действия.

**Пример:**
```bash
curl -X POST https://panel.example.com/api/users \
  -H "Authorization: Bearer wdtt_abc123..."
```

**Ответ:**
```json
{
  "success": true,
  "obj": {
    "users": [
      {
        "password": "aB3d****",
        "password_key": "aB3dEfGh",
        "comment": "Иван",
        "expires_at": 1735689600,
        "up_bytes": 1073741824,
        "down_bytes": 5368709120,
        "total_bytes": 107374182400,
        "active": true,
        "online": false,
        "last_seen_at": 1735600000,
        "link": "wdtt://..."
      }
    ],
    "devices": [...],
    "inbound": {...}
  }
}
```

### Создать пользователя

```
POST /panel/api/users/add
```

**Параметры (JSON):**

| Поле | Тип | Описание |
|------|-----|----------|
| `password` | string | Пароль (если пустой — генерируется автоматически) |
| `comment` | string | Комментарий/имя |
| `expires_at` | int64 | Unix timestamp окончания (0 = бессрочно) |
| `total_gb` | float64 | Лимит трафика в ГБ (0 = без лимита) |
| `max_down_mbps` | float64 | Лимит скорости загрузки (МБ/с) |
| `max_up_mbps` | float64 | Лимит скорости отдачи (МБ/с) |
| `max_devices` | int | Макс. количество устройств |
| `active` | bool | Активен ли пользователь |
| `ports` | string | Порты в формате "dtls,wg,tun" |
| `count` | int | Количество паролей для пакетного создания |

**Пример — создать одного пользователя:**
```bash
curl -X POST https://panel.example.com/api/users/add \
  -H "Authorization: Bearer wdtt_abc123..." \
  -H "Content-Type: application/json" \
  -d '{"comment": "Иван", "expires_at": 0, "total_gb": 100}'
```

**Пример — создать 5 пользователей:**
```bash
curl -X POST https://panel.example.com/api/users/add \
  -H "Authorization: Bearer wdtt_abc123..." \
  -H "Content-Type: application/json" \
  -d '{"count": 5, "comment": "batch", "total_gb": 50}'
```

### Обновить пользователя

```
POST /panel/api/users/update
```

**Параметры (JSON):**

| Поле | Тип | Описание |
|------|-----|----------|
| `old_password` | string | Текущий пароль (обязательно) |
| `password` | string | Новый пароль |
| `comment` | string | Новый комментарий |
| `expires_at` | int64 | Новый срок действия |
| `total_gb` | float64 | Новый лимит трафика |
| `max_down_mbps` | float64 | Новый лимит скорости ↓ |
| `max_up_mbps` | float64 | Новый лимит скорости ↑ |
| `active` | bool | Активность |
| `device_ids` | []string | Привязанные устройства |

**Пример:**
```bash
curl -X POST https://panel.example.com/api/users/update \
  -H "Authorization: Bearer wdtt_abc123..." \
  -H "Content-Type: application/json" \
  -d '{"old_password": "aB3dEfGh", "comment": "Новое имя", "total_gb": 200}'
```

### Сбросить трафик пользователя

```
POST /panel/api/users/reset-traffic
```

**Параметры (JSON):**

| Поле | Тип | Описание |
|------|-----|----------|
| `password` | string | Пароль пользователя (обязательно) |

**Пример:**
```bash
curl -X POST https://panel.example.com/api/users/reset-traffic \
  -H "Authorization: Bearer wdtt_abc123..." \
  -H "Content-Type: application/json" \
  -d '{"password": "aB3dEfGh"}'
```

### Удалить пользователя

```
POST /panel/api/users/delete
```

**Параметры (JSON):**

| Поле | Тип | Описание |
|------|-----|----------|
| `password` | string | Пароль пользователя (обязательно) |

**Пример:**
```bash
curl -X POST https://panel.example.com/api/users/delete \
  -H "Authorization: Bearer wdtt_abc123..." \
  -H "Content-Type: application/json" \
  -d '{"password": "aB3dEfGh"}'
```

---

## Инбаунды WDTT VPN

### Получить настройки инбаунда

```
GET /panel/api/inbound
```

**Пример:**
```bash
curl https://panel.example.com/api/inbound \
  -H "Authorization: Bearer wdtt_abc123..."
```

**Ответ:**
```json
{
  "success": true,
  "obj": {
    "tag": "wdtt-in",
    "dtls_port": 56000,
    "wg_port": 56001,
    "dns": "1.1.1.1",
    "mtu": 1280,
    "max_users": 10,
    "raw_enable": true,
    "raw_direct_port": 0
  }
}
```

### Сохранить настройки инбаунда

```
POST /panel/api/inbound/save
```

**Параметры (JSON):**

| Поле | Тип | Описание |
|------|-----|----------|
| `dtls_port` | int | Порт DTLS |
| `wg_port` | int | Порт WireGuard |
| `dns` | string | DNS для клиентов |
| `mtu` | int | MTU (576–1500) |
| `max_users` | int | Макс. пользователей |
| `raw_enable` | bool | Включить RAW-режим |
| `raw_direct_port` | int | Порт RAW (0 = DTLS+3) |

---

## Статистика и статус

### Общий статус

```
POST /panel/api/status
```

**Ответ:**
```json
{
  "success": true,
  "obj": {
    "wdtt_active": true,
    "xray_active": true,
    "wdtt_iface": "wdtt0",
    "xray_version": "Xray 25.1.1",
    "stats": {
      "active_users": 3,
      "sessions": 5,
      "total_up": "1.2 GB",
      "total_down": "5.4 GB"
    },
    "users_count": 10,
    "devices_count": 8,
    "server_ip": "1.2.3.4"
  }
}
```

### Статус сервера

```
POST /panel/api/server/status
```

**Ответ:**
```json
{
  "success": true,
  "obj": {
    "cpu": 12.5,
    "mem_used": 2147483648,
    "mem_total": 8589934592,
    "uptime": "3d 5h 12m",
    "xray_running": true,
    "wdtt_running": true
  }
}
```

### Логи

```
POST /panel/api/logs?n=50&service=wdtt&level=info
```

**Параметры (query):**

| Параметр | Описание |
|----------|----------|
| `n` | Количество строк (по умолчанию 50) |
| `service` | Сервис: `wdtt`, `xray`, `panel` |
| `level` | Уровень: `error`, `warning`, `info`, `debug` |
| `syslog` | `true` для syslog-формата |

---

## Управление Xray

### Получить настройки Xray

```
POST /panel/xray/
```

### Обновить настройки Xray

```
POST /panel/xray/update
```

**Параметры (form):**

| Поле | Описание |
|------|----------|
| `xraySetting` | JSON-конфиг xray |
| `outboundTestUrl` | URL для тестирования outbound |

### Тест outbound

```
POST /panel/xray/testOutbound
```

**Параметры (form):**

| Поле | Описание |
|------|----------|
| `outbound` | JSON outbound-конфига |
| `allOutbounds` | JSON массив всех outbounds (опционально) |

**Ответ:**
```json
{
  "success": true,
  "obj": {
    "success": true,
    "delay": 150,
    "statusCode": 204,
    "warnings": ["TPROXY diagnostic message"]
  }
}
```

### Версии Xray

```
POST /panel/api/xray/versions
```

### Установить версию Xray

```
POST /panel/api/xray/install/{tag}
```

**Пример:**
```bash
curl -X POST https://panel.example.com/api/xray/install/v25.1.1 \
  -H "Authorization: Bearer wdtt_abc123..."
```

### Текущий конфиг Xray

```
POST /panel/api/xray/config
```

---

## Управление сервером

### Перезапустить WDTT

```
POST /panel/api/server/restartWdttService
```

### Остановить WDTT

```
POST /panel/api/server/stopWdttService
```

### Перезапустить Xray

```
POST /panel/api/server/restartXrayService
```

### Остановить Xray

```
POST /panel/api/server/stopXrayService
```

### Обновить панель

```
POST /panel/api/server/installPanel/{tag}
```

### Скачать резервную копию БД

```
POST /panel/api/server/getDb
```

### Импорт БД

```
POST /panel/api/server/importDB
```

---

## Сертификаты

### Список сертификатов

```
POST /panel/api/certs/list
```

### Выпустить сертификат

```
POST /panel/api/certs/issue
```

**Параметры (JSON):**

| Поле | Описание |
|------|----------|
| `domain` | Доменное имя |
| `email` | Email для ACME |
| `applyToPanel` | Применить к панели |
| `restartPanel` | Перезапустить панель |

### Обновить сертификат

```
POST /panel/api/certs/renew
```

### Отозвать сертификат

```
POST /panel/api/certs/revoke
```

---

## Firewall

### Список портов

```
GET /panel/api/firewall/ports
```

### Открыть порт

```
POST /panel/api/firewall/open
```

**Параметры (JSON):**

| Поле | Описание |
|------|----------|
| `port` | Номер порта |
| `protocol` | `tcp`, `udp`, `tcp+udp` |
| `comment` | Комментарий |

### Закрыть порт

```
POST /panel/api/firewall/close
```

**Параметры (JSON):**

| Поле | Описание |
|------|----------|
| `port` | Номер порта |
| `protocol` | `tcp`, `udp`, `tcp+udp` |

---

## Настройки

### Все настройки

```
POST /panel/api/setting/all
```

### Обновить настройки

```
POST /panel/api/setting/update
```

### Сменить логин/пароль панели

```
POST /panel/api/setting/updateUser
```

**Параметры (JSON):**

| Поле | Описание |
|------|----------|
| `oldUsername` | Текущий логин |
| `oldPassword` | Текущий пароль |
| `newUsername` | Новый логин |
| `newPassword` | Новый пароль |

---

## API-токены

### Список токенов

```
POST /panel/api/tokens
```

**Ответ:**
```json
{
  "success": true,
  "obj": [
    {
      "id": 1,
      "token": "wdtt_a1b2c3d4...",
      "name": "monitoring-bot",
      "scope": "admin",
      "enabled": true,
      "created_at": 1735689600,
      "last_used": 1735690000
    }
  ]
}
```

> Токен в списке замаскирован (показаны только первые 12 символов).

### Создать токен

```
POST /panel/api/tokens/create
```

**Параметры (JSON):**

| Поле | Описание |
|------|----------|
| `name` | Имя токена |
| `scope` | `admin` или `readonly` |

**Ответ:**
```json
{
  "success": true,
  "obj": {
    "id": 2,
    "token": "wdtt_f8e7d6c5b4a39281706f5e4d3c2b1a0f9e8d7c6b5a49382716",
    "name": "my-bot",
    "scope": "admin",
    "enabled": true,
    "created_at": 1735689600,
    "last_used": 0
  }
}
```

> **Токен показан только один раз!** Сохраните его сразу.

### Удалить токен

```
POST /panel/api/tokens/delete
```

**Параметры (JSON):**

| Поле | Описание |
|------|----------|
| `id` | ID токена |

### Включить/выключить токен

```
POST /panel/api/tokens/toggle
```

**Параметры (JSON):**

| Поле | Описание |
|------|----------|
| `id` | ID токена |

---

## Примеры интеграции

### Python

```python
import requests

PANEL = "https://panel.example.com"
TOKEN = "wdtt_f8e7d6c5b4a3..."

headers = {"Authorization": f"Bearer {TOKEN}"}

# Список пользователей
r = requests.post(f"{PANEL}/api/users", headers=headers)
users = r.json()["obj"]["users"]

# Создать пользователя
r = requests.post(f"{PANEL}/api/users/add", headers=headers, json={
    "comment": "bot-user",
    "total_gb": 50,
    "expires_at": 0
})
print(r.json()["obj"]["password"])

# Статус сервера
r = requests.post(f"{PANEL}/api/server/status", headers=headers)
print(r.json()["obj"])
```

### JavaScript (Node.js)

```javascript
const axios = require('axios');

const PANEL = 'https://panel.example.com';
const TOKEN = 'wdtt_f8e7d6c5b4a3...';

const api = axios.create({
  baseURL: `${PANEL}/api`,
  headers: { 'Authorization': `Bearer ${TOKEN}` }
});

// Список пользователей
const { data } = await api.post('/users');
console.log(data.obj.users);

// Создать пользователя
const res = await api.post('/users/add', {
  comment: 'bot-user',
  total_gb: 50,
  expires_at: 0
});
console.log(res.data.obj.password);
```

### cURL

```bash
# Статус
curl -s -X POST https://panel.example.com/api/status \
  -H "Authorization: Bearer wdtt_..." | jq .

# Список пользователей
curl -s -X POST https://panel.example.com/api/users \
  -H "Authorization: Bearer wdtt_..." | jq '.obj.users[] | {password, comment, online}'

# Создать пользователя
curl -s -X POST https://panel.example.com/api/users/add \
  -H "Authorization: Bearer wdtt_..." \
  -H "Content-Type: application/json" \
  -d '{"comment":"test","total_gb":100}' | jq .

# Сбросить трафик
curl -s -X POST https://panel.example.com/api/users/reset-traffic \
  -H "Authorization: Bearer wdtt_..." \
  -H "Content-Type: application/json" \
  -d '{"password":"aB3dEfGh"}' | jq .
```

---

## Ошибки

| Код | Описание |
|-----|----------|
| 200 | Успешный запрос (даже при `success: false`) |
| 400 | Неверные параметры запроса |
| 401 | Не авторизован (невалидный токен или сессия) |
| 403 | Запрещено (readonly-токен пытается выполнить POST) |
| 404 | Эндпоинт не найден |
| 500 | Внутренняя ошибка сервера |

**Пример ответа при ошибке:**
```json
{
  "success": false,
  "msg": "invalid api token"
}
```
