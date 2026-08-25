#!/usr/bin/env bash
# Focused clean-room acceptance: real DSH Agents consume a real Gitea-backed
# Workspace mounted read-only into both non-Git and Git projects.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PLUGIN="$ROOT/dsh-plugin"
source "$PLUGIN/scripts/agent-env.sh"
load_agent_api_env
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)-$$"
ARTIFACTS="${KC_PROJECT_MOUNT_ARTIFACTS:-$ROOT/.data/project-mount/$RUN_ID}"
PROFILE="${DSH_PROFILE:-loom-project-mount-$RUN_ID}"
GITEA_NAME="kc-project-mount-${RUN_ID//[^A-Za-z0-9]/}"
GITEA_IMAGE="${KC_GITEA_IMAGE:-gitea/gitea:1.26.3}"
KC_HOME="$ARTIFACTS/home"
KC_BIN="$ARTIFACTS/kc"
CONTROL="$ARTIFACTS/control"
MODEL_PATCH="${DSH_MODEL_PATCH:-$PLUGIN/scripts/deepseek-official.patch.yml}"
require_agent_api_key_for_patch "$MODEL_PATCH"
export PATH="$HOME/.local/go/bin:$HOME/.local/bin:$PATH"
mkdir -p "$KC_HOME" "$CONTROL"

free_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

cleanup() {
  if [[ -n "${KC_PID:-}" ]]; then
    kill "$KC_PID" >/dev/null 2>&1 || true
    wait "$KC_PID" 2>/dev/null || true
  fi
  if docker inspect "$GITEA_NAME" >"$ARTIFACTS/gitea-inspect-final.json" 2>/dev/null; then
    docker logs "$GITEA_NAME" >"$ARTIFACTS/gitea.log" 2>&1 || true
  fi
  docker rm -f "$GITEA_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "artifacts: $ARTIFACTS"
echo "==> build current kc and dsh-loom"
go -C "$ROOT" build -o "$KC_BIN" ./cmd/kc
(cd "$PLUGIN" && npm run build)

GITEA_PORT="$(free_port)"
GITEA_BASE="http://127.0.0.1:$GITEA_PORT"
echo "==> start clean real Gitea at $GITEA_BASE"
docker run -d --name "$GITEA_NAME" \
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
for _ in $(seq 1 90); do
  if curl -sS -f "$GITEA_BASE/api/v1/version" >"$ARTIFACTS/gitea-version.json" 2>/dev/null; then break; fi
  sleep 2
done
curl -sS -f "$GITEA_BASE/api/v1/version" >/dev/null
docker inspect "$GITEA_NAME" >"$ARTIFACTS/gitea-inspect-ready.json"
docker exec -u git "$GITEA_NAME" gitea admin user create \
  --admin --username kc --password kcpass123 --email kc@local --must-change-password=false >/dev/null
KC_GITEA_TOKEN="$(docker exec -u git "$GITEA_NAME" gitea admin user generate-access-token \
  --username kc --token-name "project-mount-$RUN_ID" --raw --scopes all | tail -1 | tr -d '\r')"
export KC_GITEA_TOKEN
REMOTE_REPO="kr://acme/knowledge/warehouse"
REMOTE_DSN="$GITEA_BASE/kc/project-mount-${RUN_ID//[^A-Za-z0-9]/}"

KC_PORT="$(free_port)"
KC_SERVE="http://127.0.0.1:$KC_PORT"
export KC_SERVE
echo "==> start kc serve and register Gitea knowledge Repository"
"$KC_BIN" --home "$KC_HOME" serve --listen "127.0.0.1:$KC_PORT" >"$ARTIFACTS/kc-serve.log" 2>&1 &
KC_PID=$!
for _ in $(seq 1 100); do
  if curl -sS -f "$KC_SERVE/health" >/dev/null 2>&1; then break; fi
  sleep 0.1
done
curl -sS -f "$KC_SERVE/health" >/dev/null
post() {
  curl -sS -f -X POST "$KC_SERVE/v1/$1" -H 'content-type: application/json' -d "$2"
}
post init '{"catalog":"kr://acme/catalog"}' >/dev/null
post repo-add "{\"repo\":\"$REMOTE_REPO\",\"driver\":\"gitea\",\"dsn\":\"$REMOTE_DSN\"}" >/dev/null
post define-workspace "{\"workspace\":\"project-knowledge\",\"revision\":1,\"source\":[\"$REMOTE_REPO=refs/heads/main@\"]}" >/dev/null

echo "==> install current plugin into an isolated DSH profile"
dsh plugin --profile "$PROFILE" add "file:$PLUGIN"
PROFILE_DIR="${DSH_HOME:-$HOME/.dsh}/profiles/$PROFILE"
python3 - "$PROFILE_DIR" <<'PY'
import json, sys
p = sys.argv[1] + "/package.json"
with open(p) as f:
    data = json.load(f)
bundles = data.setdefault("dsh", {}).setdefault("profile", {}).setdefault("bundles", [])
for name in ("@deepseek-ai/dsh-base", "@deepseek-ai/dsh-headless", "dsh-loom"):
    if name not in bundles:
        bundles.append(name)
with open(p, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
PY
KC_WORKSPACE=project-knowledge KC_MOUNT_PATH=.knowledge dsh --profile "$PROFILE" --dump-config >"$ARTIFACTS/dsh-config.yml"
if ! rg -q "KC_MOUNT_PATH" "$ARTIFACTS/dsh-config.yml" || ! rg -q "\.knowledge" "$ARTIFACTS/dsh-config.yml"; then
  echo "FAIL: DSH composition does not expose the project knowledge mount" >&2
  exit 1
fi
curl -sS -f "$GITEA_BASE/api/v1/version" >/dev/null

echo "==> real DSH Agent: non-Git pin + Git fresh update"
export DSH_PROFILE="$PROFILE"
export DSH_MODEL_PATCH="$MODEL_PATCH"
export KC_E2E_CONTROL="$CONTROL"
export KC_E2E_ARTIFACTS="$ARTIFACTS"
export KC_E2E_REMOTE_REPO="$REMOTE_REPO"
export KC_WORKSPACE=project-knowledge
python3 "$PLUGIN/scripts/e2e-project-mount.py" 2>&1 | tee "$ARTIFACTS/run.log"

echo "==> prove the two authoritative commits exist in Gitea"
curl -sS -f -H "Authorization: token $KC_GITEA_TOKEN" \
  "$GITEA_BASE/api/v1/repos/kc/project-mount-${RUN_ID//[^A-Za-z0-9]/}/commits?sha=main&limit=10" \
  >"$ARTIFACTS/gitea-commits.json"
python3 - "$ARTIFACTS" <<'PY'
import json, sys
from pathlib import Path
root = Path(sys.argv[1])
oracle = json.loads((root / "oracle.json").read_text())
commits = {row["sha"] for row in json.loads((root / "gitea-commits.json").read_text())}
missing = {oracle["v1Commit"], oracle["v2Commit"]} - commits
if missing:
    raise SystemExit(f"FAIL: authoritative commits absent from Gitea: {sorted(missing)}")
print("Gitea commits verified:", oracle["v1Commit"], oracle["v2Commit"])
PY
echo "PASS artifacts=$ARTIFACTS"
