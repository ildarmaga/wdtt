package panel

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	acmeCronScriptPath   = "/usr/local/bin/wdtt-acme-cron.sh"
	acmeCronFilePath     = "/etc/cron.d/wdtt-acme"
	acmeCronLogPath      = "/var/log/wdtt-acme-cron.log"
	acmeCronDefaultHour  = 3
	acmeCronDefaultMinute = 12
)

var acmeCronLineRe = regexp.MustCompile(`(?m)^(\d{1,2})\s+(\d{1,2})\s+\*\s+\*\s+\*\s+\S+\s+.*wdtt-acme-cron\.sh`)

func acmeCronScript() string {
	return `#!/bin/bash
set -euo pipefail
export HOME=/root
ACME="/root/.acme.sh/acme.sh"
LOG="` + acmeCronLogPath + `"
OPENED=0

cleanup() {
  if [[ "$OPENED" == "1" ]] && command -v ufw >/dev/null 2>&1; then
    if ufw status 2>/dev/null | grep -qi 'Status: active'; then
      ufw deny 80/tcp comment "WDTT_ACME" >/dev/null 2>&1 || true
      while ufw status numbered 2>/dev/null | grep -qE '^\[[[:space:]]*[0-9]+\][[:space:]]+80/tcp[[:space:]]+ALLOW'; do
        id="$(ufw status numbered 2>/dev/null | grep -E '^\[[[:space:]]*[0-9]+\][[:space:]]+80/tcp[[:space:]]+ALLOW' | head -1 | sed -E 's/^\[[[:space:]]*([0-9]+)\].*/\1/')"
        [[ -n "$id" ]] || break
        ufw --force delete "$id" >/dev/null 2>&1 || break
      done
    fi
  fi
}
trap cleanup EXIT

if [[ ! -x "$ACME" ]]; then
  echo "$(date -Is) acme.sh not found" >> "$LOG"
  exit 0
fi

if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi 'Status: active'; then
  if ! ufw status numbered 2>/dev/null | grep -qE '80/tcp[[:space:]]+ALLOW'; then
    ufw allow 80/tcp comment "WDTT_ACME" >/dev/null
    OPENED=1
  fi
fi

"$ACME" --cron --home "/root/.acme.sh" >> "$LOG" 2>&1
echo "$(date -Is) acme cron done" >> "$LOG"

renew_dtls_if_needed() {
  cert="/etc/wdtt/dtls-cert.pem"
  [[ -f "$cert" ]] || return 0
  end_line="$(openssl x509 -enddate -noout -in "$cert" 2>/dev/null || true)"
  [[ -n "$end_line" ]] || return 0
  end_str="${end_line#notAfter=}"
  end_epoch="$(date -d "$end_str" +%s 2>/dev/null || true)"
  [[ -n "$end_epoch" ]] || return 0
  now="$(date +%s)"
  days=$(( (end_epoch - now) / 86400 ))
  if [[ "$days" -le 7 ]]; then
    rm -f /etc/wdtt/dtls-cert.pem /etc/wdtt/dtls-key.pem
    systemctl restart wdtt.service >> "$LOG" 2>&1 || true
    echo "$(date -Is) DTLS cert auto-renewed (${days}d left)" >> "$LOG"
  fi
}
renew_dtls_if_needed
`
}

func acmeCronInstalled() bool {
	data, err := os.ReadFile(acmeCronFilePath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "wdtt-acme-cron.sh")
}

func parseAcmeCronSchedule() (hour, minute int, ok bool) {
	hour, minute = acmeCronDefaultHour, acmeCronDefaultMinute
	data, err := os.ReadFile(acmeCronFilePath)
	if err != nil {
		return hour, minute, false
	}
	m := acmeCronLineRe.FindStringSubmatch(string(data))
	if len(m) < 3 {
		return hour, minute, false
	}
	minute, _ = strconv.Atoi(m[1])
	hour, _ = strconv.Atoi(m[2])
	return hour, minute, true
}

func acmeCronScheduleText(hour, minute int) string {
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

func normalizeAcmeCronTime(hour, minute int) (int, int) {
	if hour < 0 || hour > 23 {
		hour = acmeCronDefaultHour
	}
	if minute < 0 || minute > 59 {
		minute = acmeCronDefaultMinute
	}
	return hour, minute
}

func ensureAcmeCron() error {
	hour, minute := acmeCronDefaultHour, acmeCronDefaultMinute
	if acmeCronInstalled() {
		if h, m, ok := parseAcmeCronSchedule(); ok {
			hour, minute = h, m
		}
	}
	return ensureAcmeCronAt(hour, minute)
}

func ensureAcmeCronAt(hour, minute int) error {
	hour, minute = normalizeAcmeCronTime(hour, minute)
	script := acmeCronScript()
	if err := os.WriteFile(acmeCronScriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("write cron script: %w", err)
	}
	cronBody := fmt.Sprintf(`# WDTT ACME auto-renew: ufw allow 80 → acme.sh --cron → ufw deny 80
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
HOME=/root
%d %d * * * root %s
`, minute, hour, acmeCronScriptPath)
	if err := os.WriteFile(acmeCronFilePath, []byte(cronBody), 0644); err != nil {
		return fmt.Errorf("write cron.d: %w", err)
	}
	_ = os.MkdirAll(filepath.Dir(acmeCronLogPath), 0755)
	touch, _ := os.OpenFile(acmeCronLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if touch != nil {
		touch.Close()
	}
	return nil
}

func removeAcmeCron() error {
	if err := os.Remove(acmeCronFilePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func setAcmeCron(enabled bool, hour, minute int) error {
	if !enabled {
		return removeAcmeCron()
	}
	return ensureAcmeCronAt(hour, minute)
}
