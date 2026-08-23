#!/usr/bin/env bash
# Three consecutive, independent, real-model clean-room runs for MVP acceptance.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
ARTIFACT_ROOT="${KC_ACCEPT_ARTIFACTS:-$ROOT/.data/mvp-acceptance/$RUN_ID}"
GITEA_IMAGE="${KC_GITEA_IMAGE:-gitea/gitea:1.26.3}"
GITEA_NAME="kc-mvp-gitea-${RUN_ID//[^A-Za-z0-9]/}"
mkdir -p "$ARTIFACT_ROOT"

cleanup() {
  if [[ -n "${GITEA_STARTED:-}" ]]; then
    docker rm -f "$GITEA_NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

free_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

run_one() {
  local name="$1"
  local topology="$2"
  local port="$3"
  local run_dir="$ARTIFACT_ROOT/$name"
  local session_marker="$run_dir/sessions.started"
  local e2e_status
  local transcript_count
  mkdir -p "$run_dir/home"
  touch "$session_marker"
  printf 'run=%s\ntopology=%s\nport=%s\nstarted=%s\n' "$name" "$topology" "$port" "$(date -u +%FT%TZ)" >"$run_dir/metadata.txt"
  go -C "$ROOT" test ./connector -run '^TestConnectorChangeJourney$' -count=1 2>&1 | tee "$run_dir/connector.log"
  set +e
  KC_E2E_HOME="$run_dir/home" \
  KC_E2E_PORT="$port" \
  KC_E2E_TOPOLOGY="$topology" \
  DSH_PROFILE="kc-mvp-${RUN_ID}-${name}" \
  "$ROOT/dsh-plugin/scripts/e2e-dsh.sh" 2>&1 | tee "$run_dir/run.log"
  e2e_status=${PIPESTATUS[0]}
  set -e
  mkdir -p "$run_dir/transcripts"
  while IFS= read -r transcript; do
    local session_dir
    session_dir="$(basename "$(dirname "$transcript")")"
    cp "$transcript" "$run_dir/transcripts/${session_dir}.jsonl.zstd"
  done < <(find "${DSH_HOME:-$HOME/.dsh}/sessions" -type f -name 'session.jsonl.zstd' -newer "$session_marker" -print)
  find "$run_dir/transcripts" -type f -name '*.jsonl.zstd' -print | sort >"$run_dir/transcripts.txt"
  transcript_count="$(wc -l <"$run_dir/transcripts.txt" | tr -d ' ')"
  if [[ "$transcript_count" -lt 19 ]]; then
    echo "FAIL: $name saved only $transcript_count Agent transcripts; expected at least 19" >&2
    return 1
  fi
  shasum -a 256 "$run_dir"/transcripts/*.jsonl.zstd >"$run_dir/transcripts.sha256"
  cp "$ROOT/dsh-plugin/scripts/deepseek-official.patch.yml" "$run_dir/model.patch.yml"
  if [[ "$e2e_status" -ne 0 ]]; then
    return "$e2e_status"
  fi
  printf 'completed=%s\n' "$(date -u +%FT%TZ)" >>"$run_dir/metadata.txt"
}

echo "acceptance artifacts: $ARTIFACT_ROOT"

echo "==> preflight full Go and dsh-plugin suites"
go -C "$ROOT" test ./... 2>&1 | tee "$ARTIFACT_ROOT/go-test.log"
(cd "$ROOT/dsh-plugin" && npm run typecheck && npm test) 2>&1 | tee "$ARTIFACT_ROOT/dsh-plugin-test.log"
bash -n "$ROOT/dsh-plugin/scripts/e2e-dsh.sh" "$ROOT/dsh-plugin/scripts/accept-mvp.sh"
python3 -m py_compile "$ROOT/dsh-plugin/scripts/e2e_publish_consume.py"

echo "==> R1 FileGit + existing ordinary Git + managed clone"
run_one R1 ordinary-git "$(free_port)"

echo "==> start fresh real Gitea for R2"
GITEA_PORT="$(free_port)"
GITEA_BASE="http://127.0.0.1:${GITEA_PORT}"
docker run -d --rm --name "$GITEA_NAME" \
  -p "${GITEA_PORT}:3000" \
  -e GITEA__security__INSTALL_LOCK=true \
  -e GITEA__database__DB_TYPE=sqlite3 \
  -e GITEA__server__DOMAIN=127.0.0.1 \
  -e GITEA__server__HTTP_PORT=3000 \
  -e GITEA__server__ROOT_URL="${GITEA_BASE}/" \
  -e GITEA__server__START_SSH_SERVER=false \
  -e GITEA__service__DISABLE_REGISTRATION=true \
  -e GITEA__repository__DEFAULT_BRANCH=main \
  "$GITEA_IMAGE" >/dev/null
GITEA_STARTED=1
for _ in $(seq 1 90); do
  if curl -sS -f "$GITEA_BASE/api/v1/version" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
curl -sS -f "$GITEA_BASE/api/v1/version" >"$ARTIFACT_ROOT/R2-gitea-version.json"
docker exec -u git "$GITEA_NAME" gitea admin user create \
  --admin --username kc --password kcpass123 --email kc@local --must-change-password=false >/dev/null
GITEA_TOKEN="$(docker exec -u git "$GITEA_NAME" gitea admin user generate-access-token \
  --username kc --token-name "mvp-${RUN_ID}" --raw --scopes all | tail -1 | tr -d '\r')"
export KC_GITEA_TOKEN="$GITEA_TOKEN"
GITEA_REPO_SUFFIX="$(printf '%s' "$RUN_ID" | tr '[:upper:]' '[:lower:]')"
export KC_GITEA_DSN="$GITEA_BASE/kc/mvp-${GITEA_REPO_SUFFIX}"
echo "==> R2 FileGit + fresh Gitea + managed clone"
run_one R2 gitea "$(free_port)"
docker rm -f "$GITEA_NAME" >/dev/null
GITEA_STARTED=""
unset KC_GITEA_TOKEN KC_GITEA_DSN

echo "==> R3 FileGit + fresh native Dolt + managed clone"
unset KC_DOLT_BIN
export KC_DOLT_DOCKER_IMAGE="${KC_DOLT_DOCKER_IMAGE:-dolthub/dolt@sha256:0cc3822d9fafbb589a7f38da465aa21a813460cf5fae728bc95bb068f814ca01}"
export KC_DOLT_FORCE_DOCKER=1
docker run --rm "$KC_DOLT_DOCKER_IMAGE" version >"$ARTIFACT_ROOT/R3-dolt-version.txt"
run_one R3 dolt "$(free_port)"

printf 'PASS R1 R2 R3 completed=%s\n' "$(date -u +%FT%TZ)" | tee "$ARTIFACT_ROOT/PASS.txt"
