#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
runtime_container="kc-state-runtime-e2e-$$"
opensearch_container="kc-state-opensearch-e2e-$$"
image="${KC_STATE_RUNTIME_DOCKER_IMAGE:-python:3.13-alpine}"
opensearch_image="${KC_OPENSEARCH_IMAGE:-opensearchproject/opensearch:2.19.3}"

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "FAIL: Docker daemon is unavailable" >&2
  exit 1
fi

cleanup() {
  docker rm -f "$runtime_container" "$opensearch_container" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run --rm -d \
  --name "$runtime_container" \
  -p 127.0.0.1::8090 \
  -v "$repo_root/scripts/fixtures/state-runtime.py:/app/state-runtime.py:ro" \
  "$image" python /app/state-runtime.py >/dev/null

docker run --rm -d \
  --name "$opensearch_container" \
  -p 127.0.0.1::9200 \
  -e discovery.type=single-node \
  -e DISABLE_INSTALL_DEMO_CONFIG=true \
  -e DISABLE_SECURITY_PLUGIN=true \
  -e 'OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m' \
  "$opensearch_image" >/dev/null

mapped="$(docker port "$runtime_container" 8090/tcp)"
host_port="${mapped##*:}"
endpoint="http://127.0.0.1:$host_port"
opensearch_mapped="$(docker port "$opensearch_container" 9200/tcp)"
opensearch_port="${opensearch_mapped##*:}"
opensearch_endpoint="http://127.0.0.1:$opensearch_port"

for _ in {1..45}; do
  if curl -fsS "$endpoint/health" >/dev/null 2>&1 && curl -fsS "$opensearch_endpoint/_cluster/health" >/dev/null 2>&1; then
    KC_TEST_STATE_RUNTIME_URL="$endpoint" KC_TEST_OPENSEARCH_URL="$opensearch_endpoint" KC_REQUIRE_LIVE_ADAPTERS=1 \
      "${GO:-go}" test -count=1 -run '^TestLiveHTTP(StateRuntimeContainer|RuntimeBuildsOpenSearchStateProjection|DynamicStateSearchJourney)$' ./cli
    exit 0
  fi
  sleep 2
done

docker logs --tail 100 "$runtime_container" >&2
docker logs --tail 200 "$opensearch_container" >&2
exit 1
