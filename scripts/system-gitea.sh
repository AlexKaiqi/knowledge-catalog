#!/usr/bin/env bash
# Start a persistent Docker Gitea, publish the built-in System Schema into
# kr://kc/system, and print the KC_HOME / token a local serve can reuse.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CMD="${1:-up}"
NAME="${KC_SYSTEM_GITEA_CONTAINER:-kc-system-gitea}"
IMAGE="${KC_SYSTEM_GITEA_IMAGE:-gitea/gitea:1.26.3}"
PORT="${KC_SYSTEM_GITEA_PORT:-}"
VOLUME="${KC_SYSTEM_GITEA_VOLUME:-kc-system-gitea-data}"
HOME_DIR="${KC_HOME:-/tmp/kc-system-gitea}"
GO="${GO:-go}"
BASE=""
DSN=""
ENV_FILE="${HOME_DIR}/system-gitea.env"
CATALOG="${KC_CATALOG:-kr://acme/catalog}"
PRINCIPAL="${KC_AS:-user:local-admin}"

sync_endpoints() {
  BASE="http://127.0.0.1:${PORT}"
  DSN="${BASE}/kc/system"
}

port_in_use() {
  ss -H -ltn "sport = :$1" 2>/dev/null | grep -q .
}

choose_port() {
  local mapped candidate
  mapped="$(docker inspect -f '{{with (index (index .NetworkSettings.Ports "3000/tcp") 0)}}{{.HostPort}}{{end}}' "$NAME" 2>/dev/null || true)"
  if docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null | grep -q true && [[ -n "$mapped" ]]; then
    PORT="$mapped"
    sync_endpoints
    return
  fi
  if [[ -n "$PORT" ]]; then
    sync_endpoints
    return
  fi
  for candidate in 13001 13002 13003 13004 13005 3001 3002; do
    if ! port_in_use "$candidate"; then
      PORT="$candidate"
      sync_endpoints
      return
    fi
  done
  echo "FAIL: no free local port; set KC_SYSTEM_GITEA_PORT" >&2
  exit 1
}

gitea_exec() {
  local out err
  out="$(mktemp)"
  err="$(mktemp)"
  if docker exec -u git "$NAME" gitea "$@" >"$out" 2>"$err"; then
    cat "$out"
    rm -f "$out" "$err"
    return 0
  fi
  if docker exec "$NAME" gitea "$@" >"$out" 2>"$err"; then
    cat "$out"
    rm -f "$out" "$err"
    return 0
  fi
  cat "$err" >&2
  rm -f "$out" "$err"
  return 1
}

wait_gitea() {
  local deadline=$((SECONDS + 180))
  while (( SECONDS < deadline )); do
    if curl -fsS "$BASE/api/v1/version" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "FAIL: Gitea at $BASE did not become ready" >&2
  docker logs --tail 80 "$NAME" >&2 || true
  return 1
}

ensure_user() {
  local deadline=$((SECONDS + 60)) out=""
  while (( SECONDS < deadline )); do
    if out="$(gitea_exec admin user create --admin --username kc --password kcpass123 --email kc@local --must-change-password=false 2>&1)"; then
      return 0
    fi
    if [[ "$out" == *already* ]]; then
      return 0
    fi
    sleep 2
  done
  echo "FAIL: create Gitea user: $out" >&2
  return 1
}

mint_token() {
  local args out token stamp
  stamp="$(date +%s)"
  for args in \
    "admin user generate-access-token --username kc --token-name kc-system-${stamp} --raw --scopes all" \
    "admin user generate-access-token --username kc --token-name kc-system-write-${stamp} --raw --scopes write:repository,write:user,write:organization" \
    "admin user generate-access-token --username kc --token-name kc-system-plain-${stamp} --raw"
  do
    # shellcheck disable=SC2086
    if out="$(gitea_exec $args 2>/dev/null)"; then
      token="$(printf '%s\n' "$out" | tail -n 1 | tr -d '[:space:]')"
      if [[ -n "$token" && "$token" != *" "* ]]; then
        printf '%s' "$token"
        return 0
      fi
    fi
  done
  echo "FAIL: could not mint a Gitea token" >&2
  return 1
}

write_env() {
  local token="$1"
  mkdir -p "$HOME_DIR"
  cat >"$ENV_FILE" <<EOF
export KC_HOME="$HOME_DIR"
export KC_GITEA_URL="$BASE"
export KC_GITEA_TOKEN="$token"
export KC_SYSTEM_DSN="$DSN"
export KC_AS="$PRINCIPAL"
export KC_SERVER_URL="http://127.0.0.1:7380"
EOF
}

