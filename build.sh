#!/bin/bash
# Сборка wdtt-server для Linux
set -euo pipefail
cd "$(dirname "$0")"
ARCH="${1:-amd64}"
case "$ARCH" in
  amd64) GOARCH=amd64; OUT=wdtt-server-linux-amd64 ;;
  arm64) GOARCH=arm64; OUT=wdtt-server-linux-arm64 ;;
  *) echo "Usage: $0 [amd64|arm64]"; exit 1 ;;
esac
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" \
  go build -trimpath -ldflags="-s -w" -o "$OUT" server.go
chmod +x "$OUT"
echo "OK: $OUT"
