#!/bin/bash
# Локальный деплой на ЭТОМ сервере.
# systemd wdtt.service запускает /usr/local/bin/wdtt-app (unified), CLI — /usr/local/bin/wdtt
set -euo pipefail
cd "$(dirname "$0")"

ARCH="${1:-amd64}"
OUT="wdtt-linux-${ARCH}"
DEST="/usr/local/bin/wdtt-app"

./build.sh "$ARCH" unified

if [ "$(id -u)" -ne 0 ]; then
	echo "Нужен root: sudo $0 $*"
	exit 1
fi

EXEC=$(systemctl show wdtt -p ExecStart --value 2>/dev/null || true)
if [ -n "$EXEC" ] && ! echo "$EXEC" | grep -q "$DEST"; then
	echo "ВНИМАНИЕ: wdtt.service не использует $DEST"
	echo "  ExecStart=$EXEC"
fi

systemctl stop wdtt
install -m 0755 "$OUT" "$DEST"
systemctl start wdtt
sleep 2

if systemctl is-active --quiet wdtt; then
	echo "OK: $DEST ← $OUT ($(md5sum "$OUT" | awk '{print $1}'))"
	journalctl -u wdtt --no-pager -n 2 | grep СТАТ || true
else
	echo "FAIL: wdtt не запустился"
	journalctl -u wdtt --no-pager -n 10
	exit 1
fi
