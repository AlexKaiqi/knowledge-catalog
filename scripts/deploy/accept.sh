#!/usr/bin/env bash
# Bind the existing local test stack. This entry never builds/starts services.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
export PATH="$HOME/.local/go/bin:$PATH"
PYTHON="$ROOT/.venv/bin/python"
if [[ ! -x "$PYTHON" ]]; then
  echo "FAIL: repository .venv Python is unavailable; no dependency was installed" >&2
  exit 2
fi
if [[ -z "${KC_VALIDATION_RUN_ID:-}" ]]; then
  exec "$PYTHON" "$ROOT/scripts/validation.py" run --scope deploy-local-accept -- "$0" "$@"
fi
exec "$PYTHON" "$ROOT/.data/scenes/accept.py" "$@"
