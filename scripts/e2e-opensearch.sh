#!/usr/bin/env bash
set -euo pipefail

container_name="kc-opensearch-e2e"
image="${KC_OPENSEARCH_IMAGE:-opensearchproject/opensearch:2.19.3}"

if docker container inspect "$container_name" >/dev/null 2>&1; then
  echo "container $container_name already exists; refusing to replace it" >&2
  exit 1
fi

cleanup() {
  docker stop "$container_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run --rm -d \
  --name "$container_name" \
  -p 127.0.0.1:9200:9200 \
  -e discovery.type=single-node \
  -e DISABLE_INSTALL_DEMO_CONFIG=true \
  -e DISABLE_SECURITY_PLUGIN=true \
  -e 'OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m' \
  "$image" >/dev/null

for _ in {1..45}; do
  if curl -fsS http://127.0.0.1:9200/_cluster/health >/dev/null; then
    go test -count=1 -v ./retrieval/elasticsearch
    curl -fsS 'http://127.0.0.1:9200/kc-projection-control-v1/_count' \
      -H 'Content-Type: application/json' \
      -d '{"query":{"term":{"state":"READY"}}}'
    exit 0
  fi
  sleep 2
done

docker logs --tail 200 "$container_name" >&2
exit 1
