#!/usr/bin/env bash
# Compatibility entrypoint. DSH now uses its stock host filesystem tools;
# kcfs is mounted before DSH starts rather than injected as a DSH filesystem.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

(cd "$ROOT/dsh-plugin" && npm run typecheck && npm test)
exec "$ROOT/scripts/e2e-kcfs-linux.sh"
