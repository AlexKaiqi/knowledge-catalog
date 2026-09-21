#!/usr/bin/env bash
# First-stage lakeFS topology as two isolated compose projects:
#   local — disposable test stack (wipe volumes on down)
#   dev    — persistent desktop stack (restart keeps authority + ports)
# ttyd is the only kc CLI page.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

case "${1:-}" in
  local|dev)
    KC_DEPLOY_PROFILE="$1"
    shift
    ;;
esac
PROFILE="${KC_DEPLOY_PROFILE:-local}"
CMD="${1:-up}"

COMPOSE_FILE="${ROOT}/scripts/deploy/compose.yaml"
COMPOSE_DEV_FILE="${ROOT}/scripts/deploy/compose.dev.yaml"

case "$PROFILE" in
  local)
    PROJECT="${KC_DEPLOY_PROJECT:-kc-deploy-local}"
    ENV_DIR="${KC_DEPLOY_HOME:-/tmp/kc-deploy-local}"
    WIPE_ON_DOWN=1
    ALLOW_SCENES=1
    COMPOSE_FILES=(-f "$COMPOSE_FILE")
    PORT_SERVER_CANDIDATES="7380 7381 7382 7383 7393 7394"
    PORT_LAKEFS_CANDIDATES="18000 18001 18002 18003 8001 8002"
    PORT_MINIO_CANDIDATES="19000 19001 19002 19003 9002 9003"
    PORT_MINIO_CONSOLE_CANDIDATES="19011 19101 9001 9004 19004 19005"
    PORT_OPENSEARCH_CANDIDATES="19200 19201 19202 9201 9202 9203"
    PORT_OPENSEARCH_DASHBOARDS_CANDIDATES="15601 5601 19211 19212 5602 5603"
    PORT_GRAFANA_CANDIDATES="7300 7301 7302 7303 3001 3002"
    PORT_PROMETHEUS_CANDIDATES="19090 19091 19092 9091 9092 9093"
    PORT_OTLP_CANDIDATES="14318 14319 14320 4319 4320 4321"
    PORT_TTYD_CANDIDATES="7682 7683 7684 7686 7688 7689 7690 7691"
    ;;
  dev)
    PROJECT="${KC_DEPLOY_PROJECT:-kc-deploy-dev}"
    ENV_DIR="${KC_DEPLOY_HOME:-${HOME}/.kc/deploy-dev}"
    WIPE_ON_DOWN=0
    ALLOW_SCENES=0
    COMPOSE_FILES=(-f "$COMPOSE_FILE" -f "$COMPOSE_DEV_FILE")
    # Host ports are fixed so dev bookmarks and KC_PUBLIC_URL stay stable.
    # Occupied by anything other than this project: fail, do not rematch.
    DEV_SERVER_PORT=7385
    DEV_LAKEFS_PORT=18010
    DEV_MINIO_PORT=19010
    DEV_MINIO_CONSOLE_PORT=19021
    DEV_OPENSEARCH_PORT=19210
    DEV_OPENSEARCH_DASHBOARDS_PORT=15611
    DEV_GRAFANA_PORT=7310
    DEV_PROMETHEUS_PORT=19190
    DEV_OTLP_PORT=14328
    DEV_TTYD_PORT=7692
    # Separate image tags so dev rebuilds do not retag the running local stack.
    KC_DEPLOY_KC_IMAGE="${KC_DEPLOY_KC_IMAGE:-kc-deploy-dev:local}"
    KC_DEPLOY_CLI_IMAGE="${KC_DEPLOY_CLI_IMAGE:-kc-deploy-dev-cli:local}"
    export KC_DEPLOY_KC_IMAGE KC_DEPLOY_CLI_IMAGE
    ;;
  *)
    echo "FAIL: KC_DEPLOY_PROFILE must be local or dev" >&2
    exit 2
    ;;
esac

ENV_FILE="${ENV_DIR}/compose.env"
export KC_DEPLOY_PROFILE="$PROFILE"
export KC_DEPLOY_PROJECT="$PROJECT"
export KC_DEPLOY_HOME="$ENV_DIR"

