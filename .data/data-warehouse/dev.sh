#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
compose="$root/.data/data-warehouse/compose.e2e.yaml"
command="${1:-up}"
shift || true

dc() {
  docker compose -f "$compose" "$@"
}

case "$command" in
  up)
    [[ -f "${HOME}/.env" ]] || { echo "missing ${HOME}/.env" >&2; exit 1; }
    mkdir -p \
      "$root/.data/data-warehouse/runs/compose/bootstrap" \
      "$root/.data/data-warehouse/runs/compose/dsh" \
      "$root/.data/data-warehouse/runs/compose/workspace"
    dc build --quiet bootstrap dsh
    dc up --detach --wait --no-build "$@"
    dc exec -T dsh bash /usr/local/bin/kc-compose-smoke
    echo "DSH:      http://127.0.0.1:${KC_DW_DSH_PORT:-7400}"
    echo "KC Server: http://127.0.0.1:${KC_DW_SERVER_PORT:-7380}"
    echo "Gitea:    http://127.0.0.1:${KC_DW_GITEA_PORT:-3000}"
    ;;
  smoke)
    dc exec -T dsh bash /usr/local/bin/kc-compose-smoke
    ;;
  status)
    dc ps
    ;;
  logs)
    if (( $# )); then
      dc logs --follow --tail=100 "$@"
    else
      dc logs --follow --tail=100
    fi
    ;;
  down)
    dc down --remove-orphans "$@"
    ;;
  reset)
    dc down --volumes --remove-orphans
    rm -rf "$root/.data/data-warehouse/runs/compose"
    ;;
  *)
    echo "usage: $0 up|smoke|status|logs [service]|down|reset" >&2
    exit 2
    ;;
esac
