#!/usr/bin/env bash
set -euo pipefail

container_name="kc-opensearch-e2e-$$"
image="${KC_OPENSEARCH_IMAGE:-opensearchproject/opensearch:2.19.3}"

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "FAIL: Docker daemon is unavailable" >&2
  exit 1
fi

cleanup() {
  docker rm -f "$container_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run --rm -d \
  --name "$container_name" \
  -p 127.0.0.1::9200 \
  -e discovery.type=single-node \
  -e DISABLE_INSTALL_DEMO_CONFIG=true \
  -e DISABLE_SECURITY_PLUGIN=true \
  -e 'OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m' \
  "$image" >/dev/null

mapped="$(docker port "$container_name" 9200/tcp)"
host_port="${mapped##*:}"
endpoint="http://127.0.0.1:$host_port"

for _ in {1..45}; do
  if curl -fsS "$endpoint/_cluster/health" >/dev/null 2>&1; then
    KC_TEST_OPENSEARCH_URL="$endpoint" go test -count=1 -v ./retrieval/opensearch
    curl -fsS "$endpoint/kc-projection-control-v1/_count" \
      -H 'Content-Type: application/json' \
      -d '{"query":{"term":{"state":"READY"}}}'
    exit 0
  fi
  sleep 2
done

docker logs --tail 200 "$container_name" >&2
exit 1
