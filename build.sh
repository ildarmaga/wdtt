#!/bin/bash
# Сборка wdtt-server, wdtt-panel и/или единого wdtt для Linux
set -euo pipefail
cd "$(dirname "$0")"

ARCH="${1:-amd64}"
TARGET="${2:-all}"

case "$ARCH" in
  amd64|arm64) ;;
  server|panel|all|unified)
    TARGET="$ARCH"
    ARCH=amd64
    ;;
  *)
    echo "Usage: $0 [amd64|arm64] [server|panel|all|unified]"
    exit 1
    ;;
esac

case "$TARGET" in
  server|panel|all|unified) ;;
  *)
    echo "Usage: $0 [amd64|arm64] [server|panel|all|unified]"
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
    go build -trimpath -ldflags="-s -w" -o "$out" ./server/cmd
  chmod +x "$out"
  echo "OK: $out"
}

build_panel() {
  chmod +x panel/build.sh
  panel/build.sh "$1"
}

build_unified() {
  local goarch="$1"
  local out
  case "$goarch" in
    amd64) out=wdtt-linux-amd64 ;;
    arm64) out=wdtt-linux-arm64 ;;
  esac
  local version="${WDTT_VERSION:-}"
  if [[ -z "$version" ]]; then
    version="$(git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo 1.4.0)"
  fi
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" \
    go build -trimpath -ldflags="-s -w -X wdtt-panel.panelVersion=${version}" -o "$out" ./cmd/wdtt
  chmod +x "$out"
  echo "OK: $out (server + panel, v${version})"
}

case "$TARGET" in
  server)  build_server "$ARCH" ;;
  panel)   build_panel "$ARCH" ;;
  unified) build_unified "$ARCH" ;;
  all)
    build_server "$ARCH"
    build_panel "$ARCH"
    build_unified "$ARCH"
    ;;
esac
