#!/bin/bash
# Локальный деплoy на ЭТОМ сервере.
# systemd wdtt.service → /usr/local/bin/wdtt-app (unified: panel + VPN в одном процессе)
# VPN-параметры (порты, DNS, лимиты) — в /etc/wdtt/panel.db, не в unit
set -euo pipefail
cd "$(dirname "$0")"

ARCH="${1:-amd64}"
OUT="wdtt-linux-${ARCH}"
DEST="/usr/local/bin/wdtt-app"
UNIT="/etc/systemd/system/wdtt.service"

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
sleep 3

if systemctl is-active --quiet wdtt; then
	echo "OK: $DEST ← $OUT ($(md5sum "$OUT" | awk '{print $1}'))"
	if [ -f "$UNIT" ] && grep -q '\-listen ' "$UNIT" 2>/dev/null; then
		echo "Подсказка: unit ещё с legacy -listen; откройте панель → Подключения → Сохранить (или перезапустите wdtt после миграции unit)"
	fi
	if grep -q 'ExecStart=.*-config-dir' "$UNIT" 2>/dev/null; then
		echo "Unit: минимальный ExecStart (параметры VPN в panel.db)"
	fi
	journalctl -u wdtt --no-pager -n 3 | grep -E 'СТАТ|Panel|упрощён' || true
else
	echo "FAIL: wdtt не запустился"
	journalctl -u wdtt --no-pager -n 10
	exit 1
fi
