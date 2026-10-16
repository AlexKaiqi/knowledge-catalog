#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -z "${KC_TEST_OPENSEARCH_URL:-}" ]]; then
  exec python3 "$repo_root/scripts/opensearch_auto.py" \
    --image "${KC_OPENSEARCH_IMAGE:-opensearchproject/opensearch:2.19.3}" -- "$0"
fi
endpoint="$KC_TEST_OPENSEARCH_URL"

for _ in {1..45}; do
  if curl -fsS "$endpoint/_cluster/health" >/dev/null 2>&1; then
    KC_TEST_OPENSEARCH_URL="$endpoint" "${GO:-go}" test -json -count=1 -v ./declarativeindex/opensearch
    KC_TEST_OPENSEARCH_URL="$endpoint" "${GO:-go}" test -json -count=1 -run '^TestLiveOpenSearchStateProjectionRefreshAndSameBasisHydrate$' -v ./declarativeindex/projection
    curl -fsS "$endpoint/kc-projection-control-v1/_count" \
      -H 'Content-Type: application/json' \
      -d '{"query":{"term":{"state":"READY"}}}'
    exit 0
  fi
  sleep 2
done

exit 1
