#!/bin/bash
# Maintainer: скачать бинарники клиента и положить в release-assets/ (для релизов WDTT).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/release-assets}"
REPO="${PWDtt_CLIENT_REPO:-ildarmaga/pwdtt-client}"
TAG="${PWDtt_CLIENT_TAG:-}"

if [[ -z "${GH_TOKEN:-}" ]] && command -v gh >/dev/null 2>&1; then
  GH_TOKEN="$(gh auth token 2>/dev/null || true)"
  export GH_TOKEN
fi
if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "GH_TOKEN not set — trying unauthenticated download (public repo)" >&2
fi

mkdir -p "$OUT/client-tmp"
dl=(release download --repo "$REPO" --dir "$OUT/client-tmp" -p wdtt-linux-amd64 -p wdtt-windows-amd64.exe)
if [[ -n "$TAG" ]]; then
  dl+=("$TAG")
fi
gh "${dl[@]}"

mv -f "$OUT/client-tmp/wdtt-linux-amd64" "$OUT/pwdtt-client-linux-amd64"
mv -f "$OUT/client-tmp/wdtt-windows-amd64.exe" "$OUT/pwdtt-client-windows-amd64.exe"
chmod +x "$OUT/pwdtt-client-linux-amd64"
rm -rf "$OUT/client-tmp"

if [[ -n "$TAG" ]]; then
  CLIENT_TAG="$TAG"
else
  CLIENT_TAG="$(gh release view --repo "$REPO" --json tagName -q .tagName)"
fi
echo "OK: pwdtt-client (${CLIENT_TAG}) -> $OUT/pwdtt-client-linux-amd64, pwdtt-client-windows-amd64.exe" >&2
echo "${CLIENT_TAG}" > "$OUT/pwdtt-client-version"
echo "${CLIENT_TAG}"
