#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
go_bin="${GO:-go}"
cd "$repo_root"

component_packages=()
while IFS= read -r package; do
  case "$package" in
    kc/catalog|kc/cli|kc/cmd/*|kc/internal/arch) ;;
    *) component_packages+=("$package") ;;
  esac
done < <("$go_bin" list ./...)

usage() {
  printf '%s\n' \
    'usage: ./scripts/testsuite.sh <group>' \
    '' \
    'groups:' \
    '  component   component contracts against an ephemeral OpenSearch' \
    '  boundary    architecture, layering, terminology, and surface guards' \
    '  e2e         CLI/HTTP/Catalog journeys on ephemeral OpenSearch; every public kc verb required' \
    '  race        concurrency-sensitive local packages under the race detector' \
    '  coverage    short suite with a non-regression statement-coverage gate' \
    '  service-e2e authenticated provider/consumer journey on Gitea + OpenSearch' \
    '  local       component + boundary + e2e' \
    '  gitea       live Gitea Snapshot + Knowledge contract' \
    '  dolt        live Dolt Snapshot + Knowledge contract' \
    '  opensearch  live OpenSearch projection/search contract' \
    '  kcfs        Docker Linux/FUSE host projection acceptance' \
    '  adapters    gitea + dolt + opensearch' \
    '  docker      adapters + authenticated service roles + kcfs' \
    '  all         local + docker'
}

run_component() {
  "$go_bin" test -short -count=1 "${component_packages[@]}"
}

run_boundary() {
  "$go_bin" test -short -count=1 ./internal/arch
}

run_e2e() {
  KC_ASSERT_E2E_COVERAGE=1 "$go_bin" test -short -count=1 ./cli ./catalog
}

run_race() {
  "$go_bin" test -short -race -count=1 ./snapshot/commandlog ./hook ./knowledge/reader ./index ./cli
}

run_coverage() {
  local profile="${KC_COVERPROFILE:-/tmp/kc-coverage.out}"
  local minimum="${KC_COVERAGE_MIN:-55.0}"
  KC_ASSERT_E2E_COVERAGE=1 "$go_bin" test -short -count=1 -coverprofile="$profile" ./...
  local total
  total="$("$go_bin" tool cover -func="$profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
  awk -v got="$total" -v want="$minimum" 'BEGIN { if ((got + 0) < (want + 0)) { printf "statement coverage %s%% is below %s%%\n", got, want > "/dev/stderr"; exit 1 } }'
  printf 'statement coverage %s%% (minimum %s%%)\n' "$total" "$minimum"
}

run_service_e2e() {
  ./scripts/e2e-service-roles.sh
}

run_gitea() {
  KC_REQUIRE_LIVE_ADAPTERS=1 "$go_bin" test -count=1 ./snapshot/gitea
}

run_dolt() {
  KC_REQUIRE_LIVE_ADAPTERS=1 "$go_bin" test -count=1 ./snapshot/dolt
  KC_REQUIRE_LIVE_ADAPTERS=1 "$go_bin" test -count=1 -run '^TestScaleProfileRepoAddDolt$' ./cli
}

run_opensearch() {
  ./scripts/e2e-opensearch.sh
}

run_kcfs() {
  ./scripts/e2e-kcfs-docker.sh
}

run_local() {
  run_component
  run_boundary
  run_e2e
}

run_adapters() {
  run_gitea
  run_dolt
  run_opensearch
}

run_docker() {
  run_adapters
  run_service_e2e
  run_kcfs
}

group="${1:-local}"

opensearch_container=""
cleanup_local_opensearch() {
  if [[ -n "$opensearch_container" ]]; then
    docker rm -f "$opensearch_container" >/dev/null 2>&1 || true
  fi
}

start_local_opensearch() {
  if [[ -n "${KC_TEST_OPENSEARCH_URL:-}" ]]; then
    return
  fi
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    printf 'FAIL: %s requires Docker because OpenSearch is the only retrieval implementation\n' "$group" >&2
    exit 1
  fi
  opensearch_container="kc-local-opensearch-$$"
  trap cleanup_local_opensearch EXIT
  docker run --rm -d \
    --name "$opensearch_container" \
    -p 127.0.0.1::9200 \
    -e discovery.type=single-node \
    -e DISABLE_INSTALL_DEMO_CONFIG=true \
    -e DISABLE_SECURITY_PLUGIN=true \
    -e 'OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m' \
    "${KC_OPENSEARCH_IMAGE:-opensearchproject/opensearch:2.19.3}" >/dev/null
  local mapped host_port
  mapped="$(docker port "$opensearch_container" 9200/tcp)"
  host_port="${mapped##*:}"
  export KC_TEST_OPENSEARCH_URL="http://127.0.0.1:$host_port"
  for _ in {1..60}; do
    if curl -fsS "$KC_TEST_OPENSEARCH_URL/_cluster/health" >/dev/null 2>&1; then
      return
    fi
    sleep 2
  done
  docker logs --tail 200 "$opensearch_container" >&2
  exit 1
}

case "$group" in
  component|e2e|local|race|coverage|all) start_local_opensearch ;;
esac

case "$group" in
  component) run_component ;;
  boundary) run_boundary ;;
  e2e) run_e2e ;;
  race) run_race ;;
  coverage) run_coverage ;;
  service-e2e) run_service_e2e ;;
  local) run_local ;;
  gitea) run_gitea ;;
  dolt) run_dolt ;;
  opensearch) run_opensearch ;;
  kcfs) run_kcfs ;;
  adapters) run_adapters ;;
  docker) run_docker ;;
  all)
    run_local
    run_docker
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
