#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -z "${KC_TEST_OPENSEARCH_URL:-}" ]]; then
  exec python3 "$repo_root/scripts/opensearch_auto.py" \
    --image "${KC_OPENSEARCH_IMAGE:-opensearchproject/opensearch:2.19.3}" -- "$0"
fi
endpoint="$KC_TEST_OPENSEARCH_URL"

# Index names owned by these contracts. Each run allocates its own namespace and
# registers cleanup for it, so the only stable post-run fact is that the set came
# back unchanged. Asserting a fixed control index name would instead require a
# leftover from an earlier run: namespaced control indices carry a digest suffix
# and their owners delete them, which is why the previous bare
# "kc-projection-control-v1/_count" probe returned 404 on a clean instance.
owned_indices() {
  # A successful probe with an empty body means "no matching index", not a
  # malformed response; curl -f still fails the step on an HTTP error.
  curl -fsS "$endpoint/_cat/indices/kc-proj*,kc-projection-control-v1*?format=json&h=index&expand_wildcards=all" |
    python3 -c 'import json,sys
raw = sys.stdin.read().strip()
print("\n".join(sorted(i["index"] for i in json.loads(raw)))) if raw else None'
}

report_leaks() {
  local baseline="$1" current="$2"
  local leaked
  leaked="$(comm -13 "$baseline" "$current" || true)"
  if [[ -n "$leaked" ]]; then
    printf 'FAIL: contracts left indices behind:\n%s\n' "$leaked" >&2
    return 1
  fi
}

for _ in {1..45}; do
  if curl -fsS "$endpoint/_cluster/health" >/dev/null 2>&1; then
    baseline="$(mktemp)"
    current="$(mktemp)"
    trap 'rm -f "$baseline" "$current"' EXIT
    owned_indices >"$baseline"
    KC_TEST_OPENSEARCH_URL="$endpoint" "${GO:-go}" test -json -count=1 -v ./declarativeindex/opensearch
    KC_TEST_OPENSEARCH_URL="$endpoint" "${GO:-go}" test -json -count=1 -run '^TestLiveOpenSearchStateProjectionRefreshAndSameBasisHydrate$' -v ./declarativeindex/projection
    owned_indices >"$current"
    report_leaks "$baseline" "$current"
    exit 0
  fi
  sleep 2
done

exit 1
