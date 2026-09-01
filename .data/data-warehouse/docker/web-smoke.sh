#!/usr/bin/env bash
set -euo pipefail

url="${1:-http://127.0.0.1:7400}"
dom="$(mktemp)"
browser_log="$(mktemp)"
trap 'rm -f "$dom" "$browser_log"' EXIT

timeout 15s chromium \
  --headless \
  --no-sandbox \
  --disable-dev-shm-usage \
  --disable-gpu \
  --timeout=5000 \
  --dump-dom "$url" >"$dom" 2>"$browser_log"

if grep -Eq 'Failed to load plugins|did not activate|waiting for service:|Loading plugins' "$dom"; then
  echo "DSH browser plugin boot did not complete" >&2
  sed -n '1,120p' "$browser_log" >&2
  exit 1
fi
grep -Eq 'DeepSeek Harness|HARNESS|选择工作区|Select workspace|Choose workspace' "$dom" || {
  echo "DSH browser shell was not rendered" >&2
  exit 1
}
