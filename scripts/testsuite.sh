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
    '  component   component unit and local contract tests (no live adapters)' \
    '  boundary    architecture, layering, terminology, and surface guards' \
    '  e2e         local CLI/HTTP/Catalog journeys; every public kc verb required' \
    '  local       component + boundary + e2e' \
    '  gitea       live Gitea Snapshot + Knowledge contract' \
    '  dolt        live Dolt Snapshot + Knowledge contract' \
    '  opensearch  live OpenSearch projection/search contract' \
    '  kcfs        Docker Linux/FUSE host projection acceptance' \
    '  adapters    gitea + dolt + opensearch' \
    '  docker      adapters + kcfs' \
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

run_gitea() {
  KC_REQUIRE_LIVE_ADAPTERS=1 "$go_bin" test -count=1 ./snapshot/gitea
}

run_dolt() {
  KC_REQUIRE_LIVE_ADAPTERS=1 "$go_bin" test -count=1 ./snapshot/dolt
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
  run_kcfs
}

group="${1:-local}"
case "$group" in
  component) run_component ;;
  boundary) run_boundary ;;
  e2e) run_e2e ;;
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
