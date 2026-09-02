#!/usr/bin/env bash
set -euo pipefail

export KC_SERVER_URL="${KC_SERVER_URL:-http://kc-server:7380}"
export KC_CATALOG="${KC_CATALOG:-kr://dw/catalog}"
export KC_WORKSPACE="${KC_WORKSPACE:-warehouse-agent}"
unset KC_AS

mkdir -p /workspace
cd /workspace

args=(--port 7681 --interface 0.0.0.0 --writable)
if [[ -n "${KC_DW_CLI_CREDENTIAL:-}" ]]; then
  args+=(--credential "${KC_DW_CLI_CREDENTIAL}")
fi

exec ttyd "${args[@]}" bash --login
