#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
go_bin="${GO:-go}"
cd "$repo_root"

group="${1:-lakefs}"
# Keep the former local spelling on the product path, not the old mixed suite.
if [[ "$group" == "local" ]]; then
  group="lakefs"
fi

# One command, one source-bound evidence directory. Help remains read-only.
if [[ -z "${KC_VALIDATION_RUN_ID:-}" && "$group" != "help" && "$group" != "--help" && "$group" != "-h" ]]; then
  exec python3 ./scripts/validation.py run --scope "testsuite:$group" -- "$0" "$group"
fi

usage() {
  printf '%s\n' \
    'usage: ./scripts/testsuite.sh <group>' \
    '' \
    'groups:' \
    '  lakefs      default: live scenes on the existing deploy-local lakeFS stack' \
    '  local       alias for lakefs; does not start Dolt' \
    '  contracts   explicit mixed-provider component + boundary + application contracts (uses Dolt)' \
    '  component   component contracts against an ephemeral OpenSearch' \
    '  boundary    architecture, layering, terminology, and surface guards' \
    '  e2e         CLI/HTTP/Catalog journeys on ephemeral OpenSearch; every public kc verb required' \
    '  race        concurrency-sensitive local packages under the race detector' \
    '  coverage    short suite with an explicit statement-coverage floor' \
    '  service-e2e authenticated provider/consumer journey on Gitea + OpenSearch' \
    '  taihu-live  real Taihu introspection (KC_LIVE_TAIHU=1 + secrets)' \
    '  gitea       live Gitea Snapshot + Knowledge contract' \
    '  dolt        live Dolt Snapshot + Knowledge contract' \
    '  opensearch  live OpenSearch projection/search contract' \
    '  state-runtime live resource-access/v1 State runtime contract in Docker' \
    '  kcfs        Docker Linux/FUSE host projection acceptance' \
    '  adapters    gitea + dolt + opensearch' \
    '  docker      adapters + State runtime + authenticated service roles + kcfs' \
    '  all         lakefs + contracts + docker (explicit multi-provider acceptance)' \
    '' \
    'Prepare the product stack explicitly with make deploy-local-up before lakefs/all.'
}

go_test_sequence=0
run_go_test() {
  go_test_sequence=$((go_test_sequence + 1))
  local evidence_args=()
  if [[ -n "${KC_VALIDATION_RUN_DIR:-}" ]]; then
    evidence_args=(-json)
    KC_COMMAND_COVERAGE_REPORT="$KC_VALIDATION_RUN_DIR/command-coverage-$go_test_sequence.json" \
      "$go_bin" test "${evidence_args[@]}" "$@"
  else
    "$go_bin" test "$@"
  fi
}

