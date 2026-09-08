#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if [[ -z "${KC_VALIDATION_RUN_ID:-}" ]]; then
  exec python3 ./scripts/validation.py run --scope observability:rules -- "$repo_root/scripts/check-observability.sh" "$@"
fi

rules_dir="$repo_root/docs/observability"
if command -v promtool >/dev/null 2>&1; then
  runner=(promtool)
elif command -v docker >/dev/null 2>&1; then
  # Use an already cached tool image. This command never downloads an image or
  # starts a Prometheus service; the rule inputs remain read-only.
  runner=(docker run --rm --pull=never --network=none --entrypoint /bin/promtool
    --mount "type=bind,source=$rules_dir,target=/rules,readonly"
    --workdir /rules prom/prometheus:v3.5.0)
else
  printf '%s\n' 'observability checks need local promtool or a cached prom/prometheus:v3.5.0 tool image' >&2
  exit 1
fi

cd "$rules_dir"
"${runner[@]}" --version
"${runner[@]}" check rules prometheus-recording-rules.yaml prometheus-alert-rules.yaml
"${runner[@]}" test rules prometheus-recording-rules-tests.yaml
