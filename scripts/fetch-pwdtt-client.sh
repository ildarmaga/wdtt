#!/bin/bash
# Скачивает бинарники PWDTT Client из релиза ildarmaga/pwdtt-client.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/dist}"
REPO="${PWDtt_CLIENT_REPO:-ildarmaga/pwdtt-client}"
TAG="${PWDtt_CLIENT_TAG:-}"

mkdir -p "$OUT/client-tmp"
args=(release download --repo "$REPO" --dir "$OUT/client-tmp" -p 'wdtt-linux-amd64' -p 'wdtt-windows-amd64.exe')
if [[ -n "$TAG" ]]; then
  args+=( "$TAG" )
fi
gh "${args[@]}"

mv -f "$OUT/client-tmp/wdtt-linux-amd64" "$OUT/pwdtt-client-linux-amd64"
mv -f "$OUT/client-tmp/wdtt-windows-amd64.exe" "$OUT/pwdtt-client-windows-amd64.exe"
chmod +x "$OUT/pwdtt-client-linux-amd64"
rmdir "$OUT/client-tmp" 2>/dev/null || rm -rf "$OUT/client-tmp"

CLIENT_TAG="$(gh release view --repo "$REPO" ${TAG:+"$TAG"} --json tagName -q .tagName 2>/dev/null || echo latest)"
echo "OK: pwdtt-client ($CLIENT_TAG) -> $OUT/pwdtt-client-linux-amd64, pwdtt-client-windows-amd64.exe"
