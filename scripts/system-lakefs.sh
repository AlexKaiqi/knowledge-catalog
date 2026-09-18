#!/usr/bin/env bash
# Start lakeFS + MinIO, bind Catalog / System / knowledge Snapshot authorities,
# initialize a KC deployment, and print the env a server can reuse.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CMD="${1:-up}"
COMPOSE_FILE="${ROOT}/scripts/lakefs/compose.yaml"
PROJECT="${KC_SYSTEM_LAKEFS_PROJECT:-kc-system-lakefs}"
HOME_DIR="${KC_HOME:-/tmp/kc-system-lakefs}"
GO="${GO:-go}"
ENV_FILE="${HOME_DIR}/system-lakefs.env"
DEPLOYMENT_CONFIG="${HOME_DIR}/deployment.json"
CATALOG="${KC_CATALOG:-kr://acme/catalog}"
PRINCIPAL="${KC_AS:-admin}"
KNOWLEDGE="${KC_KNOWLEDGE_REPO:-kr://acme/knowledge}"
LAKEFS_ACCESS="${KC_LAKEFS_ACCESS_KEY:-AKIAKOSLOCALEXAMPLE}"
LAKEFS_SECRET="${KC_LAKEFS_SECRET_KEY:-wJalrXUtnFEMILocalLakeFSSecret12}"
MINIO_USER="${KC_MINIO_USER:-kcminio}"
MINIO_PASSWORD="${KC_MINIO_PASSWORD:-kcminio-secret}"
LAKEFS_PORT="${KC_LAKEFS_PORT:-}"
MINIO_PORT="${KC_MINIO_PORT:-}"
KC_LISTEN_PORT="${KC_SYSTEM_LAKEFS_LISTEN:-7390}"
TTYD_PORT="${KC_SYSTEM_LAKEFS_TTYD_PORT:-7683}"
TTYD_NAME="${KC_SYSTEM_LAKEFS_TTYD_CONTAINER:-kc-system-lakefs-ttyd}"
SERVE_PID_FILE="${HOME_DIR}/kc-serve.pid"

port_in_use() {
  ss -H -ltn "sport = :$1" 2>/dev/null | grep -q .
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

lakefs_base() {
  printf 'http://127.0.0.1:%s' "$LAKEFS_PORT"
}

compose() {
  KC_MINIO_USER="$MINIO_USER" \
  KC_MINIO_PASSWORD="$MINIO_PASSWORD" \
  KC_LAKEFS_ENCRYPT_KEY="${KC_LAKEFS_ENCRYPT_KEY:-kc-system-lakefs-encrypt-key}" \
  KC_LAKEFS_PORT="$LAKEFS_PORT" \
  KC_MINIO_PORT="$MINIO_PORT" \
  docker compose -p "$PROJECT" -f "$COMPOSE_FILE" "$@"
}

wait_http() {
  local url="$1"
  local deadline=$((SECONDS + 180))
  while (( SECONDS < deadline )); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "FAIL: $url did not become ready" >&2
  compose logs --tail 80 lakefs >&2 || true
  return 1
}

lakefs_basic() {
  printf '%s' "${LAKEFS_ACCESS}:${LAKEFS_SECRET}"
}

setup_lakefs() {
  local status
  status="$(curl -sS -o /tmp/kc-lakefs-setup.json -w '%{http_code}' \
    -X POST "$(lakefs_base)/api/v1/setup_lakefs" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"kc\",\"key\":{\"access_key_id\":\"${LAKEFS_ACCESS}\",\"secret_access_key\":\"${LAKEFS_SECRET}\"}}")"
  case "$status" in
    200|201|409) return 0 ;;
  esac
  echo "FAIL: lakeFS setup returned HTTP $status" >&2
  cat /tmp/kc-lakefs-setup.json >&2 || true
  return 1
}

ensure_repo() {
  local name="$1"
  local namespace="$2"
  local status
  status="$(curl -sS -o /tmp/kc-lakefs-repo.json -w '%{http_code}' \
    -u "$(lakefs_basic)" "$(lakefs_base)/api/v1/repositories/${name}")"
  if [[ "$status" == "200" ]]; then
    return 0
  fi
  status="$(curl -sS -o /tmp/kc-lakefs-repo.json -w '%{http_code}' \
    -u "$(lakefs_basic)" \
    -X POST "$(lakefs_base)/api/v1/repositories" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"${name}\",\"storage_namespace\":\"${namespace}\",\"default_branch\":\"main\"}")"
  case "$status" in
    201) return 0 ;;
  esac
  echo "FAIL: create lakeFS repository ${name} returned HTTP $status" >&2
  cat /tmp/kc-lakefs-repo.json >&2 || true
  return 1
}

