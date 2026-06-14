# Cursor в репозитории WDTT

## В git (для всех)

| Путь | Назначение |
|------|------------|
| `.cursor/rules/wdtt/*.mdc` | Правила проекта: структура, paneldb, сборка |
| `.cursor/README.md` | Этот файл |

## Только локально (не в GitHub)

В `.gitignore` — всё остальное под `.cursor/`:

| Компонент | Путь | Что делает |
|-----------|------|------------|
| **ECC rules** | `.cursor/rules/common-*.mdc`, `golang-*.mdc`, … | Общие правила ECC (~100 файлов) |
| **Skills** | `.cursor/skills/` | Agent skills (TDD, golang-patterns, …) |
| **Hooks** | `.cursor/hooks.json`, `.cursor/hooks/` | sessionStart, beforeShell, afterFileEdit, … |
| **MCP / state** | `.cursor/ecc-install-state.json` | Состояние установки ECC |

Установка ECC локально: skill `configure-ecc` или свой скрипт — **не коммитить**.

## MCP

MCP-серверы настраиваются в **Cursor Settings → MCP** (user/project), не в этом репо.
Пример: `cursor-ide-browser` для UI — конфиг IDE, не `wdtt/.cursor/`.
