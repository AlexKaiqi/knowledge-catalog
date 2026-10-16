#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -z "${KC_TEST_OPENSEARCH_URL:-}" ]]; then
  exec python3 "$repo_root/scripts/opensearch_auto.py" \
    --image "${KC_OPENSEARCH_IMAGE:-opensearchproject/opensearch:2.19.3}" -- "$0"
fi
accessor="$repo_root/.data/scenes/service-initialized/source-repositories-configured/repository-attached/_materials/accessor"
runtime_container="kc-state-runtime-e2e-$$"
opensearch_endpoint="${KC_TEST_OPENSEARCH_URL:-}"
image="${KC_STATE_RUNTIME_DOCKER_IMAGE:-python:3.13-alpine}"

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "FAIL: Docker daemon is unavailable" >&2
  exit 1
fi

cleanup() {
  docker rm -f "$runtime_container" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run --rm -d \
  --name "$runtime_container" \
  -p 127.0.0.1::7390 \
  -v "$accessor:/app:ro" \
  -e KC_SOURCE_PATH=/app/source.json \
  "$image" python /app/access.py --listen 0.0.0.0:7390 >/dev/null


mapped="$(docker port "$runtime_container" 7390/tcp)"
host_port="${mapped##*:}"
endpoint="http://127.0.0.1:$host_port"

for _ in {1..45}; do
  if curl --connect-timeout 2 --max-time 3 -fsS "$endpoint/health" >/dev/null 2>&1 && curl --connect-timeout 2 --max-time 3 -fsS "$opensearch_endpoint/_cluster/health" >/dev/null 2>&1; then
    KC_TEST_STATE_RUNTIME_URL="$endpoint" KC_TEST_OPENSEARCH_URL="$opensearch_endpoint" KC_REQUIRE_LIVE_ADAPTERS=1 \
      "${GO:-go}" test -json -count=1 -run '^TestLiveHTTP(StateRuntimeContainer|RuntimeBuildsOpenSearchStateProjection|DynamicStateSearchJourney)$' ./cli ./liveaccess
    exit 0
  fi
  sleep 2
done

docker logs --tail 100 "$runtime_container" >&2

exit 1
