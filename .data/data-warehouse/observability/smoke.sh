#!/usr/bin/env bash
set -euo pipefail

server="http://127.0.0.1:${KC_DW_SERVER_PORT:-7380}"
prometheus="http://127.0.0.1:${KC_DW_PROMETHEUS_PORT:-9090}"
jaeger="http://127.0.0.1:${KC_DW_JAEGER_PORT:-16686}"
loki="http://127.0.0.1:${KC_DW_LOKI_PORT:-3100}"
grafana="http://127.0.0.1:${KC_DW_GRAFANA_PORT:-7300}"
observability_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

wait_for() {
  local description="$1"
  shift
  for _ in {1..90}; do
    if "$@" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for ${description}" >&2
  return 1
}

prometheus_has() {
  local query="$1"
  curl -fsSG --data-urlencode "query=${query}" "${prometheus}/api/v1/query" |
    jq -e '.status == "success" and (.data.result | length) > 0'
}

prometheus_value() {
  local query="$1"
  curl -fsSG --data-urlencode "query=${query}" "${prometheus}/api/v1/query" |
    jq -er '.data.result[0].value[1]'
}

jaeger_has_kc() {
  curl -fsS "${jaeger}/api/services" |
    jq -e '.data | index("kc-server") != null'
}

jaeger_has_search_operation() {
  curl -fsSG --data-urlencode 'service=kc-server' "${jaeger}/api/operations" |
    jq -e '[.data[] | if type == "object" then .name else . end] | index("kc.search") != null'
}

jaeger_has_trace_id() {
  local trace_id="$1"
  curl -fsSG \
    --data-urlencode 'service=kc-server' \
    --data-urlencode 'limit=100' \
    "${jaeger}/api/traces" |
    jq -e --arg traceID "${trace_id}" '.data | any(.traceID == $traceID)'
}

grafana_has_dashboard() {
  local uid="$1"
  local minimum_panels="$2"
  curl -fsS "${grafana}/api/dashboards/uid/${uid}" |
    jq -e --argjson minimum "${minimum_panels}" '.dashboard.panels | length >= $minimum'
}

prometheus_has_reference_alerts() {
  curl -fsSG --data-urlencode 'type=alert' "${prometheus}/api/v1/rules" |
    jq -e '[.data.groups[].rules[] | select(.type == "alerting")] | length >= 7'
}

validate_dashboard_queries() {
  local dashboard
  local expr
  for dashboard in "${observability_dir}"/grafana/kc-*.json; do
    while IFS= read -r expr; do
      curl -fsSG --data-urlencode "query=${expr}" "${prometheus}/api/v1/query" |
        jq -e '.status == "success"' >/dev/null
    done < <(
      jq -r '
        .. | objects | select(.datasource?.uid == "kc-prometheus") | .targets[]?.expr? // empty |
        gsub("\\$__range"; "1h") |
        gsub("\\$__rate_interval"; "5m") |
        gsub("\\$provider"; ".*")
      ' "${dashboard}"
    )
  done
}

validate_loki_dashboard_queries() {
  local expr
  while IFS= read -r expr; do
    curl -fsSG \
      --data-urlencode "query=${expr}" \
      --data-urlencode 'since=1h' \
      --data-urlencode 'limit=2' \
      "${loki}/loki/api/v1/query_range" |
      jq -e '.status == "success"' >/dev/null
  done < <(
    jq -r '
      .. | objects | select(.datasource?.uid == "kc-loki") | .targets[]?.expr? // empty |
      gsub("\\$__range"; "1h") |
      gsub("\\$__interval"; "1m")
    ' "${observability_dir}/grafana/kc-logs.json"
  )
}

validate_dashboard_links() {
  local dashboard
  for dashboard in "${observability_dir}"/grafana/kc-*.json; do
    jq -e '
      [.links[]? | select(.url | contains("16686/search?service=kc-server&operation=kc.search&lookback=1h"))] |
      length == 1
    ' "${dashboard}" >/dev/null
  done
}

wait_for "KC Server" curl -fsS "${server}/readyz"
wait_for "Prometheus" curl -fsS "${prometheus}/-/ready"
wait_for "Jaeger" curl -fsS "${jaeger}/"
wait_for "Loki" curl -fsS "${loki}/ready"
wait_for "Grafana" curl -fsS "${grafana}/api/health"
wait_for "Prometheus KC target" prometheus_has 'up{job="knowledge-catalog"} == 1'

# Generate complete SEARCH observations through the same long-lived server that
# Prometheus scrapes and whose spans are exported over OTLP.
request_id="obs-smoke-$(date +%s)-$$"
for attempt in {1..5}; do
  current_request_id="${request_id}-${attempt}"
  curl -fsS -X POST "${server}/knowledge/v1/search" \
    -H 'Content-Type: application/json' \
    -H 'X-Kc-As: agent:dsh' \
    -H "X-Kc-Request-Id: ${current_request_id}" \
    -d '{"catalog":"kr://dw/catalog","workspace":"warehouse-agent","query":"lineitem","limit":20}' |
    jq -e '.completeness == "complete" and (.hits | length) > 0' >/dev/null
