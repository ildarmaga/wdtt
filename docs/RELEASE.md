# GitHub Release (CI)

При теге `v*` workflow `.github/workflows/build-server.yml` собирает `wdtt-linux-amd64` / `arm64` и прикрепляет **PWDTT Client** из [ildarmaga/pwdtt-client](https://github.com/ildarmaga/pwdtt-client).

## Файлы в релизе

| Файл | Назначение |
|------|------------|
| `wdtt-linux-amd64` | Сервер + панель (VPS) |
| `wdtt-linux-arm64` | Сервер + панель (ARM) |
| `pwdtt-client-linux-amd64` | Клиент PWDTT для пользователей (Linux) |
| `pwdtt-client-windows-amd64.exe` | Клиент PWDTT для пользователей (Windows) |

Описание релиза дополняется блоком из `docs/RELEASE_CLIENT.md`.

## Секрет для CI (обязательно)

Репозиторий **pwdtt-client** — private. В **Settings → Secrets → Actions** репозитория `wdtt` добавьте:

- **`PWDTT_CLIENT_GH_TOKEN`** — fine-grained PAT (или classic) с правом **Contents: Read** на `ildarmaga/pwdtt-client`.

Без секрета job `release` упадёт на шаге скачивания клиента.

Локально: достаточно `gh auth login` — скрипт `scripts/fetch-pwdtt-client.sh` возьмёт токен из `gh`.

## Локальная проверка

```bash
./scripts/fetch-pwdtt-client.sh dist
ls dist/pwdtt-client-*
```
