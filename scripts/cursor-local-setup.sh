#!/bin/bash
# Локальная настройка Cursor (не коммитится в GitHub).
# Запуск из корня wdtt: ./scripts/cursor-local-setup.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CURSOR_USER="${HOME}/.cursor"

mkdir -p "${CURSOR_USER}/rules"

echo "→ rules: symlink wdtt rules → ${CURSOR_USER}/rules/"
for f in "${ROOT}/.cursor/rules/wdtt/"*.mdc; do
  [ -f "$f" ] || continue
  base=$(basename "$f")
  ln -sfn "$f" "${CURSOR_USER}/rules/wdtt-${base}"
  echo "  wdtt-${base}"
done

if [ -f "${ROOT}/.cursor/mcp.json" ]; then
  echo "→ mcp: merge ${ROOT}/.cursor/mcp.json → ${CURSOR_USER}/mcp.json"
  node -e "
const fs = require('fs');
const userPath = process.env.HOME + '/.cursor/mcp.json';
const projPath = '${ROOT}/.cursor/mcp.json';
const user = fs.existsSync(userPath) ? JSON.parse(fs.readFileSync(userPath, 'utf8')) : { mcpServers: {} };
const proj = JSON.parse(fs.readFileSync(projPath, 'utf8'));
user.mcpServers = { ...user.mcpServers, ...proj.mcpServers };
fs.writeFileSync(userPath, JSON.stringify(user, null, 2) + '\n');
console.log('  servers:', Object.keys(user.mcpServers).join(', '));
"
else
  echo "→ mcp: skip (no ${ROOT}/.cursor/mcp.json)"
fi

if [ -f "${ROOT}/.cursor/hooks.json" ] && [ -d "${ROOT}/.cursor/hooks" ]; then
  echo "→ hooks: symlink project hooks (работают при Open Folder = wdtt)"
  ln -sfn "${ROOT}/.cursor/hooks.json" "${CURSOR_USER}/hooks.json.wdtt"
  ln -sfn "${ROOT}/.cursor/hooks" "${CURSOR_USER}/hooks.wdtt"
  echo "  подсказка: открой wdtt.code-workspace — hooks из wdtt/.cursor/hooks.json"
fi

echo "OK. Перезагрузи окно Cursor: Command Palette → Developer: Reload Window"
