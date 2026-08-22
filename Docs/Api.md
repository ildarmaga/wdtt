# WDTT Panel API — API-ключи (Bearer Token)

Аутентификация через API-ключи для программного доступа к панели WDTT.

## Создание токена

1. Откройте панель → **Settings → API Tokens**
2. Нажмите **Создать токен**
3. Введите имя, выберите скоуп
4. Сохраните токен — он показывается **только один раз**

## Использование

Все запросы к API панели требуют заголовок `Authorization`:

```
Authorization: Bearer wdtt_<ваш_токен>
```

## Скоупы

| Скоуп | Описание |
|-------|----------|
| `admin` | Полный доступ (чтение + запись) |
| `readonly` | Только чтение (GET-запросы) |

## Примеры

### Список пользователей

```bash
curl -X POST http://IP:2860/wdtt/api/users \
  -H "Authorization: Bearer wdtt_abc123..."
```

### Создать пользователя

```bash
curl -X POST http://IP:2860/wdtt/api/users/add \
  -H "Authorization: Bearer wdtt_abc123..." \
  -H "Content-Type: application/json" \
  -d '{"comment": "my-user", "total_gb": 100}'
```

### Статус сервера

```bash
curl -X POST http://IP:2860/wdtt/api/status \
  -H "Authorization: Bearer wdtt_abc123..."
```

### Сбросить трафик

```bash
curl -X POST http://IP:2860/wdtt/api/users/reset-traffic \
  -H "Authorization: Bearer wdtt_abc123..." \
  -H "Content-Type: application/json" \
  -d '{"password": "userPassword"}'
```

### Python

```python
import requests

PANEL = "http://IP:2860/wdtt"
TOKEN = "wdtt_abc123..."

headers = {"Authorization": f"Bearer {TOKEN}"}

# Список пользователей
r = requests.post(f"{PANEL}/api/users", headers=headers)
users = r.json()["obj"]["users"]

# Создать пользователя
r = requests.post(f"{PANEL}/api/users/add", headers=headers, json={
    "comment": "bot-user",
    "total_gb": 50
})
print(r.json()["obj"]["password"])
```

### JavaScript

```javascript
const axios = require('axios');

const api = axios.create({
  baseURL: 'http://IP:2860/wdtt/api',
  headers: { 'Authorization': 'Bearer wdtt_abc123...' }
});

const { data } = await api.post('/users');
console.log(data.obj.users);
```

## Формат ответа

```json
{
  "success": true,
  "msg": "сообщение",
  "obj": { ... }
}
```

## Ошибки

| Код | Описание |
|-----|----------|
| 401 | Невалидный токен |
| 403 | Readonly-токен пытается выполнить POST |
| 400 | Неверные параметры |
| 500 | Ошибка сервера |

## Управление токенами через API

```bash
# Список токенов
curl -X POST http://IP:2860/wdtt/api/tokens \
  -H "Authorization: Bearer wdtt_..."

# Создать токен
curl -X POST http://IP:2860/wdtt/api/tokens/create \
  -H "Authorization: Bearer wdtt_..." \
  -H "Content-Type: application/json" \
  -d '{"name": "my-bot", "scope": "admin"}'

# Удалить токен
curl -X POST http://IP:2860/wdtt/api/tokens/delete \
  -H "Authorization: Bearer wdtt_..." \
  -H "Content-Type: application/json" \
  -d '{"id": 1}'

# Включить/выключить токен
curl -X POST http://IP:2860/wdtt/api/tokens/toggle \
  -H "Authorization: Bearer wdtt_..." \
  -H "Content-Type: application/json" \
  -d '{"id": 1}'
```
