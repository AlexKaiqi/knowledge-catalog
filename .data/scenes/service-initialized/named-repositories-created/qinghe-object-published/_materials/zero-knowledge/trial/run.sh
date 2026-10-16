#!/usr/bin/env bash
# Snapshot/Object trial only. The operator's user .env is read without sourcing shell code.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
exec python3 "$HERE/run.py" "$@"
