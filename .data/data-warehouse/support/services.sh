#!/usr/bin/env bash

# Run-scoped infrastructure shared by the deterministic and Agent acceptance
# entry points. These helpers only accelerate/enable real providers; every KC
# operation still goes through the public CLI and the real Dolt/OpenSearch
# implementations.

declare -a KC_DW_SERVICE_CONTAINERS=()
declare -a KC_DW_SERVICE_TEMP_DIRS=()

cleanup_acceptance_services() {
  local container temp_dir
  for container in "${KC_DW_SERVICE_CONTAINERS[@]:-}"; do
    [[ -z "$container" ]] || docker rm -f "$container" >/dev/null 2>&1 || true
  done
  for temp_dir in "${KC_DW_SERVICE_TEMP_DIRS[@]:-}"; do
    [[ -z "$temp_dir" ]] || rm -rf "$temp_dir"
  done
}

start_acceptance_dolt() {
  local repo_root="$1"
  local run_root="$2"
  if [[ -n "${KC_DOLT_BIN:-}" ]] || { command -v dolt >/dev/null 2>&1 && [[ "${KC_DOLT_FORCE_DOCKER:-}" != "1" ]]; }; then
    echo "[preflight] Dolt: ${KC_DOLT_BIN:-$(command -v dolt)}"
    return
  fi
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1 || {
    echo "Dolt or a working Docker daemon is required" >&2
    return 1
  }

  local container="kc-dw-dolt-$$"
  local image="${KC_DOLT_DOCKER_IMAGE:-dolthub/dolt:2.3.1}"
  local wrapper_dir
  wrapper_dir="$(mktemp -d)"
  KC_DW_SERVICE_TEMP_DIRS+=("$wrapper_dir")
  local temp_root="${TMPDIR:-/tmp}"
  temp_root="${temp_root%/}"
  local -a mounts=(-v "$repo_root:$repo_root" -v "$temp_root:$temp_root")
  case "$run_root/" in
    "$repo_root"/*) ;;
    *) mounts+=(-v "$run_root:$run_root") ;;
  esac
  echo "[preflight] Dolt: starting reusable $image container"
  docker run --rm -d \
    --name "$container" \
    --entrypoint /bin/sh \
    "${mounts[@]}" \
    "$image" -c 'while :; do sleep 3600; done' >/dev/null
  KC_DW_SERVICE_CONTAINERS+=("$container")
  printf '%s\n' \
    '#!/bin/sh' \
    "exec docker exec -i -w \"\$PWD\" $container /usr/local/bin/dolt \"\$@\"" \
    >"$wrapper_dir/dolt"
  chmod 755 "$wrapper_dir/dolt"
  export KC_DOLT_BIN="$wrapper_dir/dolt"
}

start_acceptance_opensearch() {
  if [[ -n "${KC_TEST_OPENSEARCH_URL:-}" ]]; then
    echo "[preflight] OpenSearch: $KC_TEST_OPENSEARCH_URL"
    return
  fi
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1 || {
    echo "a working Docker daemon is required for Agent SEARCH acceptance" >&2
    return 1
  }

  local container="kc-dw-opensearch-$$"
  local image="${KC_OPENSEARCH_IMAGE:-opensearchproject/opensearch:2.19.3}"
  echo "[preflight] OpenSearch: starting $image"
  docker run --rm -d \
    --name "$container" \
    -p 127.0.0.1::9200 \
    -e discovery.type=single-node \
    -e DISABLE_INSTALL_DEMO_CONFIG=true \
    -e DISABLE_SECURITY_PLUGIN=true \
    -e 'OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m' \
    "$image" >/dev/null
  KC_DW_SERVICE_CONTAINERS+=("$container")
  local mapped host_port
  mapped="$(docker port "$container" 9200/tcp)"
  host_port="${mapped##*:}"
  export KC_TEST_OPENSEARCH_URL="http://127.0.0.1:$host_port"
  local attempt
  for attempt in {1..60}; do
    if curl -fsS "$KC_TEST_OPENSEARCH_URL/_cluster/health" >/dev/null 2>&1; then
      echo "[preflight] OpenSearch: ready at $KC_TEST_OPENSEARCH_URL"
      return
    fi
    sleep 2
  done
  docker logs --tail 200 "$container" >&2
  echo "OpenSearch did not become ready" >&2
  return 1
}

configure_acceptance_opensearch() {
  local kc_bin="${1:?kc binary is required}"
  local kc_home="${2:?KC home is required}"
  [[ -n "${KC_TEST_OPENSEARCH_URL:-}" ]] || {
    echo "KC_TEST_OPENSEARCH_URL is required before configuring SEARCH" >&2
    return 1
  }
  echo "[preflight] KC SEARCH: configuring run-scoped home"
  "$kc_bin" local store set --home "$kc_home" --index opensearch >/dev/null
  "$kc_bin" local store set --home "$kc_home" \
    --driver opensearch --url "$KC_TEST_OPENSEARCH_URL" >/dev/null
}
