#!/usr/bin/env bash
set -euo pipefail

config=/var/lib/kc/deployment.json
[[ -f "$config" ]] || { echo "deployment.json is missing; bootstrap did not finish" >&2; exit 1; }
export KC_LAKEFS_CREDENTIAL="${KC_LAKEFS_ACCESS_KEY}:${KC_LAKEFS_SECRET_KEY}"
exec kc serve --config "$config" --listen 0.0.0.0:7380
