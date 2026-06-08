#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
OUT="${1:-/usr/local/bin/wdtt-panel}"

VERSION="${WDTT_VERSION:-}"
if [[ -z "$VERSION" ]]; then
  VERSION="$(git -C .. describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo dev)"
fi

LDFLAGS="-s -w -X main.panelVersion=${VERSION}"
CGO_ENABLED=0 go build -trimpath -ldflags="${LDFLAGS}" -o "$OUT" .
echo "OK: $OUT (v${VERSION})"
