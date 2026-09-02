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
    mkdir -p \
      "$root/.data/data-warehouse/runs/compose/bootstrap" \
      "$root/.data/data-warehouse/runs/compose/workspace"
    dc build --quiet bootstrap cli
    dc up --detach --wait --no-build "$@"
    dc exec -T cli bash /usr/local/bin/kc-compose-cli-smoke
    echo "CLI:       http://127.0.0.1:${KC_DW_CLI_PORT:-7681}"
    echo "KC Server: http://127.0.0.1:${KC_DW_SERVER_PORT:-7380}"
    echo "Gitea:     http://127.0.0.1:${KC_DW_GITEA_PORT:-3000}"
    ;;
  dsh-up)
    [[ -f "${HOME}/.env" ]] || { echo "missing ${HOME}/.env" >&2; exit 1; }
    mkdir -p \
      "$root/.data/data-warehouse/runs/compose/dsh" \
      "$root/.data/data-warehouse/runs/compose/workspace"
    dc build --quiet dsh
    dc --profile dsh up --detach --wait --no-build dsh
    dc exec -T dsh bash /usr/local/bin/kc-compose-smoke
    echo "DSH:       http://127.0.0.1:${KC_DW_DSH_PORT:-7400}"
    ;;
  obs-up)
    export KC_DW_OTLP_TRACES_ENDPOINT="${KC_DW_OTLP_TRACES_ENDPOINT:-http://otel-collector:4318/v1/traces}"
    export KC_DW_OTLP_LOGS_ENDPOINT="${KC_DW_OTLP_LOGS_ENDPOINT:-http://otel-collector:4318/v1/logs}"
    mkdir -p "$root/.data/data-warehouse/runs/compose/bootstrap"
    dc build --quiet bootstrap
    # Provisioning files are bind-mounted definitions. Recreate these services
    # on every run so edited Collector pipelines/datasources/dashboards cannot
    # be masked by an already-running container.
    dc --profile observability up --detach --wait --no-build --force-recreate prometheus otel-collector jaeger loki grafana
    "$root/.data/data-warehouse/observability/smoke.sh"
    ;;
  smoke)
    dc exec -T cli bash /usr/local/bin/kc-compose-cli-smoke
    ;;
  dsh-smoke)
    dc exec -T dsh bash /usr/local/bin/kc-compose-smoke
    ;;
  obs-smoke)
    "$root/.data/data-warehouse/observability/smoke.sh"
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
    dc --profile dsh --profile observability down --remove-orphans "$@"
    ;;
  obs-down)
    dc --profile observability down --remove-orphans "$@"
    ;;
  reset)
    dc --profile dsh --profile observability down --volumes --remove-orphans
    rm -rf "$root/.data/data-warehouse/runs/compose"
    ;;
  *)
    echo "usage: $0 up|dsh-up|obs-up|smoke|dsh-smoke|obs-smoke|status|logs [service]|down|obs-down|reset" >&2
    exit 2
    ;;
esac