detect_access_host() {
  local iface addr
  while read -r _ iface _ addr _; do
    case "$iface" in
      docker*|br-*|veth*|virbr*|lo) continue ;;
    esac
    printf '%s' "${addr%%/*}"
    return 0
  done < <(ip -4 -o addr show scope global 2>/dev/null || true)
  hostname -I 2>/dev/null | awk '{print $1}'
}

apply_dest_publish() {
  [[ "$PROFILE" == "dev" ]] || return 0
  KC_DEPLOY_BIND="${KC_DEPLOY_BIND:-0.0.0.0}"
  if [[ -z "${KC_DEPLOY_ACCESS_HOST:-}" ]]; then
    KC_DEPLOY_ACCESS_HOST="$(detect_access_host)"
  fi
  export KC_DEPLOY_BIND KC_DEPLOY_ACCESS_HOST
}

ensure_ttyd_credential() {
  [[ "$PROFILE" == "dev" ]] || return 0
  if [[ -z "${KC_TTYD_CREDENTIAL:-}" ]]; then
    KC_TTYD_CREDENTIAL="kc:$(openssl rand -hex 12)"
  fi
  export KC_TTYD_CREDENTIAL
}

port_in_use() {
  ss -H -ltn "sport = :$1" 2>/dev/null | grep -q .
}

project_owns_port() {
  local port="$1"
  docker ps -a --filter "label=com.docker.compose.project=${PROJECT}" --format '{{.Ports}}' 2>/dev/null |
    grep -qE ":${port}->"
}

choose_port() {
  local current="$1"
  local candidates="$2"
  local label="$3"
  if [[ -n "$current" ]]; then
    printf '%s' "$current"
    return
  fi
  local candidate
  for candidate in $candidates; do
    if ! port_in_use "$candidate"; then
      printf '%s' "$candidate"
      return
    fi
  done
  echo "FAIL: no free local port for $label" >&2
  exit 1
}

require_persisted_port() {
  local port="$1"
  local label="$2"
  if [[ -z "$port" ]]; then
    echo "FAIL: ${ENV_FILE} is missing ${label}; start this dev stack once with up" >&2
    exit 1
  fi
  if port_in_use "$port" && ! project_owns_port "$port"; then
    echo "FAIL: dev ${label} port ${port} is in use; stop the other listener (dev ports are fixed)" >&2
    exit 1
  fi
}

