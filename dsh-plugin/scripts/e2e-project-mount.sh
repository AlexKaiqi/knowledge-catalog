#!/usr/bin/env bash
# Compatibility entrypoint for the host-filesystem mount acceptance.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
exec "$ROOT/scripts/e2e-kcfs-linux.sh"
