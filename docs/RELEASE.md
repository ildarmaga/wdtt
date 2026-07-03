# GitHub Release (CI)

При теге `v*` workflow `.github/workflows/build-server.yml` собирает `wdtt-linux-amd64` / `arm64` и скачивает **PWDTT Client** из [релиза pwdtt-client](https://github.com/ildarmaga/pwdtt-client/releases) (версия в `release-assets/pwdtt-client-version`). Бинарники клиента в репозитории wdtt больше не хранятся.

## Файлы в релизе

| Файл | Назначение |
|------|------------|
| `wdtt-linux-amd64` | Сервер + панель (VPS) |
| `wdtt-linux-arm64` | Сервер + панель (ARM) |
| `pwdtt-client-linux-amd64` | Клиент PWDTT для пользователей (Linux) |
| `pwdtt-client-windows-amd64.exe` | Клиент PWDTT для пользователей (Windows) |

Описание релиза дополняется блоком из `docs/RELEASE_CLIENT.md`.

## Обновление клиента перед релизом WDTT

1. В **pwdtt-client** — коммит, тег и push (сборка на GitHub Actions, сервер не нужен):

```bash
cd pwdtt-client
git tag vX.Y.Z
git push origin vX.Y.Z
# дождаться зелёного Build Desktop в Actions
```

2. В **wdtt** — указать версию клиента и выпустить сервер:

```bash
echo vX.Y.Z > release-assets/pwdtt-client-version
git add release-assets/pwdtt-client-version CHANGELOG.md
git tag vA.B.C && git push origin vA.B.C
```

CI wdtt скачает `wdtt-linux-amd64` и `wdtt-windows-amd64.exe` из релиза pwdtt-client.

Опционально локально (для проверки): `./scripts/fetch-pwdtt-client.sh release-assets`
