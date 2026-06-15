#!/bin/bash
# Сборка wdtt-panel для Linux (модуль wdtt-panel)
set -euo pipefail
cd "$(dirname "$0")"

if [[ "${1:-}" == /* ]] || [[ "${1:-}" == ./* ]]; then
  OUT="$1"
  GOARCH="${GOARCH:-amd64}"
elif [[ "${1:-}" == amd64 || "${1:-}" == arm64 ]]; then
  GOARCH="$1"
  OUT="../wdtt-panel-linux-${GOARCH}"
else
  GOARCH=amd64
  OUT="${1:-../wdtt-panel-linux-amd64}"
fi

VERSION="${WDTT_VERSION:-}"
if [[ -z "$VERSION" ]]; then
  VERSION="$(git -C .. describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo dev)"
fi

LDFLAGS="-s -w -X wdtt-panel.panelVersion=${VERSION}"
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" \
  go build -trimpath -ldflags="${LDFLAGS}" -o "$OUT" ./cmd
chmod +x "$OUT"
echo "OK: $OUT (v${VERSION})"
