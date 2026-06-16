#!/bin/bash
# Bundle panel custom JS (csrf, axios-init, util, websocket) for fewer HTTP requests.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC_DIR="$ROOT/panel/web/assets/js"
OUT="$SRC_DIR/panel-core.min.js"
TMP="$(mktemp --suffix=.js)"
trap 'rm -f "$TMP"' EXIT

cat \
  "$SRC_DIR/csrf.js" \
  "$SRC_DIR/axios-init.js" \
  "$SRC_DIR/util/index.js" \
  "$SRC_DIR/websocket.js" \
  > "$TMP"

if command -v esbuild >/dev/null 2>&1; then
  esbuild "$TMP" --minify --outfile="$OUT" --allow-overwrite
else
  npx --yes esbuild "$TMP" --minify --outfile="$OUT" --allow-overwrite
fi

echo "OK: panel-core.min.js ($(wc -c < "$OUT") bytes)"
