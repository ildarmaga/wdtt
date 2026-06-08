#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
OUT="${1:-/usr/local/bin/wdtt-panel}"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$OUT" .
echo "OK: $OUT"