write_env() {
  mkdir -p "$HOME_DIR/bin"
  cat >"$ENV_FILE" <<EOF
export KC_DEPLOYMENT_CONFIG="$DEPLOYMENT_CONFIG"
export KC_LAKEFS_URL="$(lakefs_base)"
export KC_LAKEFS_CREDENTIAL="${LAKEFS_ACCESS}:${LAKEFS_SECRET}"
export KC_LAKEFS_PORT="$LAKEFS_PORT"
export KC_MINIO_PORT="$MINIO_PORT"
export KC_MINIO_USER="$MINIO_USER"
export KC_MINIO_PASSWORD="$MINIO_PASSWORD"
export KC_LAKEFS_ENCRYPT_KEY="${KC_LAKEFS_ENCRYPT_KEY:-kc-system-lakefs-encrypt-key}"
export KC_AS="$PRINCIPAL"
export KC_CATALOG="$CATALOG"
export KC_SERVER_URL="http://127.0.0.1:${KC_LISTEN_PORT}"
export KC_TTYD_URL="http://127.0.0.1:${TTYD_PORT}/"
export KC_SNAPSHOT_PLANE=local
export PATH="${HOME_DIR}/bin:\$PATH"
EOF
}

ensure_deployment() {
  export KC_LAKEFS_CREDENTIAL="${LAKEFS_ACCESS}:${LAKEFS_SECRET}"
  if [[ ! -f "$DEPLOYMENT_CONFIG" ]]; then
    "$GO" run ./scripts/fixture-deployment \
      --root "$HOME_DIR" \
      --catalog "$CATALOG" \
      --catalog-driver lakefs \
      --catalog-dsn "$(lakefs_base)/kc-catalog" \
      --principal "$PRINCIPAL" \
      --lakefs-repo "kr://kc/system=$(lakefs_base)/kc-system" \
      --lakefs-repo "${KNOWLEDGE}=$(lakefs_base)/kc-knowledge" >/dev/null
  fi
  "$GO" run ./cmd/kc -- deployment init --config "$DEPLOYMENT_CONFIG" >/dev/null
}

publish_system() {
  export KC_LAKEFS_CREDENTIAL="${LAKEFS_ACCESS}:${LAKEFS_SECRET}"
  "$GO" run ./cmd/kc -- deployment system publish --config "$DEPLOYMENT_CONFIG"
}

build_kc() {
  mkdir -p "$HOME_DIR/bin"
  "$GO" build -o "$HOME_DIR/bin/kc" ./cmd/kc
}

write_bashrc() {
  cat >"${HOME_DIR}/.bash_profile" <<EOF
source "${ENV_FILE}"
PS1='kc-lakefs:\\w\\\$ '
cd "${HOME_DIR}"
echo "KC_SERVER_URL=\$KC_SERVER_URL  catalog=\$KC_CATALOG"
echo "example: kc schema list --repo kr://kc/system --as \$KC_AS"
EOF
}

start_ttyd() {
  docker rm -f "$TTYD_NAME" >/dev/null 2>&1 || true
  write_bashrc
  docker run -d --name "$TTYD_NAME" \
    --network host \
    --label kc.system-lakefs=1 \
    -v "${HOME_DIR}:${HOME_DIR}" \
    -e HOME="${HOME_DIR}" \
    -w "${HOME_DIR}" \
    tsl0922/ttyd:1.7.7 \
    ttyd --port "$TTYD_PORT" --interface 127.0.0.1 -W bash --login >/dev/null
}

stop_serve() {
  if [[ -f "$SERVE_PID_FILE" ]]; then
    local pid
    pid="$(cat "$SERVE_PID_FILE" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
      kill "$pid" >/dev/null 2>&1 || true
      wait "$pid" 2>/dev/null || true
    fi
    rm -f "$SERVE_PID_FILE"
  fi
}

start_serve() {
  stop_serve
  write_env
  build_kc
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  nohup "$HOME_DIR/bin/kc" serve --config "$DEPLOYMENT_CONFIG" --listen "127.0.0.1:${KC_LISTEN_PORT}" \
    >"${HOME_DIR}/kc-serve.log" 2>&1 &
  echo $! >"$SERVE_PID_FILE"
  wait_http "http://127.0.0.1:${KC_LISTEN_PORT}/readyz"
}

