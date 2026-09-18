#!/usr/bin/env bash
set -euo pipefail

export KC_SERVER_URL="${KC_SERVER_URL:-http://kc-server:7380}"
export KC_CATALOG="${KC_CATALOG:-kr://acme/catalog}"
unset KC_AS

mkdir -p /workspace
cd /workspace
ttyd_args=(--port 7682 --interface 0.0.0.0 -W)
if [[ -n "${KC_TTYD_CREDENTIAL:-}" ]]; then
  ttyd_args+=(-c "${KC_TTYD_CREDENTIAL}")
fi
exec ttyd "${ttyd_args[@]}" bash --login
