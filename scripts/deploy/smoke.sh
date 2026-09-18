#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
if [[ -z "${KC_DEPLOY_HOME:-}" ]]; then
  if [[ "${KC_DEPLOY_PROFILE:-local}" == "dev" ]]; then
    KC_DEPLOY_HOME="${HOME}/.kc/deploy-dev"
  else
    KC_DEPLOY_HOME="/tmp/kc-deploy-local"
  fi
fi
ENV_FILE="${KC_DEPLOY_HOME}/compose.env"
if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

server="http://127.0.0.1:${KC_DEPLOY_SERVER_PORT:-7380}"
lakefs="http://127.0.0.1:${KC_DEPLOY_LAKEFS_PORT:-18000}"
opensearch="http://127.0.0.1:${KC_DEPLOY_OPENSEARCH_PORT:-19200}"
opensearch_dashboards="http://127.0.0.1:${KC_DEPLOY_OPENSEARCH_DASHBOARDS_PORT:-15601}/api/status"
grafana="http://127.0.0.1:${KC_DEPLOY_GRAFANA_PORT:-7300}"
prometheus="http://127.0.0.1:${KC_DEPLOY_PROMETHEUS_PORT:-19090}"
minio_console="http://127.0.0.1:${KC_DEPLOY_MINIO_CONSOLE_PORT:-19011}/"
ttyd="http://127.0.0.1:${KC_DEPLOY_TTYD_PORT:-7682}/"

wait_for() {
  local description="$1"
  shift
  for _ in {1..90}; do
    if "$@" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "timed out waiting for ${description}" >&2
  return 1
}

prometheus_has() {
  local query="$1"
  curl -fsSG --data-urlencode "query=${query}" "${prometheus}/api/v1/query" |
    python3 -c 'import json,sys; d=json.load(sys.stdin); raise SystemExit(0 if d.get("status")=="success" and d.get("data",{}).get("result") else 1)'
}

grafana_has_dashboard() {
  local uid="$1"
  curl -fsS "${grafana}/api/dashboards/uid/${uid}" |
    python3 -c 'import json,sys; d=json.load(sys.stdin); raise SystemExit(0 if d.get("dashboard",{}).get("uid")==sys.argv[1] else 1)' "$uid"
}

wait_for "kc-server /readyz" curl -fsS "${server}/readyz"
wait_for "kc console" curl -fsS "${server}/console"
wait_for "lakeFS health" curl -fsS "${lakefs}/_health"
wait_for "OpenSearch" curl -fsS "${opensearch}/_cluster/health"
wait_for "OpenSearch Dashboards" curl -fsS "$opensearch_dashboards"
wait_for "Grafana" curl -fsS "${grafana}/api/health"
wait_for "MinIO console" curl -fsS "$minio_console"
if [[ -n "${KC_TTYD_CREDENTIAL:-}" ]]; then
  wait_for "ttyd" curl -fsS -u "$KC_TTYD_CREDENTIAL" "$ttyd"
else
  wait_for "ttyd" curl -fsS "$ttyd"
fi
wait_for "Prometheus scrape kc-server" prometheus_has 'up{job="knowledge-catalog"} == 1'
wait_for "Grafana kc-overview" grafana_has_dashboard kc-overview
wait_for "Grafana kc-logs" grafana_has_dashboard kc-logs

echo "deploy smoke ok (${KC_DEPLOY_PROFILE:-local})"
echo "ttyd: $ttyd"