run_component() {
  local component_packages=()
  local package
  while IFS= read -r package; do
    case "$package" in
      kc/catalog|kc/cli|kc/cmd/*|kc/internal/arch) ;;
      *) component_packages+=("$package") ;;
    esac
  done < <("$go_bin" list ./...)
  run_go_test -short -count=1 "${component_packages[@]}"
}

run_boundary() {
  run_go_test -short -count=1 ./internal/arch
}

run_e2e() {
  # These older application fixtures still include Dolt. They are an explicit
  # contract group, not the default lakeFS deployment acceptance path.
  if [[ -n "${KC_E2E_RUN:-}" ]]; then
    run_go_test -short -count=1 -timeout=60m -run "$KC_E2E_RUN" ./cli
    return
  fi
  KC_ASSERT_E2E_COVERAGE=1 run_go_test -short -count=1 -timeout=60m ./cli ./catalog ./cmd/kc-integration
}

run_race() {
  run_go_test -short -race -count=1 -timeout=30m ./snapshot/commandlog ./hook ./knowledge/reader ./retrieval/cache ./index ./home ./cli
}

run_coverage() {
  local profile="${KC_COVERPROFILE:-${KC_VALIDATION_RUN_DIR:-/tmp}/kc-coverage.out}"
  local minimum="${KC_COVERAGE_MIN:-55.0}"
  KC_ASSERT_E2E_COVERAGE=1 run_go_test -short -count=1 -timeout=30m -coverprofile="$profile" ./...
  local total
  total="$("$go_bin" tool cover -func="$profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
  awk -v got="$total" -v want="$minimum" 'BEGIN { if ((got + 0) < (want + 0)) { printf "statement coverage %s%% is below %s%%\n", got, want > "/dev/stderr"; exit 1 } }'
  printf 'statement coverage %s%% (minimum %s%%)\n' "$total" "$minimum"
}

run_service_e2e() {
  ./scripts/e2e-service-roles.sh
}

run_taihu_live() {
  KC_LIVE_TAIHU=1 run_go_test -count=1 -timeout=2m -run TestLiveTaihuAuthentication ./cli
}

run_gitea() {
  local contract_node
  contract_node="$(command -v "${KC_NODE_BIN:-node}" || true)"
  if [[ -z "$contract_node" ]] || [[ "$("$contract_node" -p 'process.versions.node.split(".")[0]')" != "24" ]]; then
    printf '%s\n' 'FAIL: gitea consumer contracts require an installed Node 24; set KC_NODE_BIN to its executable' >&2
    return 1
  fi
  if [[ ! -d dsh-plugin/node_modules ]]; then
    printf '%s\n' 'FAIL: prepare the locked dsh-plugin dependencies before running the gitea consumer contracts' >&2
    return 1
  fi
  PATH="$(dirname "$contract_node"):$PATH" npm --prefix dsh-plugin run build
  KC_REQUIRE_LIVE_ADAPTERS=1 run_go_test -count=1 ./snapshot/gitea
  KC_REQUIRE_LIVE_ADAPTERS=1 run_go_test -count=1 -run '^(TestLocalSystemPublishImportsBuiltinSchemasIntoLiveGitea|TestManagedRepositoryProviderOnLiveGitea|TestManagedProductHumanSelfServiceOnLiveGitea|TestRepositoryConnectionOnLiveGitea)$' ./cli
  KC_NODE_BIN="$contract_node" KC_REQUIRE_LIVE_ADAPTERS=1 run_go_test -count=1 -tags=dsh_contract,catalog_discovery_contract -run '^(TestDSHConsumerUsesActualServerContract|TestCatalogDiscoveryActualServerPinsSelectedSourcesAndMasksBodies)$' ./cli
}

run_dolt() {
  KC_REQUIRE_LIVE_ADAPTERS=1 run_go_test -count=1 ./snapshot/dolt ./knowledge/dolt
  KC_REQUIRE_LIVE_ADAPTERS=1 run_go_test -count=1 -run '^TestScaleProfileRepoAddDolt$' ./cli
}

run_opensearch() {
  ./scripts/e2e-opensearch.sh
}

run_state_runtime() {
  ./scripts/e2e-state-runtime-docker.sh
}

run_kcfs() {
  ./scripts/e2e-kcfs-docker.sh
}

run_lakefs() {
  # Reuse the chosen product topology. No replacement authority, implicit up,
  # rebuild or cleanup of an operator's existing deployment belongs here.
  ./scripts/deploy/deploy.sh local scenes
}

run_contracts() {
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
  run_state_runtime
  run_service_e2e
  run_kcfs
}

opensearch_container=""
dolt_container=""
dolt_wrapper_dir=""
cleanup_local_services() {
  if [[ -n "$opensearch_container" ]]; then
    docker rm -f "$opensearch_container" >/dev/null 2>&1 || true
  fi
  if [[ -n "$dolt_container" ]]; then
    docker rm -f "$dolt_container" >/dev/null 2>&1 || true
  fi
  if [[ -n "$dolt_wrapper_dir" ]]; then
    rm -f "$dolt_wrapper_dir/dolt"
    rmdir "$dolt_wrapper_dir" 2>/dev/null || true
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
  trap cleanup_local_services EXIT
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

# Only explicit legacy contract / adapter groups need this Dolt runtime.
# Reuse one container rather than starting an engine container for every call.
start_local_dolt() {
  if [[ -n "${KC_DOLT_BIN:-}" ]] || { command -v dolt >/dev/null 2>&1 && [[ "${KC_DOLT_FORCE_DOCKER:-}" != "1" ]]; }; then
    return
  fi
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    printf 'FAIL: explicitly selected %s contracts require Dolt or Docker\n' "$group" >&2
    exit 1
  fi
  local probe_dir temp_root image
  probe_dir="$(mktemp -d)"
  temp_root="$(dirname "$probe_dir")"
  rmdir "$probe_dir"
  image="${KC_DOLT_DOCKER_IMAGE:-dolthub/dolt:latest}"
  dolt_container="kc-local-dolt-$$"
  docker run --rm -d \
    --name "$dolt_container" \
    --entrypoint /bin/sh \
    -v "$temp_root:$temp_root" \
    -v "$repo_root:$repo_root" \
    "$image" -c 'while :; do sleep 3600; done' >/dev/null
  dolt_wrapper_dir="$(mktemp -d)"
  uid="$(id -u)"
  gid="$(id -g)"
  printf '%s\n' \
    '#!/bin/sh' \
    "exec docker exec -i -u ${uid}:${gid} -e HOME=/tmp -w \"\$PWD\" $dolt_container /bin/sh -c 'exec /usr/local/bin/dolt \"\$@\" 2>&1' dolt \"\$@\"" \
    >"$dolt_wrapper_dir/dolt"
  chmod 0o755 "$dolt_wrapper_dir/dolt" 2>/dev/null || chmod 755 "$dolt_wrapper_dir/dolt"
  docker exec -i -u "${uid}:${gid}" -e HOME=/tmp "$dolt_container" /usr/local/bin/dolt config --global --add user.email kc@localhost >/dev/null
  docker exec -i -u "${uid}:${gid}" -e HOME=/tmp "$dolt_container" /usr/local/bin/dolt config --global --add user.name kc >/dev/null
  export KC_DOLT_BIN="$dolt_wrapper_dir/dolt"
  trap cleanup_local_services EXIT
}

case "$group" in
  component|e2e|contracts|race|coverage|all|gitea|adapters|docker) start_local_opensearch ;;
esac

case "$group" in
  e2e|contracts|coverage|all|dolt|adapters|docker) start_local_dolt ;;
esac

case "$group" in
  lakefs) run_lakefs ;;
  contracts) run_contracts ;;
  component) run_component ;;
  boundary) run_boundary ;;
  e2e) run_e2e ;;
  race) run_race ;;
  coverage) run_coverage ;;
  service-e2e) run_service_e2e ;;
  taihu-live) run_taihu_live ;;
  gitea) run_gitea ;;
  dolt) run_dolt ;;
  opensearch) run_opensearch ;;
  state-runtime) run_state_runtime ;;
  kcfs) run_kcfs ;;
  adapters) run_adapters ;;
  docker) run_docker ;;
  all)
    run_lakefs
    run_contracts
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