cmd_up() {
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    echo "FAIL: Docker daemon is unavailable" >&2
    exit 1
  fi
  LAKEFS_PORT="$(choose_port "$LAKEFS_PORT" "18000 18001 18002 8001 8002" lakefs)"
  MINIO_PORT="$(choose_port "$MINIO_PORT" "19000 19001 19002 9002 9003" minio)"
  KC_LISTEN_PORT="$(choose_port "$KC_LISTEN_PORT" "7390 7391 7392 7382" kc-serve)"
  TTYD_PORT="$(choose_port "$TTYD_PORT" "7683 7684 7685 7686" ttyd)"
  mkdir -p "$HOME_DIR"
  compose up -d
  wait_http "$(lakefs_base)/_health"
  setup_lakefs
  ensure_repo kc-catalog s3://kc-authority/kc-catalog
  ensure_repo kc-system s3://kc-authority/kc-system
  ensure_repo kc-knowledge s3://kc-authority/kc-knowledge
  write_env
  echo "initializing KC deployment on lakeFS"
  ensure_deployment
  echo "publishing System Schema to $(lakefs_base)/kc-system"
  local out replay
  out="$(publish_system)"
  printf '%s\n' "$out"
  replay="$(publish_system)"
  if ! grep -Eq '"seeded"[[:space:]]*:[[:space:]]*false' <<<"$replay"; then
    echo "FAIL: second publish must verify without rewriting:" >&2
    printf '%s\n' "$replay" >&2
    exit 1
  fi
  echo
  echo "deployment status after import:"
  export KC_LAKEFS_CREDENTIAL="${LAKEFS_ACCESS}:${LAKEFS_SECRET}"
  "$GO" run ./cmd/kc -- deployment status --config "$DEPLOYMENT_CONFIG"
  start_serve
  start_ttyd
  cat <<EOF

LakeFS Snapshot is ready.

  lakeFS API     $(lakefs_base)
  MinIO (data)   http://127.0.0.1:${MINIO_PORT}
  KC typed API   http://127.0.0.1:${KC_LISTEN_PORT}   (browser / is 404)
  ttyd (kc CLI)  http://127.0.0.1:${TTYD_PORT}/

Catalog / System / knowledge repositories are independent lakeFS repos.
Credential is injected as KC_LAKEFS_CREDENTIAL; it is not in the DSN.

Stop with: ./scripts/system-lakefs.sh down
Reset volumes + home with: KC_SYSTEM_LAKEFS_RESET=1 ./scripts/system-lakefs.sh down
EOF
}

cmd_status() {
  if [[ -f "$ENV_FILE" ]]; then
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    LAKEFS_PORT="${KC_LAKEFS_PORT:-$LAKEFS_PORT}"
    MINIO_PORT="${KC_MINIO_PORT:-$MINIO_PORT}"
    MINIO_USER="${KC_MINIO_USER:-$MINIO_USER}"
    MINIO_PASSWORD="${KC_MINIO_PASSWORD:-$MINIO_PASSWORD}"
  fi
  if ! compose ps >/dev/null 2>&1; then
    echo "compose project $PROJECT is not running"
    exit 1
  fi
  compose ps
  if [[ -n "${KC_LAKEFS_URL:-}" ]]; then
    curl -fsS "${KC_LAKEFS_URL}/_health" >/dev/null
    echo "lakeFS ${KC_LAKEFS_URL} is healthy"
  fi
}

cmd_down() {
  if [[ -f "$ENV_FILE" ]]; then
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    LAKEFS_PORT="${KC_LAKEFS_PORT:-$LAKEFS_PORT}"
    MINIO_PORT="${KC_MINIO_PORT:-$MINIO_PORT}"
    MINIO_USER="${KC_MINIO_USER:-$MINIO_USER}"
    MINIO_PASSWORD="${KC_MINIO_PASSWORD:-$MINIO_PASSWORD}"
  fi
  stop_serve
  docker rm -f "$TTYD_NAME" >/dev/null 2>&1 || true
  if docker compose -p "$PROJECT" -f "$COMPOSE_FILE" ps -q >/dev/null 2>&1; then
    if [[ "${KC_SYSTEM_LAKEFS_RESET:-}" == "1" ]]; then
      docker compose -p "$PROJECT" -f "$COMPOSE_FILE" down -v >/dev/null
      rm -rf "$HOME_DIR"
    else
      docker compose -p "$PROJECT" -f "$COMPOSE_FILE" down >/dev/null
    fi
  fi
  echo "stopped $PROJECT"
}

case "$CMD" in
  up) cmd_up ;;
  status) cmd_status ;;
  down) cmd_down ;;
  *)
    echo "usage: $0 up|status|down" >&2
    exit 2
    ;;
esac
