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
    '  lakefs      default: product + index scenes using lakeFS HTTP fixtures and real OpenSearch' \
    '  local       alias for lakefs' \
    '  contracts   explicit Snapshot/application component + boundary + application contracts' \
    '  component   component contracts against an ephemeral OpenSearch' \
    '  boundary    architecture, layering, terminology, and surface guards' \
    '  e2e         CLI/HTTP/Catalog journeys on ephemeral OpenSearch; every public kc verb required' \
    '  race        concurrency-sensitive packages and listed cli tests under the race detector' \
    '  coverage    short suite with an explicit statement-coverage floor' \
    '  taihu-live  real Taihu introspection (KC_LIVE_TAIHU=1 + secrets)' \
    '  opensearch  live OpenSearch projection/search contract' \
    '  state-runtime live resource-access/v1 State runtime contract in Docker' \
    '  kcfs        Docker Linux/FUSE host projection acceptance' \
    '  adapters    opensearch' \
    '  docker      adapters + State runtime + kcfs' \
    '  all         lakefs + live deployment scenes + contracts + docker' \
    '' \
    'lakefs reuses KC_TEST_OPENSEARCH_URL or starts ephemeral OpenSearch; no deployment stack required.' \
    'Live deployment scenes: make deploy-local-scenes (prepare with make deploy-local-up); also included in all.'
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

run_named_go_test() {
  local package="$1" test_name="$2"
  shift 2
  local declarations candidate found=0
  if declarations="$("$go_bin" test -list "^${test_name}$" "$package")"; then
    while IFS= read -r candidate; do
      if [[ "$candidate" == "$test_name" ]]; then
        found=1
        break
      fi
    done <<< "$declarations"
  else
    local list_status=$?
    printf '%s\n' "$declarations" >&2
    return "$list_status"
  fi
  if [[ "$found" != 1 ]]; then
    printf 'FAIL: test %s is not declared in %s\n' "$test_name" "$package" >&2
    return 1
  fi
  run_go_test "$@" -run "^${test_name}$" "$package"
}

run_component() {
  local component_packages=()
  local package
  while IFS= read -r package; do
    case "$package" in
      kc/snapshot/dataset|kc/cli|kc/cmd/*|kc/internal/arch) ;;
      *) component_packages+=("$package") ;;
    esac
  done < <("$go_bin" list ./...)
  run_go_test -short -count=1 "${component_packages[@]}"
}

run_boundary() {
  run_go_test -short -count=1 ./internal/arch
}

run_e2e() {
  if [[ -n "${KC_E2E_RUN:-}" ]]; then
    run_go_test -short -count=1 -timeout=60m -run "$KC_E2E_RUN" ./cli
    return
  fi
  KC_ASSERT_E2E_COVERAGE=1 run_go_test -short -count=1 -timeout=60m ./cli ./snapshot/dataset
}

race_cli_tests() {
  local name names=()
  while IFS= read -r name; do
    name="${name%%#*}"
    name="${name//[[:space:]]/}"
    [[ -n "$name" ]] && names+=("$name")
  done < ./scripts/race-cli-tests.txt
  if [[ ${#names[@]} -eq 0 ]]; then
    printf 'FAIL: scripts/race-cli-tests.txt lists no cli tests\n' >&2
    return 1
  fi
  local IFS='|'
  printf '^(%s)$' "${names[*]}"
}

run_race() {
  run_go_test -short -race -count=1 -timeout=30m ./snapshot ./hook ./object/reader ./object/cache ./object/maintenance ./discovery/projection ./declarativeindex/projection ./declarativeindex/control ./declarativeindex/plugin ./home ./server ./invocation
  # The full cli package exceeds any race budget; its listed concurrent tests run here and
  # TestRaceScopeCoversCLIConcurrencyTests keeps the list complete.
  local pattern
  pattern="$(race_cli_tests)"
  run_go_test -short -race -count=1 -timeout=15m -run "$pattern" ./cli
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

run_taihu_live() {
  KC_LIVE_TAIHU=1 run_named_go_test ./server TestLiveTaihuAuthentication -count=1 -timeout=2m
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
  if [[ -z "${KC_TEST_OPENSEARCH_RUN_PREFIX:-}" ]]; then
    python3 "$repo_root/scripts/opensearch_fixture.py" run --endpoint "$KC_TEST_OPENSEARCH_URL" -- "$0" lakefs
    return
  fi
  # The existing scene tree owns its lakeFS HTTP fixtures. Index scenes use
  # real OpenSearch; neither suite needs a full product deployment.
  run_go_test -count=1 -timeout=15m -run '^(TestProductScenes|TestMetricPermissionScenes)$' ./cli
}

run_contracts() {
  run_component
  run_boundary
  run_e2e
}

run_adapters() {
  run_opensearch
}

run_docker() {
  run_adapters
  run_state_runtime
  run_kcfs
}

start_local_opensearch() {
  if [[ -n "${KC_TEST_OPENSEARCH_URL:-}" ]]; then
    return
  fi
  exec python3 "$repo_root/scripts/opensearch_auto.py" \
    --image "${KC_OPENSEARCH_IMAGE:-opensearchproject/opensearch:2.19.3}" -- "$0" "$group"
}

case "$group" in
  lakefs|component|e2e|contracts|race|coverage|all|adapters|docker) start_local_opensearch ;;
esac

case "$group" in
  lakefs) run_lakefs ;;
  contracts) run_contracts ;;
  component) run_component ;;
  boundary) run_boundary ;;
  e2e) run_e2e ;;
  race) run_race ;;
  coverage) run_coverage ;;
  taihu-live) run_taihu_live ;;
  opensearch) run_opensearch ;;
  state-runtime) run_state_runtime ;;
  kcfs) run_kcfs ;;
  adapters) run_adapters ;;
  docker) run_docker ;;
  all)
    run_lakefs
    ./scripts/deploy/deploy.sh local scenes
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
