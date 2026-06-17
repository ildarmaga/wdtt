# GitHub Release (CI)

При теге `v*` workflow `.github/workflows/build-server.yml` собирает `wdtt-linux-amd64` / `arm64` и прикрепляет **PWDTT Client** из `release-assets/` (бинарники лежат в репозитории, без внешних скачиваний в CI).

## Файлы в релизе

| Файл | Назначение |
|------|------------|
| `wdtt-linux-amd64` | Сервер + панель (VPS) |
| `wdtt-linux-arm64` | Сервер + панель (ARM) |
| `pwdtt-client-linux-amd64` | Клиент PWDTT для пользователей (Linux) |
| `pwdtt-client-windows-amd64.exe` | Клиент PWDTT для пользователей (Windows) |

Описание релиза дополняется блоком из `docs/RELEASE_CLIENT.md`.

## Обновление клиента в репозитории

Бинарники хранятся в `release-assets/`. Перед релизом (если вышла новая версия клиента):

```bash
./scripts/fetch-pwdtt-client.sh release-assets
echo vX.Y.Z > release-assets/pwdtt-client-version
git add release-assets/
```

Локально для `fetch-pwdtt-client.sh` нужен `gh auth login` (доступ maintainer'а к исходному репо клиента). Пользователям скачивать оттуда не нужно — только из [релизов WDTT](https://github.com/ildarmaga/wdtt/releases).
