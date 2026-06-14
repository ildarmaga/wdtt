#!/bin/bash
# Сборка wdtt-server и/или wdtt-panel для Linux
set -euo pipefail
cd "$(dirname "$0")"

ARCH="${1:-amd64}"
TARGET="${2:-server}"

case "$ARCH" in
  amd64|arm64) ;;
  server|panel|all)
    TARGET="$ARCH"
    ARCH=amd64
    ;;
  *)
    echo "Usage: $0 [amd64|arm64] [server|panel|all]"
    exit 1
    ;;
esac

case "$TARGET" in
  server|panel|all) ;;
  *)
    echo "Usage: $0 [amd64|arm64] [server|panel|all]"
    exit 1
    ;;
esac

build_server() {
  local goarch="$1"
  local out
  case "$goarch" in
    amd64) out=wdtt-server-linux-amd64 ;;
    arm64) out=wdtt-server-linux-arm64 ;;
  esac
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" \
    go build -trimpath -ldflags="-s -w" -o "$out" ./server
  chmod +x "$out"
  echo "OK: $out"
}

build_panel() {
  chmod +x panel/build.sh
  panel/build.sh "$1"
}

case "$TARGET" in
  server) build_server "$ARCH" ;;
  panel)  build_panel "$ARCH" ;;
  all)
    build_server "$ARCH"
    build_panel "$ARCH"
    ;;
esac