load_env() {
  if [[ -f "$ENV_FILE" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +a
  fi
}

write_env() {
  mkdir -p "$ENV_DIR"
  cat >"$ENV_FILE" <<EOF
KC_DEPLOY_PROFILE=${PROFILE}
KC_DEPLOY_SERVER_PORT=${KC_DEPLOY_SERVER_PORT}
KC_DEPLOY_LAKEFS_PORT=${KC_DEPLOY_LAKEFS_PORT}
KC_DEPLOY_MINIO_PORT=${KC_DEPLOY_MINIO_PORT}
KC_DEPLOY_MINIO_CONSOLE_PORT=${KC_DEPLOY_MINIO_CONSOLE_PORT}
KC_DEPLOY_OPENSEARCH_PORT=${KC_DEPLOY_OPENSEARCH_PORT}
KC_DEPLOY_OPENSEARCH_DASHBOARDS_PORT=${KC_DEPLOY_OPENSEARCH_DASHBOARDS_PORT}
KC_DEPLOY_GRAFANA_PORT=${KC_DEPLOY_GRAFANA_PORT}
KC_DEPLOY_PROMETHEUS_PORT=${KC_DEPLOY_PROMETHEUS_PORT}
KC_DEPLOY_OTLP_HTTP_PORT=${KC_DEPLOY_OTLP_HTTP_PORT}
KC_DEPLOY_TTYD_PORT=${KC_DEPLOY_TTYD_PORT}
KC_PUBLIC_URL=${KC_PUBLIC_URL}
KC_LAKEFS_PUBLIC_URL=${KC_LAKEFS_PUBLIC_URL:-}
KC_OPENSEARCH_PUBLIC_URL=${KC_OPENSEARCH_PUBLIC_URL:-}
KC_CATALOG=${KC_CATALOG:-kr://acme/catalog}
KC_AS=${KC_AS:-admin}
KC_MINIO_USER=${KC_MINIO_USER:-kcminio}
KC_MINIO_PASSWORD=${KC_MINIO_PASSWORD:-kcminio-secret}
KC_POSTGRES_PASSWORD=${KC_POSTGRES_PASSWORD:-kc-lakefs-postgres}
KC_LAKEFS_ENCRYPT_KEY=${KC_LAKEFS_ENCRYPT_KEY:-kc-deploy-lakefs-encrypt-key}
KC_LAKEFS_ACCESS_KEY=${KC_LAKEFS_ACCESS_KEY:-AKIAKOSLOCALEXAMPLE}
KC_LAKEFS_SECRET_KEY=${KC_LAKEFS_SECRET_KEY:-wJalrXUtnFEMILocalLakeFSSecret12}
KC_DEPLOY_KC_IMAGE=${KC_DEPLOY_KC_IMAGE:-}
KC_DEPLOY_CLI_IMAGE=${KC_DEPLOY_CLI_IMAGE:-}
KC_DEPLOY_BIND=${KC_DEPLOY_BIND:-127.0.0.1}
KC_DEPLOY_ACCESS_HOST=${KC_DEPLOY_ACCESS_HOST:-127.0.0.1}
KC_TTYD_CREDENTIAL=${KC_TTYD_CREDENTIAL:-}
EOF
}

compose() {
  local args=(docker compose --project-name "$PROJECT")
  if [[ -f "$ENV_FILE" ]]; then
    args+=(--env-file "$ENV_FILE")
  fi
  args+=("${COMPOSE_FILES[@]}")
  "${args[@]}" "$@"
}

print_access() {
  local catalog="${KC_CATALOG:-kr://acme/catalog}"
  local minio_user="${KC_MINIO_USER:-kcminio}"
  local minio_password="${KC_MINIO_PASSWORD:-kcminio-secret}"
  local lakefs_key="${KC_LAKEFS_ACCESS_KEY:-AKIAKOSLOCALEXAMPLE}"
  local lakefs_secret="${KC_LAKEFS_SECRET_KEY:-wJalrXUtnFEMILocalLakeFSSecret12}"
  local postgres_password="${KC_POSTGRES_PASSWORD:-kc-lakefs-postgres}"
  local stack_label stop_hint bind_note url_host ttyd_auth
  url_host="127.0.0.1"
  bind_note="绑 127.0.0.1；远程 Cursor 请转发这些端口"
  ttyd_auth="登录                    kc login --mode local --as admin"
  if [[ "$PROFILE" == "dev" ]]; then
    stack_label="本机开发（持久；compose 项目 ${PROJECT}）"
    stop_hint="停栈（保卷）：make deploy-dev-down
清盘：            make deploy-dev-reset
再打印本卡：      make deploy-dev-access"
    apply_dest_publish
    url_host="${KC_DEPLOY_ACCESS_HOST:-127.0.0.1}"
    bind_note="绑 ${KC_DEPLOY_BIND:-0.0.0.0}；用主机 IP ${url_host}（127.0.0.1 仍可用）"
    if [[ -n "${KC_TTYD_CREDENTIAL:-}" ]]; then
      ttyd_auth="浏览器 basic auth     ${KC_TTYD_CREDENTIAL}
  登录                    kc login --mode local --as admin"
    fi
  else
    stack_label="local 测试（清盘；compose 项目 ${PROJECT}）"
    stop_hint="停栈并清卷：make deploy-local-down
再打印本卡：    make deploy-local-access"
  fi
  cat <<EOF

访问卡（${stack_label}，${bind_note}）

kc 观察台（产品前端：Catalog、成员仓、Dataset、检索投影、Snapshot 权威）
  http://${url_host}:${KC_DEPLOY_SERVER_PORT}/console
  本地登录用户名          admin
  Catalog                 ${catalog}
  System                  kr://kc/system
  typed API 根路径        http://${url_host}:${KC_DEPLOY_SERVER_PORT}/  是 404，前端在 /console

敲 kc（不是观察台）
  ttyd                    http://${url_host}:${KC_DEPLOY_TTYD_PORT}/
  ${ttyd_auth}

lakeFS
  UI                      http://${url_host}:${KC_DEPLOY_LAKEFS_PORT}/
  Access Key ID           ${lakefs_key}
  Secret Access Key       ${lakefs_secret}
  物理仓                  kc-catalog  kc-system
  业务知识仓              启动不建；接入方 attach / create 之后才有

MinIO（本地 COS，不是生产）
  Console                 http://${url_host}:${KC_DEPLOY_MINIO_CONSOLE_PORT}/
  S3 API                  http://${url_host}:${KC_DEPLOY_MINIO_PORT}
  用户                    ${minio_user}
  密码                    ${minio_password}
  bucket                  kc-authority

OpenSearch
  API                     http://${url_host}:${KC_DEPLOY_OPENSEARCH_PORT}/
  Dashboards              http://${url_host}:${KC_DEPLOY_OPENSEARCH_DASHBOARDS_PORT}/
  账号                    无（安全插件已关；Dashboards 仅本地查看，不是生产陪伴件）

可观测（grafana/otel-lgtm 一个容器）
  Grafana                 http://${url_host}:${KC_DEPLOY_GRAFANA_PORT}/  匿名 Viewer，无登录
  总览                    http://${url_host}:${KC_DEPLOY_GRAFANA_PORT}/d/kc-overview/
  运行健康                http://${url_host}:${KC_DEPLOY_GRAFANA_PORT}/d/kc-runtime-health/
  检索分析                http://${url_host}:${KC_DEPLOY_GRAFANA_PORT}/d/kc-search-analysis/
  诊断日志                http://${url_host}:${KC_DEPLOY_GRAFANA_PORT}/d/kc-logs/
  容量与行为              http://${url_host}:${KC_DEPLOY_GRAFANA_PORT}/d/kc-capacity-behavior/
  Explore                 Tempo / Loki (KC) / Prometheus (KC)
  Prometheus              http://${url_host}:${KC_DEPLOY_PROMETHEUS_PORT}/  无账号
  OTLP HTTP               http://${url_host}:${KC_DEPLOY_OTLP_HTTP_PORT}  采集口，不是页面
  Tempo / Loki / Pyroscope 只在容器内，从 Grafana Explore 看；没有独立 Jaeger 页

PostgreSQL（只给 lakeFS，不映射到宿主机）
  容器内                  postgres://lakefs:${postgres_password}@postgres:5432/lakefs

${stop_hint}
EOF
}

assign_ports() {
  if [[ "$PROFILE" == "dev" ]]; then
    KC_DEPLOY_SERVER_PORT="$DEV_SERVER_PORT"
    KC_DEPLOY_LAKEFS_PORT="$DEV_LAKEFS_PORT"
    KC_DEPLOY_MINIO_PORT="$DEV_MINIO_PORT"
    KC_DEPLOY_MINIO_CONSOLE_PORT="$DEV_MINIO_CONSOLE_PORT"
    KC_DEPLOY_OPENSEARCH_PORT="$DEV_OPENSEARCH_PORT"
    KC_DEPLOY_OPENSEARCH_DASHBOARDS_PORT="$DEV_OPENSEARCH_DASHBOARDS_PORT"
    KC_DEPLOY_GRAFANA_PORT="$DEV_GRAFANA_PORT"
    KC_DEPLOY_PROMETHEUS_PORT="$DEV_PROMETHEUS_PORT"
    KC_DEPLOY_OTLP_HTTP_PORT="$DEV_OTLP_PORT"
    KC_DEPLOY_TTYD_PORT="$DEV_TTYD_PORT"
    require_persisted_port "$KC_DEPLOY_SERVER_PORT" kc-server
    require_persisted_port "$KC_DEPLOY_LAKEFS_PORT" lakefs
    require_persisted_port "$KC_DEPLOY_MINIO_PORT" minio
    require_persisted_port "$KC_DEPLOY_MINIO_CONSOLE_PORT" minio-console
    require_persisted_port "$KC_DEPLOY_OPENSEARCH_PORT" opensearch
    require_persisted_port "$KC_DEPLOY_OPENSEARCH_DASHBOARDS_PORT" opensearch-dashboards
    require_persisted_port "$KC_DEPLOY_GRAFANA_PORT" grafana
    require_persisted_port "$KC_DEPLOY_PROMETHEUS_PORT" prometheus
    require_persisted_port "$KC_DEPLOY_OTLP_HTTP_PORT" otlp
    require_persisted_port "$KC_DEPLOY_TTYD_PORT" ttyd
    return
  fi
  KC_DEPLOY_SERVER_PORT="$(choose_port "${KC_DEPLOY_SERVER_PORT:-}" "$PORT_SERVER_CANDIDATES" kc-server)"
  KC_DEPLOY_LAKEFS_PORT="$(choose_port "${KC_DEPLOY_LAKEFS_PORT:-}" "$PORT_LAKEFS_CANDIDATES" lakefs)"
  KC_DEPLOY_MINIO_PORT="$(choose_port "${KC_DEPLOY_MINIO_PORT:-}" "$PORT_MINIO_CANDIDATES" minio)"
  KC_DEPLOY_MINIO_CONSOLE_PORT="$(choose_port "${KC_DEPLOY_MINIO_CONSOLE_PORT:-}" "$PORT_MINIO_CONSOLE_CANDIDATES" minio-console)"
  KC_DEPLOY_OPENSEARCH_PORT="$(choose_port "${KC_DEPLOY_OPENSEARCH_PORT:-}" "$PORT_OPENSEARCH_CANDIDATES" opensearch)"
  KC_DEPLOY_OPENSEARCH_DASHBOARDS_PORT="$(choose_port "${KC_DEPLOY_OPENSEARCH_DASHBOARDS_PORT:-}" "$PORT_OPENSEARCH_DASHBOARDS_CANDIDATES" opensearch-dashboards)"
  KC_DEPLOY_GRAFANA_PORT="$(choose_port "${KC_DEPLOY_GRAFANA_PORT:-}" "$PORT_GRAFANA_CANDIDATES" grafana)"
  KC_DEPLOY_PROMETHEUS_PORT="$(choose_port "${KC_DEPLOY_PROMETHEUS_PORT:-}" "$PORT_PROMETHEUS_CANDIDATES" prometheus)"
  KC_DEPLOY_OTLP_HTTP_PORT="$(choose_port "${KC_DEPLOY_OTLP_HTTP_PORT:-}" "$PORT_OTLP_CANDIDATES" otlp)"
  KC_DEPLOY_TTYD_PORT="$(choose_port "${KC_DEPLOY_TTYD_PORT:-}" "$PORT_TTYD_CANDIDATES" ttyd)"
}

cmd_up() {
  local observe_host
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    echo "FAIL: Docker daemon is unavailable" >&2
    exit 1
  fi
  load_env
  assign_ports
  apply_dest_publish
  ensure_ttyd_credential
  if [[ "$PROFILE" == "dev" && -n "${KC_DEPLOY_ACCESS_HOST:-}" ]]; then
    KC_PUBLIC_URL="http://${KC_DEPLOY_ACCESS_HOST}:${KC_DEPLOY_SERVER_PORT}"
    observe_host="${KC_DEPLOY_ACCESS_HOST}"
  else
    KC_PUBLIC_URL="${KC_PUBLIC_URL:-http://127.0.0.1:${KC_DEPLOY_SERVER_PORT}}"
    observe_host="127.0.0.1"
  fi
  KC_LAKEFS_PUBLIC_URL="${KC_LAKEFS_PUBLIC_URL:-http://${observe_host}:${KC_DEPLOY_LAKEFS_PORT}}"
  KC_OPENSEARCH_PUBLIC_URL="${KC_OPENSEARCH_PUBLIC_URL:-http://${observe_host}:${KC_DEPLOY_OPENSEARCH_PORT}}"
  write_env
  compose build bootstrap cli
  compose up --detach --wait --no-build
  "$ROOT/scripts/deploy/smoke.sh"
  print_access
}

cmd_status() {
  load_env
  if ! compose ps >/dev/null 2>&1; then
    echo "compose project $PROJECT is not running"
    exit 1
  fi
  compose ps
  print_access
}

cmd_access() {
  load_env
  if [[ -z "${KC_DEPLOY_TTYD_PORT:-}" ]]; then
    echo "FAIL: ${ENV_FILE} missing; start with ./scripts/deploy/deploy.sh ${PROFILE} up" >&2
    exit 1
  fi
  print_access
}

cmd_smoke() {
  load_env
  "$ROOT/scripts/deploy/smoke.sh"
}

cmd_scenes() {
  if [[ "$ALLOW_SCENES" != "1" ]]; then
    echo "FAIL: live scenes run only on the local test stack" >&2
    exit 1
  fi
  if [[ ! -f "$ENV_FILE" ]]; then
    echo "FAIL: local lakeFS test stack is not configured; prepare it with make deploy-local-up" >&2
    exit 1
  fi
  load_env
  local filter="${KC_DEPLOY_SCENE_RUN:-^TestLiveLakeFSSceneDFS$}"
  local user
  user="$(id -u):$(id -g)"
  local evidence_args=()
  if [[ -n "${KC_VALIDATION_RUN_ID:-}" ]]; then
    local container_run_dir
    case "${KC_VALIDATION_RUN_DIR:-}" in
      "$ROOT"/*) container_run_dir="/src/${KC_VALIDATION_RUN_DIR#"$ROOT"/}" ;;
      *)
        echo "FAIL: scene evidence directory must be inside the repository mounted at /src" >&2
        exit 1
        ;;
    esac
    evidence_args=(
      -e "KC_VALIDATION_RUN_ID=$KC_VALIDATION_RUN_ID"
      -e "KC_VALIDATION_RUN_DIR=$container_run_dir"
      -e "KC_VALIDATION_SOURCE_FINGERPRINT=${KC_VALIDATION_SOURCE_FINGERPRINT:-}"
      -e "KC_VALIDATION_SCOPE=${KC_VALIDATION_SCOPE:-}"
    )
  fi
  compose --profile scenes run --rm --no-deps --user "$user" \
    ${evidence_args[@]+"${evidence_args[@]}"} \
    scenes test -json ./cli -count=1 -timeout 90m -run "$filter"
}

wipe_stack() {
  "$ROOT/scripts/deploy/accessors.sh" down || true
  load_env
  if [[ -f "$ENV_FILE" ]] || docker compose --project-name "$PROJECT" "${COMPOSE_FILES[@]}" ps -q >/dev/null 2>&1; then
    compose down --volumes --remove-orphans >/dev/null
  fi
  rm -rf "$ENV_DIR"
  echo "reset $PROJECT"
}

cmd_down() {
  if [[ "$WIPE_ON_DOWN" == "1" ]]; then
    wipe_stack
    return
  fi
  load_env
  if [[ ! -f "$ENV_FILE" ]] && ! docker compose --project-name "$PROJECT" "${COMPOSE_FILES[@]}" ps -q >/dev/null 2>&1; then
    echo "stopped $PROJECT"
    return 0
  fi
  "$ROOT/scripts/deploy/accessors.sh" down || true
  compose down --remove-orphans >/dev/null
  echo "stopped $PROJECT"
}

cmd_reset() {
  wipe_stack
}

cmd_goto() {
  if [[ "$PROFILE" != "local" ]]; then
    echo "FAIL: scene goto is the local walk stack" >&2
    exit 1
  fi
  load_env
  if [[ -z "${KC_DEPLOY_SERVER_PORT:-}" ]]; then
    echo "FAIL: start the local stack first: make deploy-local-up" >&2
    exit 1
  fi
  shift || true
  local target="${1:-}"
  if [[ -z "$target" ]]; then
    echo "usage: $0 local goto <scene-path-or-id> [--probe <feature>]" >&2
    exit 2
  fi
  export KC_SERVER_URL="http://127.0.0.1:${KC_DEPLOY_SERVER_PORT}"
  export KC_CATALOG="${KC_CATALOG:-kr://acme/catalog}"
  export KC_AS="${KC_AS:-admin}"
  python3 "$ROOT/.data/scenes/goto.py" "$@"
}

case "$CMD" in
  up) cmd_up ;;
  status) cmd_status ;;
  access) cmd_access ;;
  smoke) cmd_smoke ;;
  scenes) cmd_scenes ;;
  goto) cmd_goto "$@" ;;
  down) cmd_down ;;
  reset) cmd_reset ;;
  *)
    echo "usage: $0 [local|dev] up|status|access|smoke|scenes|goto|down|reset" >&2
    exit 2
    ;;
esac