done

loki_has_correlated_request() {
  curl -fsSG \
    --data-urlencode "query={service_name=\"kc-server\"} | kc_request_id = \"${request_id}-5\"" \
    --data-urlencode 'since=10m' \
    --data-urlencode 'limit=20' \
    "${loki}/loki/api/v1/query_range" |
    jq -e '.status == "success" and (.data.result | length) > 0'
}

wait_for "raw SEARCH metrics" prometheus_has 'kc_search_requests_total{kc_search_completeness="complete",kc_outcome="ok"} > 0'
wait_for "SEARCH P95 recording rule" prometheus_has 'kc:sli:search_latency_seconds:p95 >= 0'
wait_for "SEARCH phase recording rule" prometheus_has 'kc:sli:search_phase_latency_seconds:p95 >= 0'
wait_for "Prometheus reference alerts" prometheus_has_reference_alerts
wait_for "KC traces in Jaeger" jaeger_has_kc
wait_for "SEARCH operation in Jaeger" jaeger_has_search_operation
wait_for "correlated completion log in Loki" loki_has_correlated_request
wait_for "Grafana system overview" grafana_has_dashboard kc-overview 13
wait_for "Grafana SEARCH analysis" grafana_has_dashboard kc-search-analysis 12
wait_for "Grafana runtime health" grafana_has_dashboard kc-runtime-health 12
wait_for "Grafana diagnostic logs" grafana_has_dashboard kc-logs 8

curl -fsS "${grafana}/api/datasources/uid/kc-prometheus" | jq -e '.type == "prometheus"' >/dev/null
curl -fsS "${grafana}/api/datasources/uid/kc-jaeger" | jq -e '.type == "jaeger"' >/dev/null
curl -fsS "${grafana}/api/datasources/uid/kc-loki" | jq -e '.type == "loki"' >/dev/null
curl -fsS "${loki}/loki/api/v1/labels" |
  jq -e '.status == "success" and (.data | sort) == ["service_name", "service_namespace"]' >/dev/null
validate_dashboard_queries
validate_loki_dashboard_queries
validate_dashboard_links

log_payload="$(curl -fsSG \
  --data-urlencode "query={service_name=\"kc-server\"} | kc_request_id = \"${request_id}-5\"" \
  --data-urlencode 'since=10m' \
  --data-urlencode 'limit=20' \
  "${loki}/loki/api/v1/query_range")"
correlated_trace_id="$(jq -er '[.data.result[].stream.trace_id // empty][0]' <<<"${log_payload}")"
wait_for "same trace in Jaeger" jaeger_has_trace_id "${correlated_trace_id}"

p95="$(prometheus_value 'kc:sli:search_latency_seconds:p95')"
p99="$(prometheus_value 'kc:sli:search_latency_seconds:p99')"
availability="$(prometheus_value 'kc:sli:search_availability:ratio')"
trace_count="$(curl -fsSG --data-urlencode 'service=kc-server' --data-urlencode 'limit=20' "${jaeger}/api/traces" | jq -er '.data | length')"
alert_rule_count="$(curl -fsSG --data-urlencode 'type=alert' "${prometheus}/api/v1/rules" | jq -er '[.data.groups[].rules[] | select(.type == "alerting")] | length')"

jq -n \
  --arg prometheus "${prometheus}" \
  --arg jaeger "${jaeger}" \
  --arg loki "${loki}" \
  --arg grafana "${grafana}/d/kc-overview/knowledge-catalog-system-overview" \
  --arg searchDashboard "${grafana}/d/kc-search-analysis/knowledge-catalog-search-analysis" \
  --arg runtimeDashboard "${grafana}/d/kc-runtime-health/knowledge-catalog-runtime-health" \
  --arg logsDashboard "${grafana}/d/kc-logs/knowledge-catalog-diagnostic-logs" \
  --arg correlatedTraceId "${correlated_trace_id}" \
  --argjson searchP95Seconds "${p95}" \
  --argjson searchP99Seconds "${p99}" \
  --argjson searchAvailability "${availability}" \
  --argjson traceCount "${trace_count}" \
  --argjson alertRuleCount "${alert_rule_count}" \
  '{
    ok: true,
    prometheus: $prometheus,
    jaeger: $jaeger,
    loki: $loki,
    dashboards: {
      systemOverview: $grafana,
      searchAnalysis: $searchDashboard,
      runtimeHealth: $runtimeDashboard,
      diagnosticLogs: $logsDashboard
    },
    observed: {
      searchP95Seconds: $searchP95Seconds,
      searchP99Seconds: $searchP99Seconds,
      searchAvailability: $searchAvailability,
      traceCount: $traceCount,
      alertRuleCount: $alertRuleCount,
      correlatedTraceId: $correlatedTraceId
    }
  }'
