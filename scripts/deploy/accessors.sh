#!/usr/bin/env bash
# Wall-side scene accessors for the local walk stack. Not a first-stage companion.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ACCESSOR="$ROOT/.data/scenes/catalog-initialized/source-repositories-configured/repository-attached/_materials/accessor"
PROJECT="${KC_ACCESSOR_PROJECT:-kc-scene-accessor}"
CMD="${1:-up}"

accessor_compose() {
  docker compose --project-name "$PROJECT" -f "$ACCESSOR/compose.yaml" -f "$ACCESSOR/compose.walk.yaml" "$@"
}

cmd_down() {
  export KC_WALK_KC="${KC_WALK_KC:-/dev/null}"
  docker compose --project-name kc-scene-accessor-index -f "$ACCESSOR/compose.yaml" down --remove-orphans >/dev/null 2>&1 || true
  if docker compose --project-name "$PROJECT" -f "$ACCESSOR/compose.yaml" ps -q >/dev/null 2>&1; then
    accessor_compose down --remove-orphans >/dev/null
  fi
}

cmd_up() {
  if ! docker network inspect "${KC_ACCESSOR_NETWORK:-kc-deploy-local_default}" >/dev/null 2>&1; then
    echo "FAIL: deploy-local network is missing; start the local stack first" >&2
    exit 1
  fi
  if ! docker inspect kc-deploy-local-cli-1 >/dev/null 2>&1; then
    echo "FAIL: kc-deploy-local-cli-1 is missing; start the local stack first" >&2
    exit 1
  fi
  bin_dir="$(mktemp -d)"
  trap 'rm -rf "$bin_dir"' EXIT
  docker cp kc-deploy-local-cli-1:/usr/local/bin/kc "$bin_dir/kc"
  chmod 0755 "$bin_dir/kc"
  export KC_WALK_KC="$bin_dir/kc"
  export KC_REPOSITORY="${KC_REPOSITORY:-table-meta}"
  export KC_NOTICE_REPOSITORY="${KC_NOTICE_REPOSITORY:-$KC_REPOSITORY}"
  export KC_AS="${KC_AS:-admin}"
  export KC_ACCESSOR_NETWORK="${KC_ACCESSOR_NETWORK:-kc-deploy-local_default}"
  accessor_compose up --detach --wait --build
}

case "$CMD" in
  up) cmd_up ;;
  down) cmd_down ;;
  *)
    echo "usage: $0 up|down" >&2
    exit 2
    ;;
esac