load_token() {
  if [[ -f "$ENV_FILE" ]]; then
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    printf '%s' "${KC_GITEA_TOKEN:-}"
  fi
}

start_container() {
  if docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null | grep -q true; then
    return 0
  fi
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker run -d --name "$NAME" \
    --label kc.system-gitea=1 \
    -p "127.0.0.1:${PORT}:3000" \
    -v "${VOLUME}:/data" \
    -e GITEA__security__INSTALL_LOCK=true \
    -e GITEA__database__DB_TYPE=sqlite3 \
    -e GITEA__server__DOMAIN=127.0.0.1 \
    -e GITEA__server__HTTP_PORT=3000 \
    -e GITEA__server__ROOT_URL="${BASE}/" \
    -e GITEA__server__START_SSH_SERVER=false \
    -e GITEA__service__DISABLE_REGISTRATION=true \
    -e GITEA__repository__DEFAULT_BRANCH=main \
    "$IMAGE" >/dev/null
}

kc_local() {
  "$GO" run ./cmd/kc -- local "$@"
}

ensure_home() {
  if [[ ! -f "$HOME_DIR/layout.yaml" && ! -f "$HOME_DIR/stores.yaml" ]]; then
    kc_local init --home "$HOME_DIR" --catalog "$CATALOG" >/dev/null
  fi
}

publish_once() {
  local token="$1"
  export KC_GITEA_TOKEN="$token"
  ensure_home
  kc_local system publish --home "$HOME_DIR" --driver gitea --dsn "$DSN"
}

seeded_true() {
  grep -Eq '"seeded"[[:space:]]*:[[:space:]]*true' <<<"$1"
}

seeded_false() {
  grep -Eq '"seeded"[[:space:]]*:[[:space:]]*false' <<<"$1"
}

publish_system() {
  local token="$1" out=""
  export KC_GITEA_TOKEN="$token"
  if out="$(publish_once "$token" 2>&1)"; then
    printf '%s\n' "$out"
    return 0
  fi
  echo "WARN: first publish failed, minting a new Gitea token" >&2
  printf '%s\n' "$out" >&2
  token="$(mint_token)"
  write_env "$token"
  publish_once "$token"
}

bootstrap_admin() {
  local out=""
  if out="$(kc_local grant bootstrap --home "$HOME_DIR" --principal "$PRINCIPAL" 2>&1)"; then
    return 0
  fi
  if [[ "$out" == *PRECONDITION_FAILED* || "$out" == *already* ]]; then
    return 0
  fi
  echo "FAIL: grant bootstrap: $out" >&2
  return 1
}

cmd_up() {
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    echo "FAIL: Docker daemon is unavailable" >&2
    exit 1
  fi
  choose_port
  start_container
  wait_gitea
  ensure_user
  local token out replay
  token="$(load_token)"
  if [[ -z "$token" ]]; then
    token="$(mint_token)"
  fi
  write_env "$token"
  echo "publishing System Schema to $DSN"
  out="$(publish_system "$token")"
  printf '%s\n' "$out"
  # Reload in case publish_system minted a replacement token.
  token="$(load_token)"
  export KC_GITEA_TOKEN="$token"
  replay="$(publish_once "$token")"
  if ! seeded_false "$replay"; then
    echo "FAIL: second publish must verify without rewriting:" >&2
    printf '%s\n' "$replay" >&2
    exit 1
  fi
  bootstrap_admin
  echo
  echo "local status after import:"
  kc_local status --home "$HOME_DIR"
  cat <<EOF

System Schema is on Docker Gitea ($DSN).

  source $ENV_FILE
  $GO run ./cmd/kc -- serve --home \$KC_HOME

Read it back from another terminal:

  source $ENV_FILE
  $GO run ./cmd/kc -- knowledge schema browse --repo kr://kc/system --as \$KC_AS

Do not run: kc local repository attach --repo kr://kc/system

Stop with: ./scripts/system-gitea.sh down
Reset volume + home with: KC_SYSTEM_GITEA_RESET=1 ./scripts/system-gitea.sh down
EOF
}

cmd_status() {
  choose_port
  if docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null | grep -q true; then
    echo "container $NAME is running on $BASE"
  else
    echo "container $NAME is not running"
    exit 1
  fi
  curl -fsS "$BASE/api/v1/version"
  echo
  if [[ -f "$ENV_FILE" ]]; then
    echo "env file: $ENV_FILE"
  fi
}

cmd_down() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  if [[ "${KC_SYSTEM_GITEA_RESET:-}" == "1" ]]; then
    docker volume rm "$VOLUME" >/dev/null 2>&1 || true
    rm -rf "$HOME_DIR"
  fi
  echo "stopped $NAME"
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
